package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/hmzckd/eventlens/internal/event"
	"github.com/hmzckd/eventlens/internal/ingest"
)

const maxBodyBytes = 1 << 20

type Sink interface {
	Accept(ctx context.Context, events []event.Event) error
}

type eventsRequest struct {
	Events []event.Input `json:"events"`
}

type invalidEvent struct {
	Index   int    `json:"index"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func handleEvents(logger *slog.Logger, writeKey string, sink Sink, now func() time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validWriteKey(r.Header.Get("X-Write-Key"), writeKey) {
			writeProblem(w, http.StatusUnauthorized, "Unauthorized", "missing or wrong X-Write-Key header", nil)
			return
		}

		var req eventsRequest
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeProblem(w, http.StatusRequestEntityTooLarge, "Request too large", "the body must be at most 1 MiB", nil)
				return
			}
			writeProblem(w, http.StatusBadRequest, "Invalid JSON", err.Error(), nil)
			return
		}
		if len(req.Events) == 0 {
			writeProblem(w, http.StatusBadRequest, "No events", `send at least one event in "events"`, nil)
			return
		}
		if len(req.Events) > event.MaxBatch {
			writeProblem(w, http.StatusBadRequest, "Too many events",
				fmt.Sprintf("send at most %d events per request, got %d", event.MaxBatch, len(req.Events)), nil)
			return
		}

		at := now()
		events := make([]event.Event, 0, len(req.Events))
		var invalid []invalidEvent
		for i, in := range req.Events {
			ev, err := event.Parse(in, at)
			if err != nil {
				var fe event.FieldError
				errors.As(err, &fe)
				invalid = append(invalid, invalidEvent{Index: i, Field: fe.Field, Message: fe.Message})
				continue
			}
			events = append(events, ev)
		}
		if len(invalid) > 0 {
			writeProblem(w, http.StatusBadRequest, "Invalid events",
				fmt.Sprintf("%d of %d events are invalid, none were accepted", len(invalid), len(req.Events)), invalid)
			return
		}

		if err := sink.Accept(r.Context(), events); err != nil {
			if errors.Is(err, ingest.ErrQueueFull) {
				writeProblem(w, http.StatusServiceUnavailable, "Queue full", "the collector is busy, try again", nil)
				return
			}
			logger.Error("accept events", "count", len(events), "error", err)
			writeProblem(w, http.StatusServiceUnavailable, "Unavailable", "events could not be accepted, try again", nil)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]int{"accepted": len(events)})
	}
}

func validWriteKey(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
