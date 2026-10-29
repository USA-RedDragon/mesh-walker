package output

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCanonicalName(t *testing.T) {
	for in, want := range map[string]string{
		"ki5vmf-oklahoma":             "KI5VMF-OKLAHOMA",
		".KI5VMF-Oklahoma.local.mesh": "KI5VMF-OKLAHOMA",
		"node.LOCAL.MESH":             "NODE",
		"local.mesh":                  "LOCAL.MESH",
		"KI5VMF-OKLAHOMA-SUPERNODE":   "KI5VMF-OKLAHOMA-SUPERNODE",
	} {
		if got := CanonicalName(in); got != want {
			t.Errorf("CanonicalName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestShardOf pins the shard function. MeshMap computes the same values; the
// README lists these vectors.
func TestShardOf(t *testing.T) {
	for name, want := range shardVectors {
		if got := ShardOf(name); got != want {
			t.Errorf("ShardOf(%q) = %#x, want %#x", name, got, want)
		}
	}
}

var shardVectors = map[string]int{
	"":                          0xc5,
	"KI5VMF-OKLAHOMA":           0x7e,
	"KI5VMF-OKLAHOMA-SUPERNODE": 0xec,
	"KP4DJT-HAP":                0x94,
}

func readGzip(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// checkMapData checks map.json against the sample and that every node's
// detail file holds its full record.
func checkMapData(t *testing.T, dir string, sample sampleFile) {
	t.Helper()
	plain, err := os.ReadFile(filepath.Join(dir, "map.json"))
	if err != nil {
		t.Fatal(err)
	}
	if gz := readGzip(t, filepath.Join(dir, "map.json.gz")); string(gz) != string(plain) {
		t.Error("map.json.gz does not match map.json")
	}
	var m struct {
		Version        int              `json:"version"`
		Date           string           `json:"date"`
		HostsScraped   int64            `json:"hostsScraped"`
		NonMapped      int              `json:"nonMapped"`
		Details        string           `json:"details"`
		Shards         int              `json:"shards"`
		TrackerColumns []string         `json:"trackerColumns"`
		Nodes          []map[string]any `json:"nodes"`
	}
	if err := json.Unmarshal(plain, &m); err != nil {
		t.Fatal(err)
	}
	if m.Version != MapVersion || m.Date != sample.Date || m.HostsScraped != sample.HostsScraped || m.NonMapped != sample.NonMapped || m.Shards != Shards {
		t.Errorf("map.json header = %+v", m)
	}
	if len(m.Nodes) != len(sample.NodeInfo) {
		t.Fatalf("map.json has %d nodes, want %d", len(m.Nodes), len(sample.NodeInfo))
	}
	if len(m.TrackerColumns) != 11 {
		t.Errorf("trackerColumns = %v", m.TrackerColumns)
	}

	shards := map[int]map[string]json.RawMessage{}
	trackers := 0
	for i, n := range m.Nodes {
		var full map[string]any
		if err := json.Unmarshal(sample.NodeInfo[i].Data, &full); err != nil {
			t.Fatal(err)
		}
		if n["node"] != full["node"] || !reflect.DeepEqual(n["lat"], full["lat"]) || !reflect.DeepEqual(n["lon"], full["lon"]) {
			t.Errorf("node %d: map entry %v does not match %v/%v/%v", i, n["node"], full["node"], full["lat"], full["lon"])
		}
		if li, ok := full["link_info"].(map[string]any); ok {
			got, _ := n["link_info"].([]any)
			if len(got) != len(li) {
				t.Errorf("node %v: %d links, want %d", n["node"], len(got), len(li))
			}
		}
		if tr, ok := n["trackers"].([]any); ok {
			trackers += len(tr)
		}
		name := CanonicalName(n["node"].(string))
		s := ShardOf(name)
		if shards[s] == nil {
			b := readGzip(t, filepath.Join(dir, m.Details, fmtShard(s)))
			var sh map[string]json.RawMessage
			if err := json.Unmarshal(b, &sh); err != nil {
				t.Fatalf("shard %d: %v", s, err)
			}
			shards[s] = sh
		}
		rec, ok := shards[s][name]
		if !ok {
			t.Errorf("node %s missing from shard %d", name, s)
			continue
		}
		if !reflect.DeepEqual(normalize(t, rec), full) {
			t.Errorf("node %s: detail record differs", name)
		}
	}
	if trackers == 0 {
		t.Error("no LQM trackers in map.json")
	}
}

func fmtShard(s int) string {
	return fmt.Sprintf("%02x.json.gz", s)
}

func TestPruneDetailsKeepsNewestTwo(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 10, 29, 16, 0, 0, 0, time.UTC)
	for i := range 4 {
		if err := os.Mkdir(filepath.Join(dir, detailsPrefix+base.Add(time.Duration(i)*time.Hour).Format("20060102T150405Z")), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "details-other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := pruneDetails(dir, 2); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{"details-20261029T180000Z", "details-20261029T190000Z", "details-other"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("left %v, want %v", names, want)
	}
}
