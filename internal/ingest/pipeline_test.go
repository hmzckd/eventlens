package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/hmzckd/eventlens/internal/chtest"
	"github.com/hmzckd/eventlens/internal/event"
)

type fakeTicker struct{ ch chan time.Time }

func (f *fakeTicker) C() <-chan time.Time { return f.ch }
func (f *fakeTicker) Stop()               {}

type recordingWriter struct {
	mu      sync.Mutex
	batches [][]event.Event
	flushed chan int
}

func newRecordingWriter() *recordingWriter {
	return &recordingWriter{flushed: make(chan int, 100)}
}

func (w *recordingWriter) InsertEvents(_ context.Context, _ string, events []event.Event) error {
	w.mu.Lock()
	w.batches = append(w.batches, append([]event.Event(nil), events...))
	w.mu.Unlock()
	w.flushed <- len(events)
	return nil
}

func (w *recordingWriter) waitFlush(t *testing.T) int {
	t.Helper()
	select {
	case n := <-w.flushed:
		return n
	case <-time.After(2 * time.Second):
		t.Fatal("no flush happened")
		return 0
	}
}

func (w *recordingWriter) assertNoFlush(t *testing.T) {
	t.Helper()
	select {
	case n := <-w.flushed:
		t.Fatalf("unexpected flush of %d events", n)
	case <-time.After(50 * time.Millisecond):
	}
}

type harness struct {
	pipeline *Pipeline
	writer   *recordingWriter
	tick     chan time.Time
	stop     context.CancelFunc
	done     chan struct{}
}

func start(t *testing.T, capacity, batchSize int) *harness {
	t.Helper()
	h := &harness{writer: newRecordingWriter(), tick: make(chan time.Time), done: make(chan struct{})}
	h.pipeline = New(h.writer, Options{
		Capacity:      capacity,
		BatchSize:     batchSize,
		FlushInterval: time.Second,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewTicker:     func(time.Duration) Ticker { return &fakeTicker{ch: h.tick} },
	})
	ctx, cancel := context.WithCancel(context.Background())
	h.stop = cancel
	go func() {
		h.pipeline.Run(ctx)
		close(h.done)
	}()
	t.Cleanup(func() { h.shutdown(t) })
	return h
}

func (h *harness) shutdown(t *testing.T) {
	h.stop()
	select {
	case <-h.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func waitEmpty(t *testing.T, p *Pipeline) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for p.Depth() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("queue still holds %d events", p.Depth())
		}
		time.Sleep(time.Millisecond)
	}
}

func accept(t *testing.T, p *Pipeline, n int) {
	t.Helper()
	if err := p.Accept(context.Background(), chtest.Events(n, time.Now())); err != nil {
		t.Fatalf("Accept(%d): %v", n, err)
	}
}

func TestFlushesWhenTheBatchIsFull(t *testing.T) {
	h := start(t, 100, 3)
	accept(t, h.pipeline, 7)

	if n := h.writer.waitFlush(t); n != 3 {
		t.Fatalf("first flush = %d events, want 3", n)
	}
	if n := h.writer.waitFlush(t); n != 3 {
		t.Fatalf("second flush = %d events, want 3", n)
	}
	h.writer.assertNoFlush(t)

	waitEmpty(t, h.pipeline)
	h.tick <- time.Now()
	if n := h.writer.waitFlush(t); n != 1 {
		t.Fatalf("flush on tick = %d events, want the 1 left over", n)
	}
}

func TestFlushesOnTheTimerEvenIfTheBatchIsSmall(t *testing.T) {
	h := start(t, 100, 1000)
	accept(t, h.pipeline, 2)
	waitEmpty(t, h.pipeline)
	h.writer.assertNoFlush(t)

	h.tick <- time.Now()
	if n := h.writer.waitFlush(t); n != 2 {
		t.Fatalf("flush = %d events, want 2", n)
	}

	h.tick <- time.Now()
	h.writer.assertNoFlush(t)
}

func TestDrainsTheQueueOnShutdown(t *testing.T) {
	h := start(t, 100, 1000)
	accept(t, h.pipeline, 5)
	h.shutdown(t)

	if n := h.writer.waitFlush(t); n != 5 {
		t.Fatalf("flush on shutdown = %d events, want 5", n)
	}
	if h.pipeline.Depth() != 0 {
		t.Fatalf("depth after drain = %d", h.pipeline.Depth())
	}
}

func TestAcceptIsAllOrNothingWhenTheQueueIsFull(t *testing.T) {
	p := New(newRecordingWriter(), Options{Capacity: 5, BatchSize: 5, FlushInterval: time.Second})

	accept(t, p, 3)
	err := p.Accept(context.Background(), chtest.Events(3, time.Now()))
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("err = %v, want ErrQueueFull", err)
	}
	if p.Depth() != 3 {
		t.Fatalf("depth = %d, want 3: a rejected request must not leave events behind", p.Depth())
	}
	accept(t, p, 2)
	if p.Depth() != 5 {
		t.Fatalf("depth = %d, want 5", p.Depth())
	}
}

type flakyWriter struct {
	mu       sync.Mutex
	failures int
	tokens   []string
	written  int
}

func (w *flakyWriter) InsertEvents(_ context.Context, token string, events []event.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.tokens = append(w.tokens, token)
	if w.failures > 0 {
		w.failures--
		return errors.New("clickhouse unavailable")
	}
	w.written += len(events)
	return nil
}

func (w *flakyWriter) setFailures(n int) {
	w.mu.Lock()
	w.failures = n
	w.mu.Unlock()
}

func (w *flakyWriter) snapshot() (int, []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.written, append([]string(nil), w.tokens...)
}

func startFlaky(t *testing.T, w *flakyWriter, capacity, batchSize int, drain time.Duration) (*Pipeline, context.CancelFunc, chan struct{}) {
	t.Helper()
	p := New(w, Options{
		Capacity:      capacity,
		BatchSize:     batchSize,
		FlushInterval: time.Hour,
		RetryMin:      time.Millisecond,
		RetryMax:      5 * time.Millisecond,
		DrainTimeout:  drain,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return p, cancel, done
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRetriesAFailedBatchWithTheSameToken(t *testing.T) {
	w := &flakyWriter{failures: 2}
	p, _, _ := startFlaky(t, w, 100, 5, time.Second)

	accept(t, p, 5)
	eventually(t, "batch written", func() bool { n, _ := w.snapshot(); return n == 5 })

	_, tokens := w.snapshot()
	if len(tokens) != 3 {
		t.Fatalf("attempts = %d, want 3 (2 failures + 1 success)", len(tokens))
	}
	if tokens[0] == "" || tokens[0] != tokens[1] || tokens[1] != tokens[2] {
		t.Fatalf("tokens = %v, every retry must reuse the first token", tokens)
	}
}

func TestQueueFillsUpWhileClickHouseIsDownAndEmptiesAfterwards(t *testing.T) {
	w := &flakyWriter{failures: 1 << 30}
	p, _, _ := startFlaky(t, w, 10, 5, time.Second)

	accept(t, p, 5)
	eventually(t, "flusher took the first batch", func() bool { return p.Depth() == 0 })
	accept(t, p, 10)

	err := p.Accept(context.Background(), chtest.Events(1, time.Now()))
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("err = %v, want ErrQueueFull while ClickHouse is down", err)
	}

	w.setFailures(0)
	eventually(t, "all 15 events written", func() bool { n, _ := w.snapshot(); return n == 15 })
	if p.Dropped() != 0 {
		t.Fatalf("dropped = %d, want 0", p.Dropped())
	}
}

func TestShutdownGivesUpAfterTheDrainTimeout(t *testing.T) {
	w := &flakyWriter{failures: 1 << 30}
	p, cancel, done := startFlaky(t, w, 100, 5, 50*time.Millisecond)

	accept(t, p, 7)
	eventually(t, "flusher took the first batch", func() bool { _, tk := w.snapshot(); return len(tk) > 0 })
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not give up after the drain timeout")
	}
	if p.Dropped() != 7 {
		t.Fatalf("dropped = %d, want 7", p.Dropped())
	}
}
