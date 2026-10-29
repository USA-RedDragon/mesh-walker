package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/USA-RedDragon/mesh-walker/internal/logging"
	"github.com/USA-RedDragon/mesh-walker/internal/output"
	"github.com/USA-RedDragon/mesh-walker/internal/walker/walker"
)

var version = "dev"

func main() {
	slog.SetDefault(logging.New(os.Stdout, os.Stderr))
	if err := run(); err != nil {
		slog.Error("Walk failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localnode"
	}

	start := flag.String("start", hostname, "node to start the walk from")
	out := flag.String("out", "/meshmap/data/out.json", "path of the MeshMap data file to write")
	timeout := flag.Duration("timeout", 2*time.Minute, "timeout for each sysinfo request")
	retries := flag.Int("retries", 5, "attempts for each sysinfo request")
	jitter := flag.Duration("jitter", 5*time.Second, "maximum random delay before each attempt")
	concurrency := flag.Int("concurrency", 64, "maximum number of nodes fetched at the same time")
	progressInterval := flag.Duration("progress-interval", 2*time.Second, "how often to log walk progress")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}
	if *retries < 1 {
		return fmt.Errorf("--retries must be at least 1")
	}
	if *jitter <= 0 {
		return fmt.Errorf("--jitter must be greater than 0")
	}
	if *concurrency < 1 {
		return fmt.Errorf("--concurrency must be at least 1")
	}
	if *progressInterval <= 0 {
		return fmt.Errorf("--progress-interval must be greater than 0")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	walk := walker.NewWalker(*timeout, *retries, *jitter, *concurrency)
	respChan, err := walk.Walk(ctx, *start)
	if err != nil {
		return fmt.Errorf("failed to start walk: %w", err)
	}

	slog.Info("Starting walk", "startingNode", *start)

	var counts output.Counts
	go func() {
		for range time.Tick(*progressInterval) {
			completed := counts.Completed.Load()
			nonMapped := counts.NonMapped.Load()
			slog.Info("Still walking", "completed", completed, "total", walk.TotalCount.Value(), "mapped", completed-nonMapped, "unmapped", nonMapped)
		}
	}()

	err = output.Write(ctx, *out, respChan, &counts, walk.TotalCount.Value)
	if err != nil {
		return err
	}

	slog.Info("Finished walking")
	return nil
}
