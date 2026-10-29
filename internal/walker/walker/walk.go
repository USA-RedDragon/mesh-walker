package walker

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/USA-RedDragon/mesh-walker/internal/apimodels"
	"github.com/USA-RedDragon/mesh-walker/internal/walker/http"
)

var midMatch = regexp.MustCompile(`^mid[0-9]+\.`)

type task struct {
	hostname   string
	sourceNode string
}

// Counter is a concurrency-safe counter.
type Counter struct {
	v atomic.Int64
}

func (c *Counter) Inc() { c.v.Add(1) }

func (c *Counter) Value() int64 { return c.v.Load() }

// Walker crawls the mesh from a starting node. It fetches at most
// concurrency nodes at a time and queues the rest as plain hostnames.
type Walker struct {
	client      *http.Client
	concurrency int
	TotalCount  *Counter

	mu     sync.Mutex
	cond   *sync.Cond
	queue  []task
	active int
	seen   map[string]struct{}
}

func NewWalker(timeout time.Duration, retries int, jitter time.Duration, concurrency int) *Walker {
	if concurrency < 1 {
		concurrency = 1
	}
	w := &Walker{
		client:      http.NewClient(timeout, retries, jitter),
		concurrency: concurrency,
		TotalCount:  &Counter{},
		seen:        make(map[string]struct{}),
	}
	w.cond = sync.NewCond(&w.mu)
	return w
}

// Walk fetches the starting node, then crawls every host it learns about. The
// returned channel receives one response per host (nil when a fetch failed)
// and closes when the walk is done.
func (w *Walker) Walk(ctx context.Context, startingNode string) (<-chan *apimodels.SysinfoResponse, error) {
	resp, err := w.walk(ctx, startingNode)
	if err != nil {
		return nil, fmt.Errorf("failed to walk starting node: %w", err)
	}

	responses := make(chan *apimodels.SysinfoResponse, 1)
	responses <- resp

	var wg sync.WaitGroup
	for range w.concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.worker(ctx, responses)
		}()
	}
	go func() {
		wg.Wait()
		close(responses)
	}()

	return responses, nil
}

func (w *Walker) worker(ctx context.Context, responses chan<- *apimodels.SysinfoResponse) {
	for {
		w.mu.Lock()
		for len(w.queue) == 0 && w.active > 0 {
			w.cond.Wait()
		}
		if len(w.queue) == 0 {
			w.mu.Unlock()
			w.cond.Broadcast()
			return
		}
		t := w.queue[0]
		w.queue[0] = task{}
		w.queue = w.queue[1:]
		w.active++
		w.mu.Unlock()

		response, err := w.walk(ctx, t.hostname)
		if err != nil {
			if strings.Contains(err.Error(), "Client.Timeout") {
				slog.Debug("Timeout fetching data", "node", t.hostname, "source", t.sourceNode, "error", err)
			} else {
				slog.Error("Error fetching data", "node", t.hostname, "source", t.sourceNode, "error", err)
			}
		}
		responses <- response

		w.mu.Lock()
		w.active--
		w.mu.Unlock()
		w.cond.Broadcast()
	}
}

func (w *Walker) walk(ctx context.Context, node string) (*apimodels.SysinfoResponse, error) {
	url := fmt.Sprintf("http://%s.local.mesh:8080/cgi-bin/sysinfo.json?hosts=1&link_info=1&lqm=1", node)
	resp, err := w.client.Get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get sysinfo from node %s: %w", node, err)
	}

	queued := false
	w.mu.Lock()
	for _, host := range resp.GetHosts() {
		if (strings.HasPrefix(host.Name, "lan.") && strings.HasSuffix(host.Name, ".local.mesh")) || midMatch.MatchString(host.Name) {
			continue
		}
		key := strings.ToUpper(host.Name)
		if _, ok := w.seen[key]; ok {
			continue
		}
		w.seen[key] = struct{}{}
		w.TotalCount.Inc()
		w.queue = append(w.queue, task{hostname: host.Name, sourceNode: node})
		queued = true
	}
	w.mu.Unlock()
	if queued {
		w.cond.Broadcast()
	}

	return resp, nil
}
