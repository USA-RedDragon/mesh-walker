package main

import (
	"bufio"
	"encoding/json"
	"fmt"
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

	n, err := w.Write([]byte("["))
	if err != nil {
		log.Fatalf("Error writing opening bracket: %v", err)
	}
	if n != 1 {
		log.Fatalf("Error writing opening bracket: %v", err)
	}
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

			err = enc.Encode(nodeEntry{
				Data: *resp,
			})
			if err != nil {
				log.Fatalf("Error encoding response: %v", err)
			}
			n, err = w.Write([]byte(","))
			if err != nil {
				log.Fatalf("Error writing comma: %v", err)
			}
			if n != 1 {
				log.Fatalf("Error writing comma: %v", err)
			}
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
		"nonMapped":    nonMapped,
		"hostsScraped": walk.TotalCount.Value(),
		"date":         time.Now().UTC().Format(time.RFC3339),
	}

	err = createFile(output, responsesFile)
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}
}

func createFile(output map[string]interface{}, responsesFile *os.File) error {
	file, err := os.Create("/usr/share/nginx/html/data/out.json.new")
	if err != nil {
		return fmt.Errorf("error creating output file: %w", err)
	}

	if err := json.NewEncoder(file).Encode(output); err != nil {
		return fmt.Errorf("error encoding output file: %w", err)
	}

	err = file.Close()
	if err != nil {
		return fmt.Errorf("error closing output file: %w", err)
	}

	// Now we need to combine out.json.new and responses.json
	file, err = os.OpenFile("/usr/share/nginx/html/data/out.json.new", os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error opening output file: %w", err)
	}

	// Seek to before the closing bracket
	_, err = file.Seek(-2, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("error seeking to before closing bracket: %w", err)
	}
	// Replace the closing bracket
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
	r := bufio.NewReader(responsesFile)
	_, err = io.Copy(file, r)
	if err != nil {
		return fmt.Errorf("error copying responses file to output file: %w", err)
	}
	// Write the closing bracket
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
	err = responsesFile.Close()
	if err != nil {
		return fmt.Errorf("error closing responses file: %w", err)
	}

	err = os.Rename("/usr/share/nginx/html/data/out.json.new", "/usr/share/nginx/html/data/out.json")
	if err != nil {
		return fmt.Errorf("error renaming output file: %w", err)
	}

	return nil
}
