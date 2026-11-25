package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hmzckd/eventlens/internal/store"
)

type fakeStats struct {
	since time.Time
	limit int
	err   error
}

func (f *fakeStats) TopProducts(_ context.Context, since time.Time, limit int) ([]store.ProductStat, error) {
	f.since, f.limit = since, limit
	return []store.ProductStat{{ProductID: "p-1", Views: 30, Visitors: 12}}, f.err
}

func (f *fakeStats) Funnel(_ context.Context, since time.Time) (store.Funnel, error) {
	f.since = since
	return store.Funnel{Viewed: 1000, AddedToCart: 250, StartedCheckout: 100, Purchased: 0}, f.err
}

func (f *fakeStats) ActiveSessions(_ context.Context, since time.Time) (uint64, error) {
	f.since = since
	return 77, f.err
}

func getStats(t *testing.T, stats Stats, url string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	h := NewHandler(Options{
		Logger: discardLogger(), DB: fakePinger{}, WriteKey: testWriteKey, Sink: &recordingSink{},
		Stats: stats, Now: func() time.Time { return testNow },
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestTopProductsUsesWindowAndLimit(t *testing.T) {
	stats := &fakeStats{}
	rec, body := getStats(t, stats, "/v1/stats/top-products?window=2h&limit=5")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if !stats.since.Equal(testNow.Add(-2*time.Hour)) || stats.limit != 5 {
		t.Fatalf("since = %s, limit = %d", stats.since, stats.limit)
	}
	products := body["products"].([]any)
	first := products[0].(map[string]any)
	if first["product_id"] != "p-1" || first["views"] != float64(30) || first["visitors"] != float64(12) {
		t.Fatalf("products = %v", products)
	}
}

func TestStatsDefaults(t *testing.T) {
	cases := map[string]time.Duration{
		"/v1/stats/top-products": time.Hour,
		"/v1/stats/funnel":       24 * time.Hour,
		"/v1/stats/active-users": 5 * time.Minute,
	}
	for url, window := range cases {
		stats := &fakeStats{}
		rec, _ := getStats(t, stats, url)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", url, rec.Code)
		}
		if !stats.since.Equal(testNow.Add(-window)) {
			t.Errorf("%s: since = %s, want now - %s", url, stats.since, window)
		}
	}
}

func TestWindowAcceptsDays(t *testing.T) {
	stats := &fakeStats{}
	rec, _ := getStats(t, stats, "/v1/stats/funnel?window=7d")
	if rec.Code != http.StatusOK || !stats.since.Equal(testNow.Add(-7*24*time.Hour)) {
		t.Fatalf("status = %d, since = %s", rec.Code, stats.since)
	}
}

func TestStatsRejectBadParameters(t *testing.T) {
	for _, url := range []string{
		"/v1/stats/top-products?window=forever",
		"/v1/stats/top-products?window=10s",
		"/v1/stats/top-products?window=31d",
		"/v1/stats/top-products?limit=0",
		"/v1/stats/top-products?limit=101",
		"/v1/stats/funnel?window=xd",
	} {
		rec, _ := getStats(t, &fakeStats{}, url)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", url, rec.Code)
		}
	}
}

func TestFunnelComputesConversionRates(t *testing.T) {
	_, body := getStats(t, &fakeStats{}, "/v1/stats/funnel")
	steps := body["steps"].([]any)
	want := []struct {
		step               string
		sessions, fromPrev float64
	}{
		{"product_view", 1000, 1},
		{"add_to_cart", 250, 0.25},
		{"checkout_start", 100, 0.4},
		{"purchase", 0, 0},
	}
	for i, w := range want {
		s := steps[i].(map[string]any)
		if s["step"] != w.step || s["sessions"] != w.sessions || s["conversion_from_previous"] != w.fromPrev {
			t.Errorf("step %d = %v, want %+v", i, s, w)
		}
	}
	if got := steps[2].(map[string]any)["conversion_from_start"]; got != 0.1 {
		t.Errorf("checkout from start = %v, want 0.1", got)
	}
}

func TestActiveUsers(t *testing.T) {
	_, body := getStats(t, &fakeStats{}, "/v1/stats/active-users?window=15m")
	if body["active_sessions"] != float64(77) || body["window"] != "15m" {
		t.Fatalf("body = %v", body)
	}
}

func TestStatsReturn503WhenTheQueryFails(t *testing.T) {
	rec, _ := getStats(t, &fakeStats{err: errors.New("timeout")}, "/v1/stats/funnel")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
