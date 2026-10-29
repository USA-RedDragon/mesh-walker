package output

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/mesh-walker/internal/apimodels"
	"github.com/klauspost/compress/zstd"
)

type sampleFile struct {
	Date         string `json:"date"`
	HostsScraped int64  `json:"hostsScraped"`
	NonMapped    int    `json:"nonMapped"`
	NodeInfo     []struct {
		Data json.RawMessage `json:"data"`
	} `json:"nodeInfo"`
}

func loadSample(t *testing.T) ([]byte, sampleFile) {
	t.Helper()
	compressed, err := os.ReadFile(filepath.Join("testdata", "sample-out.json.zst"))
	if err != nil {
		t.Fatal(err)
	}
	dec, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()
	raw, err := dec.DecodeAll(compressed, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sample sampleFile
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatal(err)
	}
	if len(sample.NodeInfo) == 0 {
		t.Fatal("sample has no nodes")
	}
	return raw, sample
}

func normalize(t *testing.T, data []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	return v
}

// TestSampleRoundTrip decodes every node of a real mesh-manager out.json with
// the sysinfo models, writes them with Write, and checks that the result is
// the same JSON document.
func TestSampleRoundTrip(t *testing.T) {
	raw, sample := loadSample(t)

	date, err := time.Parse(time.RFC3339, sample.Date)
	if err != nil {
		t.Fatal(err)
	}
	now = func() time.Time { return date }
	t.Cleanup(func() { now = time.Now })

	responses := make(chan *apimodels.SysinfoResponse, 1)
	go func() {
		defer close(responses)
		for i, n := range sample.NodeInfo {
			var resp apimodels.SysinfoResponse
			if err := resp.Decode(bytes.NewReader(n.Data)); err != nil {
				t.Errorf("node %d: decode: %v", i, err)
				return
			}
			if resp.GetObject() == nil {
				t.Errorf("node %d: unsupported api_version %q", i, resp.APIVersion)
				return
			}
			responses <- &resp
			responses <- nil
		}
		for range sample.NonMapped {
			var resp apimodels.SysinfoResponse
			if err := resp.Decode(strings.NewReader(`{"api_version":"2.0","node":"NOLOC","lat":0,"lon":0}`)); err != nil {
				t.Errorf("decode unmapped node: %v", err)
				return
			}
			responses <- &resp
		}
	}()

	out := filepath.Join(t.TempDir(), "out.json")
	var counts Counts
	if err := Write(context.Background(), out, responses, &counts, func() int64 { return sample.HostsScraped }); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(normalize(t, got), normalize(t, raw)) {
		var gotSample sampleFile
		_ = json.Unmarshal(got, &gotSample)
		for i := range sample.NodeInfo {
			if i >= len(gotSample.NodeInfo) {
				t.Errorf("node %d missing from output", i)
				continue
			}
			if !reflect.DeepEqual(normalize(t, gotSample.NodeInfo[i].Data), normalize(t, sample.NodeInfo[i].Data)) {
				t.Errorf("node %d differs:\n got: %s\nwant: %s", i, gotSample.NodeInfo[i].Data, sample.NodeInfo[i].Data)
			}
		}
		t.Fatal("output does not match the sample")
	}

	wantPrefix := `{"date":"` + sample.Date + `","hostsScraped":` +
		jsonNumber(sample.HostsScraped) + `,"nonMapped":` + jsonNumber(int64(sample.NonMapped)) +
		`,"nodeInfo":[{"data":{"api_version":`
	if !bytes.HasPrefix(got, []byte(wantPrefix)) {
		t.Errorf("output starts with %q, want %q", got[:min(len(got), len(wantPrefix))], wantPrefix)
	}
	if !bytes.HasSuffix(got, []byte("}}\n]}")) {
		t.Errorf("output ends with %q, want %q", got[max(0, len(got)-8):], "}}\n]}")
	}
	if n := bytes.Count(got, []byte("}\n,{\"data\":")); n != len(sample.NodeInfo)-1 {
		t.Errorf("found %d entry separators, want %d", n, len(sample.NodeInfo)-1)
	}

	if c := counts.Completed.Load(); c != int64(2*len(sample.NodeInfo)+sample.NonMapped) {
		t.Errorf("Completed = %d", c)
	}

	entries, err := os.ReadDir(filepath.Dir(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("output directory has %d entries, want only out.json", len(entries))
	}
}

func jsonNumber(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestWriteNoMappedNodes(t *testing.T) {
	responses := make(chan *apimodels.SysinfoResponse)
	close(responses)
	out := filepath.Join(t.TempDir(), "out.json")
	var counts Counts
	if err := Write(context.Background(), out, responses, &counts, func() int64 { return 0 }); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := normalize(t, got).(map[string]any)
	if !ok {
		t.Fatalf("output is not an object: %s", got)
	}
	if nodes, ok := v["nodeInfo"].([]any); !ok || len(nodes) != 0 {
		t.Errorf("nodeInfo = %v, want empty array", v["nodeInfo"])
	}
}

func TestSupernodeRewrite(t *testing.T) {
	var resp apimodels.SysinfoResponse
	err := resp.Decode(strings.NewReader(`{
		"api_version": "2.0",
		"lat": 1, "lon": 1,
		"node_details": {"mesh_supernode": true},
		"link_info": {
			"a": {"hostname": "a", "linkType": "TUN"},
			"b": {"hostname": "b", "linkType": "WIREGUARD"},
			"c": {"hostname": "c", "linkType": "RF"}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	RewriteSupernodeLinks(&resp)
	li := resp.SysinfoResponse2Point0.LinkInfo
	if li["a"].LinkType != apimodels.LinkTypeSupernode || li["b"].LinkType != apimodels.LinkTypeSupernode {
		t.Errorf("tunnel links not rewritten: %+v", li)
	}
	if li["c"].LinkType != apimodels.LinkTypeRF {
		t.Errorf("RF link changed: %+v", li["c"])
	}
}

// TestSupernodeRewriteSkipsOldAPIs pins mesh-manager's behavior: the rewrite
// does not apply to API 1.7 to 1.14 responses.
func TestSupernodeRewriteSkipsOldAPIs(t *testing.T) {
	var resp apimodels.SysinfoResponse
	err := resp.Decode(strings.NewReader(`{
		"api_version": "1.14",
		"lat": "1", "lon": "1",
		"node_details": {"mesh_supernode": true},
		"link_info": {"a": {"hostname": "a", "linkType": "TUN"}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	RewriteSupernodeLinks(&resp)
	if lt := resp.SysinfoResponse1Point14.LinkInfo["a"].LinkType; lt != apimodels.LinkTypeTun {
		t.Errorf("linkType = %s, want TUN", lt)
	}
}

func TestWriteCanceledRemovesTempFiles(t *testing.T) {
	dir := t.TempDir()
	responses := make(chan *apimodels.SysinfoResponse)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var counts Counts
	go func() {
		done <- Write(ctx, filepath.Join(dir, "out.json"), responses, &counts, func() int64 { return 0 })
	}()
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected an error after cancel")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("left %d files behind", len(entries))
	}
}
