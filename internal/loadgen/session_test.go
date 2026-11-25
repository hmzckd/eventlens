package loadgen

import (
	"testing"
	"time"

	"github.com/hmzckd/eventlens/internal/event"
)

func TestEverySessionIsValidAndInFunnelOrder(t *testing.T) {
	g := NewGenerator(1, 200)
	now := time.Now()

	for i := 0; i < 2000; i++ {
		events := g.Session(now)
		if events[0].Type != event.PageView {
			t.Fatalf("session %d starts with %s, want page_view", i, events[0].Type)
		}
		viewed := map[string]bool{}
		carted := map[string]bool{}
		checkedOut := map[string]bool{}
		var last time.Time
		for _, in := range events {
			ev, err := event.Parse(in, now)
			if err != nil {
				t.Fatalf("session %d: invalid event %+v: %v", i, in, err)
			}
			if !ev.OccurredAt.After(last) {
				t.Fatalf("session %d: events not in time order", i)
			}
			last = ev.OccurredAt
			if ev.SessionID != events[0].SessionID {
				t.Fatalf("session %d: mixed session ids", i)
			}
			switch ev.Type {
			case event.ProductView:
				viewed[ev.ProductID] = true
			case event.AddToCart:
				if !viewed[ev.ProductID] {
					t.Fatalf("session %d: cart before view", i)
				}
				carted[ev.ProductID] = true
			case event.CheckoutStart:
				if !carted[ev.ProductID] {
					t.Fatalf("session %d: checkout before cart", i)
				}
				checkedOut[ev.ProductID] = true
			case event.Purchase:
				if !checkedOut[ev.ProductID] {
					t.Fatalf("session %d: purchase before checkout", i)
				}
			}
		}
	}
}

func TestFunnelNarrowsAndSomeProductsArePopular(t *testing.T) {
	g := NewGenerator(2, 200)
	counts := map[string]int{}
	views := map[string]int{}
	for i := 0; i < 10_000; i++ {
		for _, in := range g.Session(time.Now()) {
			counts[in.Type]++
			if in.Type == event.ProductView {
				views[in.ProductID]++
			}
		}
	}

	order := []string{event.PageView, event.ProductView, event.AddToCart, event.CheckoutStart, event.Purchase}
	for i := 1; i < len(order); i++ {
		if counts[order[i]] == 0 || counts[order[i]] >= counts[order[i-1]] && order[i-1] != event.PageView {
			t.Fatalf("funnel does not narrow: %v", counts)
		}
	}
	if counts[event.PageView] != 10_000 {
		t.Fatalf("page views = %d, want one per session", counts[event.PageView])
	}

	top := views["p-001"]
	median := views["p-100"]
	if top < 10*median {
		t.Fatalf("p-001 has %d views and p-100 has %d: the most popular product should lead by far", top, median)
	}
}

func TestTheSameSeedGivesTheSameCatalog(t *testing.T) {
	a, b := NewGenerator(7, 50).Products(), NewGenerator(7, 50).Products()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("product %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}
