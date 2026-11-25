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

const (
	startEvery = 100 * time.Millisecond
	flushEvery = time.Second
)

func Run(ctx context.Context, cfg Config, stats *Stats) {
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.BatchSize > cfg.Rate {
		cfg.BatchSize = cfg.Rate
	}

	work := make(chan []byte)
	var workers sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for body := range work {
				send(ctx, cfg, stats, body)
			}
		}()
	}

	happened := make(chan event.Input, cfg.BatchSize)
	var visitors sync.WaitGroup
	gen := NewGenerator(cfg.Seed, cfg.Products)
	owed := 0.0
	last := time.Now().Add(-startEvery)
	startSessions := func(now time.Time) {
		owed = min(owed+float64(cfg.Rate)*now.Sub(last).Seconds(), float64(cfg.Rate))
		last = now
		for owed > 0 {
			events := gen.Session(now)
			owed -= float64(len(events))
			visitors.Add(1)
			go func() {
				defer visitors.Done()
				play(ctx, events, happened)
			}()
		}
	}

	var batch []event.Input
	flush := func() {
		body, err := json.Marshal(map[string]any{"events": batch})
		if err != nil {
			panic(err)
		}
		batch = nil
		select {
		case work <- body:
		case <-ctx.Done():
		}
	}

	starts := time.NewTicker(startEvery)
	defer starts.Stop()
	flushes := time.NewTicker(flushEvery)
	defer flushes.Stop()
	startSessions(time.Now())
	for {
		select {
		case <-ctx.Done():
			close(work)
			workers.Wait()
			visitors.Wait()
			return
		case now := <-starts.C:
			startSessions(now)
		case in := <-happened:
			batch = append(batch, in)
			if len(batch) == cfg.BatchSize {
				flush()
			}
		case <-flushes.C:
			if len(batch) > 0 {
				flush()
			}
		}
	}
}

func play(ctx context.Context, events []event.Input, happened chan<- event.Input) {
	for _, in := range events {
		at, err := time.Parse(time.RFC3339Nano, in.OccurredAt)
		if err != nil {
			panic(err)
		}
		wait := time.NewTimer(time.Until(at))
		select {
		case <-wait.C:
		case <-ctx.Done():
			wait.Stop()
			return
		}
		select {
		case happened <- in:
		case <-ctx.Done():
			return
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
