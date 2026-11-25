package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hmzckd/eventlens/internal/config"
	"github.com/hmzckd/eventlens/internal/httpapi"
	"github.com/hmzckd/eventlens/internal/store"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("eventlens stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, store.Options{
		Addr:     cfg.ClickHouseAddr,
		Database: cfg.ClickHouseDatabase,
		Username: cfg.ClickHouseUser,
		Password: cfg.ClickHousePassword,
	})
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return err
	}
	logger.Info("clickhouse ready", "addr", cfg.ClickHouseAddr, "database", cfg.ClickHouseDatabase)

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}

	srv := &http.Server{
		Handler:           httpapi.NewHandler(httpapi.Options{Logger: logger, DB: db}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return serve(ctx, logger, srv, listener)
}

func serve(ctx context.Context, logger *slog.Logger, srv *http.Server, listener net.Listener) error {
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("eventlens started", "addr", listener.Addr().String())
		serveErr <- srv.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down", "timeout", shutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}
	logger.Info("eventlens stopped cleanly")
	return nil
}
