package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/hmzckd/eventlens/internal/chtest"
	"github.com/hmzckd/eventlens/internal/event"
)

func TestMain(m *testing.M) {
	os.Exit(chtest.Run(m))
}

func TestBatchingTurnsManyRequestsIntoFewParts(t *testing.T) {
	ctx := context.Background()
	s, conn := chtest.Store(t)
	if err := conn.Exec(ctx, "SYSTEM STOP MERGES events"); err != nil {
		t.Fatal(err)
	}

	p := New(s, Options{
		Capacity:      50_000,
		BatchSize:     10_000,
		FlushInterval: time.Hour,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { p.Run(runCtx); close(done) }()

	for i := 0; i < 50; i++ {
		if err := p.Accept(ctx, chtest.Events(500, time.Now())); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	stop()
	<-done

	var rows, parts uint64
	if err := conn.QueryRow(ctx, "SELECT count() FROM events").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `
		SELECT count() FROM system.parts
		WHERE database = currentDatabase() AND table = 'events' AND active`).Scan(&parts); err != nil {
		t.Fatal(err)
	}
	if rows != 25_000 {
		t.Fatalf("rows = %d, want 25000", rows)
	}
	if parts != 3 {
		t.Fatalf("parts = %d, want 3 (10000 + 10000 + 5000) for 50 requests", parts)
	}
}

type lostAckWriter struct {
	inner Writer
	calls int
}

func (w *lostAckWriter) InsertEvents(ctx context.Context, token string, events []event.Event) error {
	w.calls++
	if err := w.inner.InsertEvents(ctx, token, events); err != nil {
		return err
	}
	if w.calls == 1 {
		return errors.New("connection reset after the insert reached the server")
	}
	return nil
}

func TestARetryAfterALostAnswerDoesNotDuplicateEvents(t *testing.T) {
	ctx := context.Background()
	s, conn := chtest.Store(t)
	w := &lostAckWriter{inner: s}

	p := New(w, Options{
		Capacity:      1000,
		BatchSize:     100,
		FlushInterval: time.Hour,
		RetryMin:      time.Millisecond,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { p.Run(runCtx); close(done) }()

	if err := p.Accept(ctx, chtest.Events(100, time.Now())); err != nil {
		t.Fatal(err)
	}
	stop()
	<-done

	if w.calls != 2 {
		t.Fatalf("insert calls = %d, want 2 (first answer lost, one retry)", w.calls)
	}
	var rows uint64
	if err := conn.QueryRow(ctx, "SELECT count() FROM events").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 100 {
		t.Fatalf("rows = %d, want 100: the retry must be deduplicated by its token", rows)
	}
}
