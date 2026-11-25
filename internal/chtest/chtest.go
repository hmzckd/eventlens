package chtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	chmodule "github.com/testcontainers/testcontainers-go/modules/clickhouse"

	"github.com/hmzckd/eventlens/internal/event"
	"github.com/hmzckd/eventlens/internal/store"
)

const (
	Image    = "clickhouse/clickhouse-server:26.8.47.5"
	username = "eventlens"
	password = "eventlens"
)

var addr string

func Run(m *testing.M) int {
	ctx := context.Background()
	container, err := chmodule.Run(ctx, Image,
		chmodule.WithUsername(username),
		chmodule.WithPassword(password),
		chmodule.WithDatabase("eventlens"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start clickhouse container (is Docker running?): %v\n", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(container) }()

	addr, err = container.ConnectionHost(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clickhouse address: %v\n", err)
		return 1
	}
	return m.Run()
}

func Options(t *testing.T) store.Options {
	t.Helper()
	ctx := context.Background()

	admin, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{Username: username, Password: password},
	})
	if err != nil {
		t.Fatalf("connect to clickhouse: %v", err)
	}
	defer admin.Close()

	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	database := "test_" + hex.EncodeToString(suffix)
	if err := admin.Exec(ctx, "CREATE DATABASE "+database); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() { dropDatabase(database) })

	return store.Options{Addr: addr, Database: database, Username: username, Password: password}
}

func Store(t *testing.T) (*store.Store, driver.Conn) {
	t.Helper()
	ctx := context.Background()
	opts := Options(t)
	s, err := store.Open(ctx, opts)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s, Conn(t, opts)
}

func Conn(t *testing.T, opts store.Options) driver.Conn {
	t.Helper()
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{opts.Addr},
		Auth: clickhouse.Auth{Database: opts.Database, Username: opts.Username, Password: opts.Password},
	})
	if err != nil {
		t.Fatalf("connect to clickhouse: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func dropDatabase(name string) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{Username: username, Password: password},
	})
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name)
}

func Events(n int, at time.Time) []event.Event {
	out := make([]event.Event, n)
	for i := range out {
		out[i] = event.Event{
			EventID:    uuid.New(),
			Type:       event.ProductView,
			OccurredAt: at.UTC(),
			ReceivedAt: at.UTC(),
			SessionID:  fmt.Sprintf("s-%d", i%50),
			ProductID:  fmt.Sprintf("p-%d", i%10),
			Category:   "shoes",
			Country:    "TR",
			Device:     "mobile",
			URL:        fmt.Sprintf("/products/p-%d", i%10),
		}
	}
	return out
}
