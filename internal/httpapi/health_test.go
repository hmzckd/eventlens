package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func getHealth(t *testing.T, db Pinger) (int, map[string]string) {
	t.Helper()
	handler := NewHandler(Options{Logger: discardLogger(), DB: db})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return rec.Code, body
}

func TestHealthReturnsOKWhenClickHouseAnswers(t *testing.T) {
	status, body := getHealth(t, fakePinger{})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if body["status"] != "ok" || body["clickhouse"] != "ok" {
		t.Fatalf("body = %v", body)
	}
}

func TestHealthReturns503WhenClickHouseIsDown(t *testing.T) {
	status, body := getHealth(t, fakePinger{err: errors.New("connection refused")})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
	if body["status"] != "unavailable" || body["clickhouse"] != "connection refused" {
		t.Fatalf("body = %v", body)
	}
}
