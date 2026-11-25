package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type Recorder interface {
	EventsAccepted(n int)
	RequestRejected(reason string)
}

type noopRecorder struct{}

func (noopRecorder) EventsAccepted(int)     {}
func (noopRecorder) RequestRejected(string) {}

type Options struct {
	Logger   *slog.Logger
	DB       Pinger
	WriteKey string
	Sink     Sink
	Now      func() time.Time
	Recorder Recorder
	Metrics  http.Handler
}

func NewHandler(opts Options) http.Handler {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Recorder == nil {
		opts.Recorder = noopRecorder{}
	}
	mux := http.NewServeMux()
	mux.Handle("GET /health", handleHealth(opts.DB))
	mux.Handle("POST /v1/events", handleEvents(opts))
	if opts.Metrics != nil {
		mux.Handle("GET /metrics", opts.Metrics)
	}
	return logRequests(opts.Logger, mux)
}
