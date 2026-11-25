package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSuccessfulRequestsAreLoggedOnlyAtDebugLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	h := NewHandler(Options{Logger: logger, DB: fakePinger{}, WriteKey: testWriteKey, Sink: &recordingSink{}})

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	if buf.Len() != 0 {
		t.Fatalf("a 200 was logged at info level: %s", buf.String())
	}

	postEvents(t, h, "wrong", batch(1))
	if !strings.Contains(buf.String(), "status=401") {
		t.Fatalf("a 401 should be logged at info level, log: %q", buf.String())
	}
}
