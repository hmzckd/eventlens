package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hmzckd/eventlens/internal/loadgen"
)

func main() {
	cfg := loadgen.Config{}
	flag.StringVar(&cfg.URL, "url", "http://localhost:8080", "collector base URL")
	flag.StringVar(&cfg.WriteKey, "key", os.Getenv("EVENTLENS_WRITE_KEY"), "write key (default: $EVENTLENS_WRITE_KEY)")
	flag.IntVar(&cfg.Rate, "rate", 2000, "events per second to send")
	flag.IntVar(&cfg.BatchSize, "batch", 200, "events per request")
	flag.IntVar(&cfg.Workers, "workers", 8, "requests in flight at the same time")
	flag.Int64Var(&cfg.Seed, "seed", time.Now().UnixNano(), "random seed")
	flag.IntVar(&cfg.Products, "products", 200, "number of products in the shop")
	duration := flag.Duration("duration", 0, "stop after this long (0 = until Ctrl+C)")
	flag.Parse()

	if cfg.WriteKey == "" {
		fmt.Fprintln(os.Stderr, "set -key or EVENTLENS_WRITE_KEY")
		os.Exit(2)
	}
	if cfg.Rate <= 0 || cfg.BatchSize <= 0 || cfg.BatchSize > 500 || cfg.Workers <= 0 || cfg.Products < 2 {
		fmt.Fprintln(os.Stderr, "rate, workers must be positive, batch 1..500, products at least 2")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}

	fmt.Printf("sending about %d events/s to %s (batch %d, workers %d), Ctrl+C to stop\n",
		cfg.Rate, cfg.URL, cfg.BatchSize, cfg.Workers)

	var stats loadgen.Stats
	done := make(chan struct{})
	go func() {
		loadgen.Run(ctx, cfg, &stats)
		close(done)
	}()

	start := time.Now()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var lastAccepted int64
	for {
		select {
		case <-done:
			fmt.Printf("done in %s: accepted %d events, %d busy answers (503), %d events failed\n",
				time.Since(start).Round(time.Second), stats.Accepted.Load(), stats.Busy.Load(), stats.Failed.Load())
			return
		case <-ticker.C:
			accepted := stats.Accepted.Load()
			fmt.Printf("accepted %6d events/s | total %9d | 503 answers %5d | failed %d\n",
				(accepted-lastAccepted)/2, accepted, stats.Busy.Load(), stats.Failed.Load())
			lastAccepted = accepted
		}
	}
}
