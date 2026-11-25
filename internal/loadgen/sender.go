package loadgen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hmzckd/eventlens/internal/event"
)

type Config struct {
	URL       string
	WriteKey  string
	Rate      int
	BatchSize int
	Workers   int
	Seed      int64
	Products  int
	Client    *http.Client
}

type Stats struct {
	Accepted atomic.Int64
	Busy     atomic.Int64
	Failed   atomic.Int64
	Requests atomic.Int64
}

func Run(ctx context.Context, cfg Config, stats *Stats) {
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.BatchSize > cfg.Rate {
		cfg.BatchSize = cfg.Rate
	}

	work := make(chan []byte)
	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for body := range work {
				send(ctx, cfg, stats, body)
			}
		}()
	}

	gen := NewGenerator(cfg.Seed, cfg.Products)
	interval := time.Duration(float64(time.Second) * float64(cfg.BatchSize) / float64(cfg.Rate))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var pending []event.Input
	for {
		select {
		case <-ctx.Done():
			close(work)
			wg.Wait()
			return
		case <-ticker.C:
		}
		for len(pending) < cfg.BatchSize {
			pending = append(pending, gen.Session(time.Now())...)
		}
		body, err := json.Marshal(map[string]any{"events": pending[:cfg.BatchSize]})
		if err != nil {
			panic(err)
		}
		pending = append(pending[:0], pending[cfg.BatchSize:]...)

		select {
		case work <- body:
		case <-ctx.Done():
		}
	}
}

func send(ctx context.Context, cfg Config, stats *Stats, body []byte) {
	n := int64(bytes.Count(body, []byte(`"event_id"`)))
	for {
		stats.Requests.Add(1)
		status, retryAfter, err := post(ctx, cfg, body)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			retryAfter = time.Second
		case status == http.StatusAccepted:
			stats.Accepted.Add(n)
			return
		case status == http.StatusServiceUnavailable:
			stats.Busy.Add(1)
		default:
			stats.Failed.Add(n)
			return
		}
		select {
		case <-time.After(retryAfter):
		case <-ctx.Done():
			return
		}
	}
}

func post(ctx context.Context, cfg Config, body []byte) (int, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL+"/v1/events", bytes.NewReader(body))
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Write-Key", cfg.WriteKey)
	resp, err := cfg.Client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("post events: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	retryAfter := time.Second
	if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs >= 0 {
		retryAfter = time.Duration(secs) * time.Second
	}
	return resp.StatusCode, retryAfter, nil
}
