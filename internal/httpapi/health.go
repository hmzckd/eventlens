package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func handleHealth(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status":     "unavailable",
				"clickhouse": err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "clickhouse": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
