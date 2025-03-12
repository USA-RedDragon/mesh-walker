package main

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/USA-RedDragon/mesh-walker/internal/data"
	"github.com/USA-RedDragon/mesh-walker/internal/walker"
)

type nodeEntry struct {
	Data data.Response `json:"data"`
}

func main() {
	startingNode := "KI5VMF-oklahoma-supernode"
	walk := walker.NewWalker(2*time.Minute, 5, 5*time.Second)

	respChan, err := walk.Walk(startingNode)
	if err != nil {
		log.Fatalf("Error running walker: %v", err)
	}

	slog.Info("Starting walk", "startingNode", startingNode)

	nonMapped := 0
	completed := 0
	go func() {
		for range time.Tick(2 * time.Second) {
			total := walk.TotalCount.Value()
			mapped := completed - nonMapped
			slog.Info("Still walking", "completed", completed, "total", total, "mapped", mapped, "unmapped", nonMapped)
		}
	}()

	responsesFile, err := os.CreateTemp(os.TempDir(), "responses.json")
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}

	w := bufio.NewWriter(responsesFile)

	w.Write([]byte("["))
	enc := json.NewEncoder(w)

	for resp := range respChan {
		completed++
		if resp == nil {
			continue
		}
		if resp.Lat != "" && resp.Lon != "" {
			if resp.NodeDetails.MeshSupernode {
				for key, value := range resp.LinkInfo {
					if value.LinkType == data.LinkTypeTunnel || value.LinkType == data.LinkTypeWireguard {
						value.LinkType = data.LinkTypeSupernode
						resp.LinkInfo[key] = value
					}
				}
			}

			enc.Encode(nodeEntry{
				Data: *resp,
			})
			w.Write([]byte(","))
		} else {
			nonMapped++
		}
	}

	err = w.Flush()
	if err != nil {
		log.Fatalf("Error flushing output file: %v", err)
	}

	// We now need to delete the last comma and replace it with a closing bracket
	_, err = responsesFile.Seek(-1, io.SeekEnd)
	if err != nil {
		log.Fatalf("Error seeking to end of file: %v", err)
	}
	written, err := responsesFile.Write([]byte("]"))
	if err != nil {
		log.Fatalf("Error writing closing bracket: %v", err)
	}
	if written != 1 {
		log.Fatalf("Error writing closing bracket: %v", err)
	}
	err = responsesFile.Sync()
	if err != nil {
		log.Fatalf("Error syncing output file: %v", err)
	}

	slog.Info("Finished walking")

	// Save output
	output := map[string]interface{}{
		// "nodeInfo":     nodeInfo,
		"nonMapped":    nonMapped,
		"hostsScraped": walk.TotalCount.Value(),
		"date":         time.Now().UTC().Format(time.RFC3339),
	}

	file, err := os.Create("/usr/share/nginx/html/data/out.json.new")
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}

	if err := json.NewEncoder(file).Encode(output); err != nil {
		log.Fatalf("Error writing to output file: %v", err)
	}

	err = file.Close()
	if err != nil {
		log.Fatalf("Error closing output file: %v", err)
	}

	// Now we need to combine out.json.new and responses.json
	file, err = os.OpenFile("/usr/share/nginx/html/data/out.json.new", os.O_RDWR, 0644)
	if err != nil {
		log.Fatalf("Error opening output file: %v", err)
	}

	// Seek to before the closing bracket
	_, err = file.Seek(-2, io.SeekEnd)
	if err != nil {
		log.Fatalf("Error seeking to end of file: %v", err)
	}
	// Replace the closing bracket
	file.Write([]byte(",\"nodeInfo\":"))
	responsesFile.Seek(0, io.SeekStart)
	r := bufio.NewReader(responsesFile)
	_, err = io.Copy(file, r)
	if err != nil {
		log.Fatalf("Error copying responses file to output file: %v", err)
	}
	// Write the closing bracket
	file.Write([]byte("}"))
	err = file.Sync()
	if err != nil {
		log.Fatalf("Error syncing output file: %v", err)
	}
	err = responsesFile.Close()
	if err != nil {
		log.Fatalf("Error closing responses file: %v", err)
	}

	err = os.Rename("/usr/share/nginx/html/data/out.json.new", "/usr/share/nginx/html/data/out.json")
	if err != nil {
		log.Fatalf("Error renaming output file: %v", err)
	}
}
