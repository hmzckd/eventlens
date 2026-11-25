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

type Options struct {
	Logger   *slog.Logger
	DB       Pinger
	WriteKey string
	Sink     Sink
	Now      func() time.Time
}

func NewHandler(opts Options) http.Handler {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	mux := http.NewServeMux()
	mux.Handle("GET /health", handleHealth(opts.DB))
	mux.Handle("POST /v1/events", handleEvents(opts.Logger, opts.WriteKey, opts.Sink, opts.Now))
	return logRequests(opts.Logger, mux)
}
