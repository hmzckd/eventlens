package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/hmzckd/eventlens/internal/chtest"
	"github.com/hmzckd/eventlens/internal/store"
)

func rollupViews(t *testing.T, conn driver.Conn) uint64 {
	t.Helper()
	var n uint64
	if err := conn.QueryRow(context.Background(),
		"SELECT countMerge(views) FROM product_views_1m").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTheRollupFollowsEveryInsert(t *testing.T) {
	ctx := context.Background()
	s, conn := chtest.Store(t)

	for i := 0; i < 3; i++ {
		if err := s.InsertEvents(ctx, "", chtest.Events(1000, time.Now())); err != nil {
			t.Fatal(err)
		}
	}
	if got := rollupViews(t, conn); got != 3000 {
		t.Fatalf("rollup views = %d, want 3000", got)
	}

	var rawRows, rollupRows uint64
	if err := conn.QueryRow(ctx, "SELECT count() FROM events").Scan(&rawRows); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, "SELECT count() FROM product_views_1m FINAL").Scan(&rollupRows); err != nil {
		t.Fatal(err)
	}
	if rollupRows > 20 {
		t.Fatalf("rollup has %d rows for %d raw rows, want at most a few rows per product and minute", rollupRows, rawRows)
	}
}

func TestADeduplicatedRetryDoesNotCountTwiceInTheRollup(t *testing.T) {
	ctx := context.Background()
	s, conn := chtest.Store(t)
	events := chtest.Events(500, time.Now())

	for i := 0; i < 2; i++ {
		if err := s.InsertEvents(ctx, "same-batch", events); err != nil {
			t.Fatal(err)
		}
	}
	if got := rollupViews(t, conn); got != 500 {
		t.Fatalf("rollup views = %d, want 500", got)
	}
}

func TestBackfillCopiesEventsThatExistedBeforeTheView(t *testing.T) {
	ctx := context.Background()
	opts := chtest.Options(t)
	s, err := store.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.MigrateTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertEvents(ctx, "", chtest.Events(700, time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertEvents(ctx, "", chtest.Events(300, time.Now())); err != nil {
		t.Fatal(err)
	}

	if got := rollupViews(t, chtest.Conn(t, opts)); got != 1000 {
		t.Fatalf("rollup views = %d, want 700 backfilled + 300 live", got)
	}
}

func TestTopProductsFromTheRollupMatchesTheRawEvents(t *testing.T) {
	ctx := context.Background()
	s, conn := chtest.Store(t)
	if err := s.InsertEvents(ctx, "", chtest.Events(5000, time.Now().Add(-10*time.Minute))); err != nil {
		t.Fatal(err)
	}

	got, err := s.TopProducts(ctx, time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := conn.Query(ctx, `
		SELECT product_id, count(), uniqExact(session_id) FROM events
		GROUP BY product_id ORDER BY count() DESC, product_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var id string
		var views, visitors uint64
		if err := rows.Scan(&id, &views, &visitors); err != nil {
			t.Fatal(err)
		}
		if got[i].ProductID != id || got[i].Views != views || got[i].Visitors != visitors {
			t.Fatalf("row %d: rollup %+v, raw %s/%d/%d", i, got[i], id, views, visitors)
		}
		i++
	}
	if i != len(got) {
		t.Fatalf("rollup has %d products, raw has %d", len(got), i)
	}
}
