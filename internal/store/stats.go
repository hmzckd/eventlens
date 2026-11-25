package store

import (
	"context"
	"fmt"
	"time"
)

type ProductStat struct {
	ProductID string `json:"product_id"`
	Views     uint64 `json:"views"`
	Visitors  uint64 `json:"visitors"`
}

type Funnel struct {
	Viewed          uint64
	AddedToCart     uint64
	StartedCheckout uint64
	Purchased       uint64
}

const funnelStepWindow = time.Hour

func (s *Store) TopProducts(ctx context.Context, since time.Time, limit int) ([]ProductStat, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT product_id, countMerge(views) AS total_views, uniqMerge(visitors) AS total_visitors
		FROM product_views_1m
		WHERE minute >= toStartOfMinute(?)
		GROUP BY product_id
		ORDER BY total_views DESC, product_id
		LIMIT ?`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("top products: %w", err)
	}
	defer rows.Close()

	out := []ProductStat{}
	for rows.Next() {
		var p ProductStat
		if err := rows.Scan(&p.ProductID, &p.Views, &p.Visitors); err != nil {
			return nil, fmt.Errorf("top products: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Funnel(ctx context.Context, since time.Time) (Funnel, error) {
	var f Funnel
	err := s.conn.QueryRow(ctx, `
		SELECT
			countIf(level >= 1),
			countIf(level >= 2),
			countIf(level >= 3),
			countIf(level >= 4)
		FROM (
			SELECT
				session_id,
				windowFunnel(?)(
					toDateTime(occurred_at),
					event_type = 'product_view',
					event_type = 'add_to_cart',
					event_type = 'checkout_start',
					event_type = 'purchase'
				) AS level
			FROM events
			WHERE occurred_at >= ?
			  AND event_type IN ('product_view', 'add_to_cart', 'checkout_start', 'purchase')
			GROUP BY session_id
		)`, uint64(funnelStepWindow.Seconds()), since).Scan(&f.Viewed, &f.AddedToCart, &f.StartedCheckout, &f.Purchased)
	if err != nil {
		return Funnel{}, fmt.Errorf("funnel: %w", err)
	}
	return f, nil
}

func (s *Store) ActiveSessions(ctx context.Context, since time.Time) (uint64, error) {
	var n uint64
	err := s.conn.QueryRow(ctx, `
		SELECT uniq(session_id) FROM events WHERE occurred_at >= ?`, since).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("active sessions: %w", err)
	}
	return n, nil
}
