package ingest

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/hmzckd/eventlens/internal/event"
)

var ErrQueueFull = errors.New("event queue is full")

type Writer interface {
	InsertEvents(ctx context.Context, token string, events []event.Event) error
}

type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type Options struct {
	Capacity      int
	BatchSize     int
	FlushInterval time.Duration
	FlushTimeout  time.Duration
	RetryMin      time.Duration
	RetryMax      time.Duration
	DrainTimeout  time.Duration
	Logger        *slog.Logger
	NewTicker     func(time.Duration) Ticker
}

type Pipeline struct {
	writer  Writer
	opts    Options
	queue   chan event.Event
	queued  atomic.Int64
	dropped atomic.Int64
	giveUp  chan struct{}
}

func New(writer Writer, opts Options) *Pipeline {
	if opts.FlushTimeout == 0 {
		opts.FlushTimeout = 10 * time.Second
	}
	if opts.RetryMin == 0 {
		opts.RetryMin = 100 * time.Millisecond
	}
	if opts.RetryMax == 0 {
		opts.RetryMax = 5 * time.Second
	}
	if opts.DrainTimeout == 0 {
		opts.DrainTimeout = 10 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.NewTicker == nil {
		opts.NewTicker = newRealTicker
	}
	return &Pipeline{
		writer: writer,
		opts:   opts,
		queue:  make(chan event.Event, opts.Capacity),
		giveUp: make(chan struct{}),
	}
}

func (p *Pipeline) Accept(_ context.Context, events []event.Event) error {
	n := int64(len(events))
	if p.queued.Add(n) > int64(p.opts.Capacity) {
		p.queued.Add(-n)
		return ErrQueueFull
	}
	for _, e := range events {
		p.queue <- e
	}
	return nil
}

func (p *Pipeline) Depth() int {
	return int(p.queued.Load())
}

func (p *Pipeline) Capacity() int {
	return p.opts.Capacity
}

func (p *Pipeline) Dropped() int64 {
	return p.dropped.Load()
}

func (p *Pipeline) Run(ctx context.Context) {
	ticker := p.opts.NewTicker(p.opts.FlushInterval)
	defer ticker.Stop()

	go func() {
		<-ctx.Done()
		time.Sleep(p.opts.DrainTimeout)
		close(p.giveUp)
	}()

	batch := make([]event.Event, 0, p.opts.BatchSize)
	for {
		select {
		case e := <-p.queue:
			p.queued.Add(-1)
			batch = append(batch, e)
			if len(batch) >= p.opts.BatchSize {
				batch = p.flush(batch)
			}
		case <-ticker.C():
			if len(batch) > 0 {
				batch = p.flush(batch)
			}
		case <-ctx.Done():
			p.drain(batch)
			return
		}
	}
}

func (p *Pipeline) drain(batch []event.Event) {
	for {
		select {
		case e := <-p.queue:
			p.queued.Add(-1)
			batch = append(batch, e)
			if len(batch) >= p.opts.BatchSize {
				batch = p.flush(batch)
			}
		default:
			if len(batch) > 0 {
				p.flush(batch)
			}
			if lost := p.dropped.Load(); lost > 0 {
				p.opts.Logger.Error("event queue drained with losses", "dropped_events", lost)
			} else {
				p.opts.Logger.Info("event queue drained")
			}
			return
		}
	}
}

func (p *Pipeline) flush(batch []event.Event) []event.Event {
	token := uuid.NewString()
	wait := p.opts.RetryMin
	for attempt := 1; ; attempt++ {
		start := time.Now()
		err := p.write(token, batch)
		if err == nil {
			p.opts.Logger.Debug("flushed", "events", len(batch), "attempt", attempt,
				"duration_ms", time.Since(start).Milliseconds())
			return batch[:0]
		}
		p.opts.Logger.Warn("flush failed, will retry", "events", len(batch), "attempt", attempt,
			"retry_in", wait.String(), "error", err)

		select {
		case <-time.After(wait):
		case <-p.giveUp:
			p.dropped.Add(int64(len(batch)))
			p.opts.Logger.Error("giving up on batch during shutdown", "events", len(batch), "error", err)
			return batch[:0]
		}
		wait = min(wait*2, p.opts.RetryMax)
	}
}

func (p *Pipeline) write(token string, batch []event.Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.opts.FlushTimeout)
	defer cancel()
	return p.writer.InsertEvents(ctx, token, batch)
}

type realTicker struct{ t *time.Ticker }

func newRealTicker(d time.Duration) Ticker { return realTicker{time.NewTicker(d)} }
func (r realTicker) C() <-chan time.Time   { return r.t.C }
func (r realTicker) Stop()                 { r.t.Stop() }
