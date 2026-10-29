// Package output writes the MeshMap data file in the same format as the
// mesh-manager walker.
package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/USA-RedDragon/mesh-walker/internal/apimodels"
)

// Counts holds progress counters that the caller may read while Write runs.
type Counts struct {
	Completed atomic.Int64
	NonMapped atomic.Int64
}

var now = time.Now

// Write consumes responses until the channel closes. Each response with a
// location is streamed to an intermediate file next to path. When the channel
// closes, the final file is written to path+".new" and renamed to path.
func Write(path string, responses <-chan *apimodels.SysinfoResponse, counts *Counts, hostsScraped func() int64) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	responsesFile, err := os.CreateTemp(dir, filepath.Base(path)+".responses-*")
	if err != nil {
		return fmt.Errorf("failed to create responses file: %w", err)
	}
	defer func() {
		_ = responsesFile.Close()
		_ = os.Remove(responsesFile.Name())
	}()

	w := bufio.NewWriter(responsesFile)

	n, err := w.Write([]byte("["))
	if err != nil {
		return fmt.Errorf("failed to write opening bracket: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("failed to write opening bracket: %w", err)
	}
	enc := json.NewEncoder(w)

	mapped := 0
	for resp := range responses {
		counts.Completed.Add(1)
		if resp == nil {
			continue
		}
		lat := resp.GetLatitude()
		lon := resp.GetLongitude()
		if lat != 0 && lon != 0 {
			RewriteSupernodeLinks(resp)

			err = enc.Encode(map[string]any{
				"data": resp.GetObject(),
			})
			if err != nil {
				return fmt.Errorf("failed to encode response for node %s (api %s): %w", resp.GetNode(), resp.APIVersion, err)
			}
			n, err = w.Write([]byte(","))
			if err != nil {
				return fmt.Errorf("failed to write comma: %w", err)
			}
			if n != 1 {
				return fmt.Errorf("failed to write comma: %w", err)
			}
			mapped++
		} else {
			counts.NonMapped.Add(1)
		}
	}

	if mapped == 0 {
		if _, err := w.Write([]byte("]")); err != nil {
			return fmt.Errorf("failed to write closing bracket: %w", err)
		}
	}

	err = w.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush responses: %w", err)
	}

	if mapped > 0 {
		// Replace the trailing comma with the closing bracket.
		_, err = responsesFile.Seek(-1, io.SeekEnd)
		if err != nil {
			return fmt.Errorf("failed to seek to end of file: %w", err)
		}
		written, err := responsesFile.Write([]byte("]"))
		if err != nil {
			return fmt.Errorf("failed to write closing bracket: %w", err)
		}
		if written != 1 {
			return fmt.Errorf("failed to write closing bracket: %w", err)
		}
	}

	header := map[string]any{
		"nonMapped":    int(counts.NonMapped.Load()),
		"hostsScraped": hostsScraped(),
		"date":         now().UTC().Format(time.RFC3339),
	}

	if err := createFile(path, header, responsesFile); err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	return nil
}

// RewriteSupernodeLinks marks the tunnel and WireGuard links of a supernode as
// supernode links.
func RewriteSupernodeLinks(resp *apimodels.SysinfoResponse) {
	if !resp.GetMeshSupernode() {
		return
	}
	linkInfo := resp.GetLinkInfo()
	if linkInfo == nil {
		return
	}
	// This matches mesh-manager exactly. API 1.7 to 1.14 responses hold a
	// LinkInfoMap1Point7, which the first case does not match, so only API 2.0
	// supernodes get the rewrite.
	switch v := linkInfo.(type) {
	case map[string]apimodels.LinkInfo1Point7:
		for key, value := range v {
			if value.LinkType == apimodels.LinkTypeTun || value.LinkType == apimodels.LinkTypeWireguard {
				value.LinkType = apimodels.LinkTypeSupernode
				v[key] = value
			}
		}
		resp.SetLinkInfo(v)
	case map[string]apimodels.LinkInfo2Point0:
		for key, value := range v {
			if value.LinkType == apimodels.LinkTypeTun || value.LinkType == apimodels.LinkTypeWireguard {
				value.LinkType = apimodels.LinkTypeSupernode
				v[key] = value
			}
		}
		resp.SetLinkInfo(v)
	}
}

func createFile(path string, header map[string]any, responsesFile *os.File) error {
	newPath := path + ".new"
	file, err := os.Create(newPath)
	if err != nil {
		return fmt.Errorf("error creating output file: %w", err)
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(newPath)
	}()

	if err := json.NewEncoder(file).Encode(header); err != nil {
		return fmt.Errorf("error encoding output file: %w", err)
	}

	// Replace the closing brace and newline with the nodeInfo key.
	_, err = file.Seek(-2, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("error seeking to before closing bracket: %w", err)
	}
	n, err := file.Write([]byte(",\"nodeInfo\":"))
	if err != nil {
		return fmt.Errorf("error writing nodeInfo key: %w", err)
	}
	if n != 12 {
		return fmt.Errorf("error writing nodeInfo key: %w", err)
	}
	_, err = responsesFile.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("error seeking to start of responses file: %w", err)
	}
	_, err = io.Copy(file, bufio.NewReader(responsesFile))
	if err != nil {
		return fmt.Errorf("error copying responses file to output file: %w", err)
	}
	n, err = file.Write([]byte("}"))
	if err != nil {
		return fmt.Errorf("error writing closing bracket: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("error writing closing bracket: %w", err)
	}
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("error syncing output file: %w", err)
	}
	err = file.Close()
	if err != nil {
		return fmt.Errorf("error closing output file: %w", err)
	}

	err = os.Rename(newPath, path)
	if err != nil {
		return fmt.Errorf("error renaming output file: %w", err)
	}
	return nil
}
