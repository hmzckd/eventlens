package store

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/hmzckd/eventlens/internal/event"
)

func (s *Store) InsertEvents(ctx context.Context, token string, events []event.Event) error {
	if len(events) == 0 {
		return nil
	}
	if token != "" {
		ctx = clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
			"insert_deduplication_token":                         token,
			"deduplicate_blocks_in_dependent_materialized_views": 1,
		}))
	}
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO events")
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	for _, e := range events {
		err := batch.Append(
			e.EventID,
			e.Type,
			e.OccurredAt,
			e.ReceivedAt,
			e.SessionID,
			e.UserID,
			e.ProductID,
			e.Category,
			e.Price,
			e.Country,
			e.Device,
			e.URL,
		)
		if err != nil {
			_ = batch.Abort()
			return fmt.Errorf("append event %s: %w", e.EventID, err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("insert %d events: %w", len(events), err)
	}
	return nil
}
