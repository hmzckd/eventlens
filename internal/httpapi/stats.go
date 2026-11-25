package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hmzckd/eventlens/internal/store"
)

type Stats interface {
	TopProducts(ctx context.Context, since time.Time, limit int) ([]store.ProductStat, error)
	Funnel(ctx context.Context, since time.Time) (store.Funnel, error)
	ActiveSessions(ctx context.Context, since time.Time) (uint64, error)
}

const (
	minWindow = time.Minute
	maxWindow = 30 * 24 * time.Hour
)

func parseWindow(r *http.Request, def string) (string, time.Duration, error) {
	raw := r.URL.Query().Get("window")
	if raw == "" {
		raw = def
	}
	var d time.Duration
	if days, ok := strings.CutSuffix(raw, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return "", 0, fmt.Errorf("window %q: use a duration such as 5m, 1h or 7d", raw)
		}
		d = time.Duration(n) * 24 * time.Hour
	} else {
		var err error
		if d, err = time.ParseDuration(raw); err != nil {
			return "", 0, fmt.Errorf("window %q: use a duration such as 5m, 1h or 7d", raw)
		}
	}
	if d < minWindow || d > maxWindow {
		return "", 0, fmt.Errorf("window %q: must be between 1m and 30d", raw)
	}
	return raw, d, nil
}

func handleTopProducts(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, window, err := parseWindow(r, "1h")
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "Invalid window", err.Error(), nil)
			return
		}
		limit := 10
		if v := r.URL.Query().Get("limit"); v != "" {
			limit, err = strconv.Atoi(v)
			if err != nil || limit < 1 || limit > 100 {
				writeProblem(w, http.StatusBadRequest, "Invalid limit", "limit must be a number from 1 to 100", nil)
				return
			}
		}
		since := opts.Now().Add(-window)
		products, err := opts.Stats.TopProducts(r.Context(), since, limit)
		if err != nil {
			statsFailed(w, opts, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"window":   raw,
			"since":    since.UTC(),
			"products": products,
		})
	}
}

type funnelStep struct {
	Step                   string  `json:"step"`
	Sessions               uint64  `json:"sessions"`
	ConversionFromPrevious float64 `json:"conversion_from_previous"`
	ConversionFromStart    float64 `json:"conversion_from_start"`
}

func handleFunnel(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, window, err := parseWindow(r, "24h")
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "Invalid window", err.Error(), nil)
			return
		}
		since := opts.Now().Add(-window)
		f, err := opts.Stats.Funnel(r.Context(), since)
		if err != nil {
			statsFailed(w, opts, err)
			return
		}
		counts := []uint64{f.Viewed, f.AddedToCart, f.StartedCheckout, f.Purchased}
		names := []string{"product_view", "add_to_cart", "checkout_start", "purchase"}
		steps := make([]funnelStep, len(counts))
		for i, n := range counts {
			steps[i] = funnelStep{Step: names[i], Sessions: n, ConversionFromPrevious: 1, ConversionFromStart: 1}
			if i > 0 {
				steps[i].ConversionFromPrevious = ratio(n, counts[i-1])
				steps[i].ConversionFromStart = ratio(n, counts[0])
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"window": raw, "since": since.UTC(), "steps": steps})
	}
}

func handleActiveUsers(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, window, err := parseWindow(r, "5m")
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "Invalid window", err.Error(), nil)
			return
		}
		since := opts.Now().Add(-window)
		n, err := opts.Stats.ActiveSessions(r.Context(), since)
		if err != nil {
			statsFailed(w, opts, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"window": raw, "since": since.UTC(), "active_sessions": n})
	}
}

func ratio(part, whole uint64) float64 {
	if whole == 0 {
		return 0
	}
	return float64(int(float64(part)/float64(whole)*10000+0.5)) / 10000
}

func statsFailed(w http.ResponseWriter, opts Options, err error) {
	opts.Logger.Error("stats query", "error", err)
	writeProblem(w, http.StatusServiceUnavailable, "Stats unavailable", "the query failed, try again", nil)
}
