package ingest

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/hmzckd/eventlens/internal/event"
)

var ErrQueueFull = errors.New("event queue is full")

type Writer interface {
	InsertEvents(ctx context.Context, events []event.Event) error
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
	Logger        *slog.Logger
	NewTicker     func(time.Duration) Ticker
}

type Pipeline struct {
	writer Writer
	opts   Options
	queue  chan event.Event
	queued atomic.Int64
}

func New(writer Writer, opts Options) *Pipeline {
	if opts.FlushTimeout == 0 {
		opts.FlushTimeout = 10 * time.Second
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

func (p *Pipeline) Run(ctx context.Context) {
	ticker := p.opts.NewTicker(p.opts.FlushInterval)
	defer ticker.Stop()

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
			p.opts.Logger.Info("event queue drained")
			return
		}
	}
}

func (p *Pipeline) flush(batch []event.Event) []event.Event {
	ctx, cancel := context.WithTimeout(context.Background(), p.opts.FlushTimeout)
	defer cancel()

	start := time.Now()
	if err := p.writer.InsertEvents(ctx, batch); err != nil {
		p.opts.Logger.Error("flush failed, batch dropped", "events", len(batch), "error", err)
	} else {
		p.opts.Logger.Debug("flushed", "events", len(batch), "duration_ms", time.Since(start).Milliseconds())
	}
	return batch[:0]
}

type realTicker struct{ t *time.Ticker }

func newRealTicker(d time.Duration) Ticker { return realTicker{time.NewTicker(d)} }
func (r realTicker) C() <-chan time.Time   { return r.t.C }
func (r realTicker) Stop()                 { r.t.Stop() }
