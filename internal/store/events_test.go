package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hmzckd/eventlens/internal/chtest"
	"github.com/hmzckd/eventlens/internal/event"
)

func TestInsertEventsStoresEveryColumn(t *testing.T) {
	ctx := context.Background()
	s, conn := chtest.Store(t)

	at := time.Now().UTC().Truncate(time.Millisecond)
	in := event.Event{
		EventID:    uuid.New(),
		Type:       event.Purchase,
		OccurredAt: at.Add(-time.Minute),
		ReceivedAt: at,
		SessionID:  "s-1",
		UserID:     "u-7",
		ProductID:  "p-42",
		Category:   "shoes",
		Price:      129_900,
		Country:    "TR",
		Device:     "desktop",
		URL:        "/checkout",
	}
	if err := s.InsertEvents(ctx, []event.Event{in}); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	var got event.Event
	err := conn.QueryRow(ctx, `
		SELECT event_id, event_type, occurred_at, received_at, session_id, user_id,
		       product_id, category, price, country, device, url
		FROM events`).Scan(
		&got.EventID, &got.Type, &got.OccurredAt, &got.ReceivedAt, &got.SessionID, &got.UserID,
		&got.ProductID, &got.Category, &got.Price, &got.Country, &got.Device, &got.URL,
	)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got.OccurredAt = got.OccurredAt.UTC()
	got.ReceivedAt = got.ReceivedAt.UTC()
	if got != in {
		t.Fatalf("read back\n got %+v\nwant %+v", got, in)
	}
}

func TestEveryInsertCreatesANewPart(t *testing.T) {
	ctx := context.Background()
	s, conn := chtest.Store(t)

	if err := conn.Exec(ctx, "SYSTEM STOP MERGES events"); err != nil {
		t.Fatalf("stop merges: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := s.InsertEvents(ctx, chtest.Events(1, time.Now())); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	var parts uint64
	err := conn.QueryRow(ctx, `
		SELECT count() FROM system.parts
		WHERE database = currentDatabase() AND table = 'events' AND active`).Scan(&parts)
	if err != nil {
		t.Fatalf("count parts: %v", err)
	}
	if parts != 3 {
		t.Fatalf("active parts = %d, want 3 (one per insert)", parts)
	}
}
