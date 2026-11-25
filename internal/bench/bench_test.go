package bench

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/hmzckd/eventlens/internal/chtest"
)

func TestMain(m *testing.M) {
	os.Exit(chtest.Run(m))
}

func TestTheGeneratorIsRepeatable(t *testing.T) {
	end := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	a, b := NewGenerator(9, 1000, end), NewGenerator(9, 1000, end)
	count := 0
	for {
		ra, okA := a.Next()
		rb, okB := b.Next()
		if okA != okB {
			t.Fatal("generators stopped at different rows")
		}
		if !okA {
			break
		}
		if ra != rb {
			t.Fatalf("row %d differs:\n%+v\n%+v", count, ra, rb)
		}
		if ra.OccurredAt.After(end) || ra.OccurredAt.Before(end.Add(-span)) {
			t.Fatalf("row %d outside the 30 days: %s", count, ra.OccurredAt)
		}
		count++
	}
	if count != 1000 {
		t.Fatalf("generated %d rows, want 1000", count)
	}
}

func TestBothDatabasesGiveTheSameAnswers(t *testing.T) {
	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx, "postgres:18.7",
		postgres.WithDatabase("eventlens"),
		postgres.WithUsername("eventlens"),
		postgres.WithPassword("eventlens"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(pgContainer) })
	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable", "timezone=UTC")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	conn := chtest.Conn(t, chtest.Options(t))
	end := time.Now().Add(-time.Hour)

	pg, err := Measure(ctx, Postgres{Pool: pool}, 20_000, 1, end, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := Measure(ctx, ClickHouse{Conn: conn}, 20_000, 1, end, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if diffs := SameAnswers(pg, ch); len(diffs) > 0 {
		for i := range Queries {
			t.Logf("%s\n pg %v\n ch %v", Queries[i].Name, pg.Answers[i], ch.Answers[i])
		}
		t.Fatalf("answers differ for %v", diffs)
	}
	for i, q := range Queries {
		if len(pg.Answers[i]) == 0 {
			t.Fatalf("%q returned no rows", q.Name)
		}
	}
	if pg.Bytes <= 0 || ch.Bytes <= 0 {
		t.Fatalf("sizes: pg %d, ch %d", pg.Bytes, ch.Bytes)
	}
}
