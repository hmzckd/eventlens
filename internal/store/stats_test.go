package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hmzckd/eventlens/internal/chtest"
	"github.com/hmzckd/eventlens/internal/event"
	"github.com/hmzckd/eventlens/internal/store"
)

type step struct {
	session, typ, product string
	ago                   time.Duration
}

func insertSteps(t *testing.T, s *store.Store, now time.Time, steps []step) {
	t.Helper()
	var events []event.Event
	for _, st := range steps {
		e := event.Event{
			EventID:    uuid.New(),
			Type:       st.typ,
			OccurredAt: now.Add(-st.ago),
			ReceivedAt: now,
			SessionID:  st.session,
			ProductID:  st.product,
		}
		if st.typ == event.Purchase {
			e.Price = 1000
		}
		events = append(events, e)
	}
	if err := s.InsertEvents(context.Background(), "", events); err != nil {
		t.Fatal(err)
	}
}

func TestTopProductsCountsViewsAndVisitorsInTheWindow(t *testing.T) {
	ctx := context.Background()
	s, _ := chtest.Store(t)
	now := time.Now().UTC()

	insertSteps(t, s, now, []step{
		{"s1", event.ProductView, "p-1", 10 * time.Minute},
		{"s1", event.ProductView, "p-1", 9 * time.Minute},
		{"s2", event.ProductView, "p-1", 8 * time.Minute},
		{"s2", event.ProductView, "p-2", 7 * time.Minute},
		{"s3", event.AddToCart, "p-2", 6 * time.Minute},
		{"s4", event.ProductView, "p-3", 2 * time.Hour},
	})

	got, err := s.TopProducts(ctx, now.Add(-time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []store.ProductStat{
		{ProductID: "p-1", Views: 3, Visitors: 2},
		{ProductID: "p-2", Views: 1, Visitors: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	one, err := s.TopProducts(ctx, now.Add(-time.Hour), 1)
	if err != nil || len(one) != 1 || one[0].ProductID != "p-1" {
		t.Fatalf("limit 1: %+v, %v", one, err)
	}
}

func TestFunnelFollowsTheStepsInOrderPerSession(t *testing.T) {
	ctx := context.Background()
	s, _ := chtest.Store(t)
	now := time.Now().UTC()

	insertSteps(t, s, now, []step{
		{"viewer", event.ProductView, "p-1", 30 * time.Minute},

		{"carter", event.ProductView, "p-1", 30 * time.Minute},
		{"carter", event.AddToCart, "p-1", 29 * time.Minute},

		{"buyer", event.ProductView, "p-1", 30 * time.Minute},
		{"buyer", event.AddToCart, "p-1", 29 * time.Minute},
		{"buyer", event.CheckoutStart, "p-1", 28 * time.Minute},
		{"buyer", event.Purchase, "p-1", 27 * time.Minute},

		{"cart-only", event.AddToCart, "p-1", 30 * time.Minute},

		{"wrong-order", event.Purchase, "p-1", 30 * time.Minute},
		{"wrong-order", event.ProductView, "p-1", 29 * time.Minute},

		{"old", event.ProductView, "p-1", 3 * time.Hour},
	})

	got, err := s.Funnel(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := store.Funnel{Viewed: 4, AddedToCart: 2, StartedCheckout: 1, Purchased: 1}
	if got != want {
		t.Fatalf("funnel = %+v, want %+v", got, want)
	}
}

func TestActiveSessionsCountsEachSessionOnce(t *testing.T) {
	ctx := context.Background()
	s, _ := chtest.Store(t)
	now := time.Now().UTC()

	insertSteps(t, s, now, []step{
		{"s1", event.PageView, "", time.Minute},
		{"s1", event.ProductView, "p-1", 30 * time.Second},
		{"s2", event.PageView, "", 2 * time.Minute},
		{"s3", event.PageView, "", 10 * time.Minute},
	})

	got, err := s.ActiveSessions(ctx, now.Add(-5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("active sessions = %d, want 2", got)
	}
}
