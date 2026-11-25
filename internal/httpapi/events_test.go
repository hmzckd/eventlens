package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hmzckd/eventlens/internal/event"
	"github.com/hmzckd/eventlens/internal/ingest"
)

const testWriteKey = "test-write-key"

var testNow = time.Date(2026, 11, 25, 18, 0, 0, 0, time.UTC)

type recordingSink struct {
	got [][]event.Event
	err error
}

func (s *recordingSink) Accept(_ context.Context, events []event.Event) error {
	if s.err != nil {
		return s.err
	}
	s.got = append(s.got, events)
	return nil
}

func newEventsHandler(sink Sink) http.Handler {
	return NewHandler(Options{
		Logger:   discardLogger(),
		DB:       fakePinger{},
		WriteKey: testWriteKey,
		Sink:     sink,
		Now:      func() time.Time { return testNow },
	})
}

func eventJSON(i int) map[string]any {
	return map[string]any{
		"event_id":    fmt.Sprintf("0193b4c2-7d1e-7a3b-9c4d-%012d", i),
		"type":        "product_view",
		"occurred_at": "2026-11-25T17:59:00Z",
		"session_id":  "s-1",
		"product_id":  "p-1",
	}
}

func postEvents(t *testing.T, h http.Handler, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	switch b := body.(type) {
	case string:
		buf.WriteString(b)
	default:
		if err := json.NewEncoder(&buf).Encode(b); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/events", &buf)
	if key != "" {
		req.Header.Set("X-Write-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func batch(n int) map[string]any {
	events := make([]map[string]any, n)
	for i := range events {
		events[i] = eventJSON(i)
	}
	return map[string]any{"events": events}
}

func TestPostEventsHandsValidEventsToTheSink(t *testing.T) {
	sink := &recordingSink{}
	rec := postEvents(t, newEventsHandler(sink), testWriteKey, batch(3))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"accepted":3`) {
		t.Fatalf("body = %s", rec.Body)
	}
	if len(sink.got) != 1 || len(sink.got[0]) != 3 {
		t.Fatalf("sink got %d calls", len(sink.got))
	}
	if !sink.got[0][0].ReceivedAt.Equal(testNow) {
		t.Fatalf("ReceivedAt = %s, want server time", sink.got[0][0].ReceivedAt)
	}
}

func TestPostEventsNeedsTheWriteKey(t *testing.T) {
	for _, key := range []string{"", "wrong-key"} {
		sink := &recordingSink{}
		rec := postEvents(t, newEventsHandler(sink), key, batch(1))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("key %q: status = %d, want 401", key, rec.Code)
		}
		if len(sink.got) != 0 {
			t.Errorf("key %q: events reached the sink", key)
		}
	}
}

func TestPostEventsRejectsTheWholeRequestIfOneEventIsInvalid(t *testing.T) {
	body := batch(3)
	events := body["events"].([]map[string]any)
	events[1]["type"] = "click"

	sink := &recordingSink{}
	rec := postEvents(t, newEventsHandler(sink), testWriteKey, body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	var p struct {
		Errors []invalidEvent `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if len(p.Errors) != 1 || p.Errors[0].Index != 1 || p.Errors[0].Field != "type" {
		t.Fatalf("errors = %+v, want index 1 field type", p.Errors)
	}
	if len(sink.got) != 0 {
		t.Fatal("no event may be accepted when one is invalid")
	}
}

func TestPostEventsChecksTheBatchSize(t *testing.T) {
	cases := map[string]any{
		"empty":    batch(0),
		"over 500": batch(event.MaxBatch + 1),
		"bad json": `{"events": [`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := postEvents(t, newEventsHandler(&recordingSink{}), testWriteKey, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
	rec := postEvents(t, newEventsHandler(&recordingSink{}), testWriteKey, batch(event.MaxBatch))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("exactly %d events: status = %d", event.MaxBatch, rec.Code)
	}
}

func TestPostEventsRejectsHugeBodies(t *testing.T) {
	body := `{"events":[{"url":"` + strings.Repeat("x", maxBodyBytes) + `"}]}`
	rec := postEvents(t, newEventsHandler(&recordingSink{}), testWriteKey, body)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestPostEventsAsksTheClientToRetryWhenTheQueueIsFull(t *testing.T) {
	rec := postEvents(t, newEventsHandler(&recordingSink{err: ingest.ErrQueueFull}), testWriteKey, batch(1))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
}

func TestPostEventsReturns503WhenTheSinkFails(t *testing.T) {
	for _, err := range []error{ingest.ErrQueueFull, errors.New("something else")} {
		rec := postEvents(t, newEventsHandler(&recordingSink{err: err}), testWriteKey, batch(1))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%v: status = %d, want 503", err, rec.Code)
		}
	}
}
