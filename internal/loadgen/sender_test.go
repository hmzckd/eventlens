package loadgen

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hmzckd/eventlens/internal/event"
)

func TestRunRetriesTheSameBodyAfterA503(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	accepted := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Write-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var req struct {
			Events []json.RawMessage `json:"events"`
		}
		_ = json.Unmarshal(b, &req)
		accepted += len(req.Events)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	var stats Stats
	Run(ctx, Config{URL: srv.URL, WriteKey: "k", Rate: 5000, BatchSize: 20, Workers: 1, Seed: 1, Products: 50}, &stats)

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) < 3 {
		t.Fatalf("only %d requests reached the server", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Fatal("the request after a 503 must resend the same events")
	}
	if stats.Busy.Load() != 1 {
		t.Fatalf("busy = %d, want 1", stats.Busy.Load())
	}
	if got := stats.Accepted.Load(); got != int64(accepted) || got == 0 {
		t.Fatalf("stats say %d accepted, server accepted %d", got, accepted)
	}
	if stats.Failed.Load() != 0 {
		t.Fatalf("failed = %d", stats.Failed.Load())
	}
}

func TestRunCountsRejectedBatchesAsFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var stats Stats
	Run(ctx, Config{URL: srv.URL, WriteKey: "wrong", Rate: 5000, BatchSize: 10, Workers: 2, Seed: 1, Products: 50}, &stats)

	if stats.Failed.Load() == 0 || stats.Accepted.Load() != 0 {
		t.Fatalf("failed = %d, accepted = %d", stats.Failed.Load(), stats.Accepted.Load())
	}
}

func TestRunSendsEachEventWhenItHappens(t *testing.T) {
	var mu sync.Mutex
	var delays []time.Duration
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received := time.Now()
		var req struct {
			Events []event.Input `json:"events"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		defer mu.Unlock()
		for _, in := range req.Events {
			at, err := time.Parse(time.RFC3339Nano, in.OccurredAt)
			if err != nil {
				t.Errorf("bad occurred_at %q", in.OccurredAt)
				continue
			}
			delays = append(delays, received.Sub(at))
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	var stats Stats
	Run(ctx, Config{URL: srv.URL, WriteKey: "k", Rate: 2000, BatchSize: 50, Workers: 2, Seed: 1, Products: 50}, &stats)

	mu.Lock()
	defer mu.Unlock()
	if len(delays) < 100 {
		t.Fatalf("only %d events were sent", len(delays))
	}
	onTime := 0
	for _, d := range delays {
		if d < -50*time.Millisecond {
			t.Fatalf("an event was sent %s before it happened", -d)
		}
		if d > 2*time.Second {
			t.Fatalf("an event was sent %s after it happened", d)
		}
		if d < 500*time.Millisecond {
			onTime++
		}
	}
	if onTime < len(delays)*9/10 {
		t.Fatalf("only %d of %d events arrived within half a second", onTime, len(delays))
	}
}
