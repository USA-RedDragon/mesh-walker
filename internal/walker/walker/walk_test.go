package walker

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeMesh struct {
	links    map[string][]string
	inFlight atomic.Int64
	maxSeen  atomic.Int64
	mu       sync.Mutex
	hits     map[string]int
}

func (m *fakeMesh) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cur := m.inFlight.Add(1)
	defer m.inFlight.Add(-1)
	for {
		prev := m.maxSeen.Load()
		if cur <= prev || m.maxSeen.CompareAndSwap(prev, cur) {
			break
		}
	}
	time.Sleep(2 * time.Millisecond)

	node := r.URL.Query().Get("node")
	m.mu.Lock()
	m.hits[node]++
	m.mu.Unlock()

	if strings.HasPrefix(node, "dead") {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	hosts := []map[string]string{}
	for _, h := range m.links[node] {
		hosts = append(hosts, map[string]string{"name": h, "ip": "10.0.0.1"})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"api_version": "2.0",
		"node":        node,
		"lat":         1.5,
		"lon":         2.5,
		"hosts":       hosts,
	})
}

func TestWalkVisitsEveryHostOnceWithinConcurrency(t *testing.T) {
	const nodes = 60
	const concurrency = 4

	r := rand.New(rand.NewSource(1))
	mesh := &fakeMesh{links: map[string][]string{}, hits: map[string]int{}}
	for i := range nodes {
		name := fmt.Sprintf("node-%d", i)
		for range 8 {
			mesh.links[name] = append(mesh.links[name], fmt.Sprintf("NODE-%d", r.Intn(nodes)))
		}
		mesh.links[name] = append(mesh.links[name], fmt.Sprintf("node-%d", (i+1)%nodes))
	}
	mesh.links["node-0"] = append(mesh.links["node-0"],
		"lan.node-0.local.mesh", "mid1.node-0", "dead-1", "dead-2")

	srv := httptest.NewServer(mesh)
	defer srv.Close()

	w := NewWalker(5*time.Second, 2, time.Nanosecond, concurrency)
	w.nodeURL = func(node string) string {
		return srv.URL + "/cgi-bin/sysinfo.json?node=" + url.QueryEscape(strings.ToLower(node))
	}

	ch, err := w.Walk(context.Background(), "node-0")
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	got := 0
	failed := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		for resp := range ch {
			if resp == nil {
				failed++
				continue
			}
			got++
		}
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("walk did not finish")
	}

	// The starting node is not marked as seen, as in mesh-manager, so it is
	// fetched a second time when another node lists it.
	if got != nodes+1 {
		t.Errorf("got %d responses, want %d", got, nodes+1)
	}
	if failed != 2 {
		t.Errorf("got %d failed fetches, want 2", failed)
	}
	if total := w.TotalCount.Value(); total != nodes+2 {
		t.Errorf("TotalCount = %d, want %d", total, nodes+2)
	}
	if maxSeen := mesh.maxSeen.Load(); maxSeen > concurrency {
		t.Errorf("saw %d requests at once, limit is %d", maxSeen, concurrency)
	}
	for node, hits := range mesh.hits {
		want := 1
		if node == "node-0" {
			want = 2
		}
		if strings.HasPrefix(node, "dead") {
			want = 2
		}
		if hits != want {
			t.Errorf("node %s fetched %d times, want %d", node, hits, want)
		}
	}
	if _, ok := mesh.hits["lan.node-0.local.mesh"]; ok {
		t.Error("lan. host was fetched")
	}
	if _, ok := mesh.hits["mid1.node-0"]; ok {
		t.Error("mid host was fetched")
	}
}

func TestWalkStartWithoutHosts(t *testing.T) {
	mesh := &fakeMesh{links: map[string][]string{}, hits: map[string]int{}}
	srv := httptest.NewServer(mesh)
	defer srv.Close()

	w := NewWalker(5*time.Second, 1, time.Nanosecond, 2)
	w.nodeURL = func(node string) string {
		return srv.URL + "/?node=" + url.QueryEscape(node)
	}
	ch, err := w.Walk(context.Background(), "alone")
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	n := 0
	for range ch {
		n++
	}
	if n != 1 {
		t.Errorf("got %d responses, want 1", n)
	}
}

func TestWalkStartFailure(t *testing.T) {
	mesh := &fakeMesh{links: map[string][]string{}, hits: map[string]int{}}
	srv := httptest.NewServer(mesh)
	defer srv.Close()

	w := NewWalker(5*time.Second, 1, time.Nanosecond, 2)
	w.nodeURL = func(node string) string {
		return srv.URL + "/?node=" + url.QueryEscape(node)
	}
	if _, err := w.Walk(context.Background(), "dead-start"); err == nil {
		t.Fatal("expected an error for an unreachable starting node")
	}
}
