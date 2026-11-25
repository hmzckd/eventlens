package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/hmzckd/eventlens/internal/chtest"
	"github.com/hmzckd/eventlens/internal/store"
)

func TestMain(m *testing.M) {
	os.Exit(chtest.Run(m))
}

func TestMigrateCreatesEventsTable(t *testing.T) {
	ctx := context.Background()
	_, conn := chtest.Store(t)

	var engine, partitionKey, sortingKey string
	err := conn.QueryRow(ctx, `
		SELECT engine, partition_key, sorting_key
		FROM system.tables
		WHERE database = currentDatabase() AND name = 'events'`).Scan(&engine, &partitionKey, &sortingKey)
	if err != nil {
		t.Fatalf("read events table: %v", err)
	}
	if engine != "MergeTree" {
		t.Errorf("engine = %q, want MergeTree", engine)
	}
	if partitionKey != "toYYYYMM(occurred_at)" {
		t.Errorf("partition key = %q", partitionKey)
	}
	if sortingKey != "event_type, occurred_at" {
		t.Errorf("sorting key = %q", sortingKey)
	}
}

func TestMigrateIsSafeToRunTwice(t *testing.T) {
	ctx := context.Background()
	opts := chtest.Options(t)

	for i := 0; i < 2; i++ {
		s, err := store.Open(ctx, opts)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if err := s.Migrate(ctx); err != nil {
			t.Fatalf("migrate run %d: %v", i+1, err)
		}
		_ = s.Close()
	}

	conn := chtest.Conn(t, opts)
	var applied, files uint64
	if err := conn.QueryRow(ctx, "SELECT count(), uniqExact(version) FROM schema_migrations").Scan(&applied, &files); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if applied != files {
		t.Fatalf("schema_migrations has %d rows for %d versions, a migration ran twice", applied, files)
	}
}

func TestOpenFailsFastWhenClickHouseIsUnreachable(t *testing.T) {
	_, err := store.Open(context.Background(), store.Options{Addr: "127.0.0.1:1", Database: "eventlens"})
	if err == nil {
		t.Fatal("expected an error for an address nobody listens on")
	}
}
