package metrics

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeQueue struct{}

func (fakeQueue) Depth() int     { return 42 }
func (fakeQueue) Capacity() int  { return 50000 }
func (fakeQueue) Dropped() int64 { return 3 }

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	b, _ := io.ReadAll(rec.Body)
	return string(b)
}

func TestMetricsAreExposed(t *testing.T) {
	m := New()
	m.WatchQueue(fakeQueue{})
	m.EventsAccepted(500)
	m.RequestRejected("queue_full")
	m.FlushAttempt(500, 20*time.Millisecond, nil)
	m.FlushAttempt(500, time.Second, errors.New("down"))

	out := scrape(t, m)
	for _, want := range []string{
		"eventlens_events_accepted_total 500",
		`eventlens_requests_rejected_total{reason="queue_full"} 1`,
		`eventlens_requests_rejected_total{reason="invalid"} 0`,
		"eventlens_events_written_total 500",
		"eventlens_flush_failures_total 1",
		"eventlens_queue_depth 42",
		"eventlens_queue_capacity 50000",
		"eventlens_events_dropped_total 3",
		`eventlens_flush_duration_seconds_count{result="error"} 1`,
		"eventlens_flush_batch_size_sum 500",
		"go_goroutines",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics output is missing %q", want)
		}
	}
}
