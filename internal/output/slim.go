package output

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MapVersion is the version of the map.json format.
const MapVersion = 1

// Shards is the number of node detail files.
const Shards = 256

const detailsPrefix = "details-"

// CanonicalName is the node name MeshMap keys nodes by: no leading dot, no
// .local.mesh suffix, upper case.
func CanonicalName(hostname string) string {
	h := strings.TrimPrefix(hostname, ".")
	if len(h) >= len(".local.mesh") && strings.EqualFold(h[len(h)-len(".local.mesh"):], ".local.mesh") {
		h = h[:len(h)-len(".local.mesh")]
	}
	return strings.ToUpper(h)
}

// ShardOf is the detail file that holds the node with this canonical name:
// FNV-1a 32 of its UTF-8 bytes, modulo Shards.
func ShardOf(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return int(h.Sum32() % Shards)
}

type slimRF struct {
	Status  any `json:"status,omitempty"`
	Channel any `json:"channel,omitempty"`
	Freq    any `json:"freq,omitempty"`
	Chanbw  any `json:"chanbw,omitempty"`
	Azimuth any `json:"azimuth,omitempty"`
}

type slimLink struct {
	Hostname any `json:"hostname,omitempty"`
	LinkType any `json:"linkType,omitempty"`
	Signal   any `json:"signal,omitempty"`
	Noise    any `json:"noise,omitempty"`
	Bitrate  any `json:"bitrate,omitempty"`
	Rate     any `json:"rate,omitempty"`
}

type sourceTracker struct {
	Hostname           any `json:"hostname"`
	IP                 any `json:"ip"`
	CanonicalIP        any `json:"canonical_ip"`
	Quality            any `json:"quality"`
	RevQuality         any `json:"rev_quality"`
	PingSuccessTime    any `json:"ping_success_time"`
	RevPingSuccessTime any `json:"rev_ping_success_time"`
	PingQuality        any `json:"ping_quality"`
	RevPingQuality     any `json:"rev_ping_quality"`
	BabelRouteCount    any `json:"babel_route_count"`
	BabelMetric        any `json:"babel_metric"`
}

type sourceNode struct {
	Node        string  `json:"node"`
	Lat         any     `json:"lat"`
	Lon         any     `json:"lon"`
	Mlat        any     `json:"mlat"`
	Mlon        any     `json:"mlon"`
	Chanbw      any     `json:"chanbw"`
	Meshrf      *slimRF `json:"meshrf"`
	NodeDetails struct {
		MeshSupernode bool `json:"mesh_supernode"`
	} `json:"node_details"`
	Interfaces []struct {
		IP string `json:"ip"`
	} `json:"interfaces"`
	LinkInfo json.RawMessage `json:"link_info"`
	LQM      *struct {
		Info *struct {
			Trackers json.RawMessage `json:"trackers"`
		} `json:"info"`
		Trackers json.RawMessage `json:"trackers"`
	} `json:"lqm"`
}

type slimDetails struct {
	MeshSupernode bool `json:"mesh_supernode"`
}

type slimInterface struct {
	IP string `json:"ip"`
}

// slimNode is one node of map.json. Its keys match the sysinfo record so
// MeshMap reads it the same way. Trackers holds the LQM trackers as rows in
// the order of trackerColumns.
type slimNode struct {
	Node        string          `json:"node"`
	Lat         any             `json:"lat,omitempty"`
	Lon         any             `json:"lon,omitempty"`
	Mlat        any             `json:"mlat,omitempty"`
	Mlon        any             `json:"mlon,omitempty"`
	Chanbw      any             `json:"chanbw,omitempty"`
	Meshrf      *slimRF         `json:"meshrf,omitempty"`
	NodeDetails *slimDetails    `json:"node_details,omitempty"`
	Interfaces  []slimInterface `json:"interfaces,omitempty"`
	LinkInfo    []slimLink      `json:"link_info,omitempty"`
	Trackers    [][]any         `json:"trackers,omitempty"`
}

var trackerColumns = []string{
	"hostname", "ip", "canonical_ip", "quality", "rev_quality", "ping_success_time",
	"rev_ping_success_time", "ping_quality", "rev_ping_quality", "babel_route_count", "babel_metric",
}

type mapFile struct {
	Version        int        `json:"version"`
	Date           string     `json:"date"`
	HostsScraped   int64      `json:"hostsScraped"`
	NonMapped      int        `json:"nonMapped"`
	Details        string     `json:"details"`
	Shards         int        `json:"shards"`
	TrackerColumns []string   `json:"trackerColumns"`
	Nodes          []slimNode `json:"nodes"`
}

// round keeps floats short: digits decimal places.
func round(v any, digits int) any {
	f, ok := v.(float64)
	if !ok {
		return v
	}
	p := math.Pow(10, float64(digits))
	return math.Round(f*p) / p
}

func emptyToNil(v any) any {
	if s, ok := v.(string); ok && s == "" {
		return nil
	}
	return v
}

// decodeEntries reads a JSON object or array of objects into a list.
func decodeEntries[T any](raw json.RawMessage) []T {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]T
	if err := json.Unmarshal(raw, &m); err == nil {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]T, 0, len(m))
		for _, k := range keys {
			out = append(out, m[k])
		}
		return out
	}
	var a []T
	if err := json.Unmarshal(raw, &a); err == nil {
		return a
	}
	return nil
}

func slimOf(data []byte) (slimNode, error) {
	var src sourceNode
	if err := json.Unmarshal(data, &src); err != nil {
		return slimNode{}, err
	}
	n := slimNode{
		Node:   src.Node,
		Lat:    emptyToNil(src.Lat),
		Lon:    emptyToNil(src.Lon),
		Mlat:   emptyToNil(src.Mlat),
		Mlon:   emptyToNil(src.Mlon),
		Chanbw: emptyToNil(src.Chanbw),
		Meshrf: src.Meshrf,
	}
	if src.NodeDetails.MeshSupernode {
		n.NodeDetails = &slimDetails{MeshSupernode: true}
	}
	for _, i := range src.Interfaces {
		if i.IP != "" {
			n.Interfaces = append(n.Interfaces, slimInterface(i))
		}
	}
	n.LinkInfo = decodeEntries[slimLink](src.LinkInfo)
	if src.LQM != nil {
		raw := src.LQM.Trackers
		if src.LQM.Info != nil && len(src.LQM.Info.Trackers) > 0 {
			raw = src.LQM.Info.Trackers
		}
		for _, t := range decodeEntries[sourceTracker](raw) {
			n.Trackers = append(n.Trackers, []any{
				t.Hostname, t.IP, t.CanonicalIP,
				round(t.Quality, 1), round(t.RevQuality, 1),
				round(t.PingSuccessTime, 5), round(t.RevPingSuccessTime, 5),
				round(t.PingQuality, 1), round(t.RevPingQuality, 1),
				t.BabelRouteCount, t.BabelMetric,
			})
		}
	}
	return n, nil
}

type record struct {
	name  string
	shard int
	off   int64
	size  int
}

// slimWriter collects map.json in memory while the walk runs, and where each
// node's record sits in the responses file, so the detail files can be cut
// from that file afterwards without holding the records.
type slimWriter struct {
	nodes   []slimNode
	records []record
}

func (s *slimWriter) add(data []byte, off int64) error {
	n, err := slimOf(data)
	if err != nil {
		return err
	}
	s.nodes = append(s.nodes, n)
	name := CanonicalName(n.Node)
	s.records = append(s.records, record{name: name, shard: ShardOf(name), off: off, size: len(data)})
	return nil
}

// write writes the detail files into a new details-<stamp> directory, then
// map.json and map.json.gz, then removes all but the two newest detail
// directories. map.json names its directory, and is replaced last, so it
// never points at files that are not there yet.
func (s *slimWriter) write(dir string, responses io.ReaderAt, header mapFile) error {
	stamp := now().UTC().Format("20060102T150405Z")
	details := detailsPrefix + stamp
	tmp, err := os.MkdirTemp(dir, ".details-*")
	if err != nil {
		return fmt.Errorf("failed to create details directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	byShard := make([][]record, Shards)
	for _, r := range s.records {
		byShard[r.shard] = append(byShard[r.shard], r)
	}
	buf := make([]byte, 0, 64*1024)
	for shard, recs := range byShard {
		if err := writeShard(filepath.Join(tmp, fmt.Sprintf("%02x.json.gz", shard)), recs, responses, &buf); err != nil {
			return err
		}
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return fmt.Errorf("failed to set details directory mode: %w", err)
	}
	final := filepath.Join(dir, details)
	_ = os.RemoveAll(final)
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("failed to rename details directory: %w", err)
	}

	header.Version = MapVersion
	header.Details = details + "/"
	header.Shards = Shards
	header.TrackerColumns = trackerColumns
	header.Nodes = s.nodes
	if header.Nodes == nil {
		header.Nodes = []slimNode{}
	}
	body, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to encode map.json: %w", err)
	}
	if err := writeFile(filepath.Join(dir, "map.json.gz"), func(w io.Writer) error { return gzipTo(w, body) }); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "map.json"), func(w io.Writer) error { _, err := w.Write(body); return err }); err != nil {
		return err
	}
	return pruneDetails(dir, 2)
}

func gzipTo(w io.Writer, body []byte) error {
	gz := gzip.NewWriter(w)
	if _, err := gz.Write(body); err != nil {
		return err
	}
	return gz.Close()
}

func writeShard(path string, recs []record, responses io.ReaderAt, buf *[]byte) error {
	return writeFile(path, func(w io.Writer) error {
		gz := gzip.NewWriter(w)
		bw := bufio.NewWriter(gz)
		_ = bw.WriteByte('{')
		for i, r := range recs {
			if i > 0 {
				_ = bw.WriteByte(',')
			}
			key, _ := json.Marshal(r.name)
			_, _ = bw.Write(key)
			_ = bw.WriteByte(':')
			if cap(*buf) < r.size {
				*buf = make([]byte, r.size)
			}
			b := (*buf)[:r.size]
			if _, err := responses.ReadAt(b, r.off); err != nil {
				return fmt.Errorf("failed to read record of %s: %w", r.name, err)
			}
			_, _ = bw.Write(b)
		}
		_ = bw.WriteByte('}')
		if err := bw.Flush(); err != nil {
			return err
		}
		return gz.Close()
	})
}

// writeFile writes path through a temporary file and a rename.
func writeFile(path string, fill func(io.Writer) error) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()
	if err := fill(f); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := f.Chmod(0o644); err != nil {
		return fmt.Errorf("failed to set mode of %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %w", path, err)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("failed to rename %s: %w", path, err)
	}
	return nil
}

// pruneDetails keeps the newest keep details-<stamp> directories, so a page
// that loaded the previous map.json can still open its detail files.
func pruneDetails(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to list %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), detailsPrefix) {
			if _, err := time.Parse("20060102T150405Z", strings.TrimPrefix(e.Name(), detailsPrefix)); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		if err := os.RemoveAll(filepath.Join(dir, names[0])); err != nil {
			return fmt.Errorf("failed to remove %s: %w", names[0], err)
		}
		names = names[1:]
	}
	return nil
}
