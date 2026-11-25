package httpapi

import (
	"context"
	"log/slog"
	"net/http"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type Options struct {
	Logger *slog.Logger
	DB     Pinger
}

func NewHandler(opts Options) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", handleHealth(opts.DB))
	return logRequests(opts.Logger, mux)
}
