package bench

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Target interface {
	Name() string
	Load(ctx context.Context, gen *Generator) error
	Size(ctx context.Context) (int64, error)
	Newest(ctx context.Context) (time.Time, error)
	Query(ctx context.Context, q Query, end time.Time) ([][]string, error)
}

type Postgres struct{ Pool *pgxpool.Pool }

func (Postgres) Name() string { return "PostgreSQL" }

func (p Postgres) Load(ctx context.Context, gen *Generator) error {
	stmts := []string{
		`DROP TABLE IF EXISTS bench_events`,
		`CREATE TABLE bench_events (
			event_id    uuid        NOT NULL PRIMARY KEY,
			event_type  text        NOT NULL,
			occurred_at timestamptz NOT NULL,
			received_at timestamptz NOT NULL,
			session_id  text        NOT NULL,
			user_id     text        NOT NULL,
			product_id  text        NOT NULL,
			category    text        NOT NULL,
			price       bigint      NOT NULL,
			country     text        NOT NULL,
			device      text        NOT NULL,
			url         text        NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := p.Pool.Exec(ctx, s); err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
	}
	columns := []string{"event_id", "event_type", "occurred_at", "received_at", "session_id", "user_id",
		"product_id", "category", "price", "country", "device", "url"}
	src := pgx.CopyFromFunc(func() ([]any, error) {
		r, ok := gen.Next()
		if !ok {
			return nil, nil
		}
		return []any{r.EventID, r.Type, r.OccurredAt, r.ReceivedAt, r.SessionID, r.UserID,
			r.ProductID, r.Category, int64(r.Price), r.Country, r.Device, r.URL}, nil
	})
	if _, err := p.Pool.CopyFrom(ctx, pgx.Identifier{"bench_events"}, columns, src); err != nil {
		return fmt.Errorf("postgres copy: %w", err)
	}
	for _, s := range []string{
		`CREATE INDEX bench_events_type_time ON bench_events (event_type, occurred_at)`,
		`VACUUM ANALYZE bench_events`,
	} {
		if _, err := p.Pool.Exec(ctx, s); err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
	}
	return nil
}

func (p Postgres) Size(ctx context.Context) (int64, error) {
	var n int64
	err := p.Pool.QueryRow(ctx, `SELECT pg_total_relation_size('bench_events')`).Scan(&n)
	return n, err
}

func (p Postgres) Newest(ctx context.Context) (time.Time, error) {
	var t time.Time
	err := p.Pool.QueryRow(ctx, `SELECT max(occurred_at) FROM bench_events`).Scan(&t)
	return t.UTC(), err
}

func (p Postgres) Query(ctx context.Context, q Query, end time.Time) ([][]string, error) {
	literal := "'" + end.UTC().Format("2006-01-02 15:04:05.000") + "+00'::timestamptz"
	rows, err := p.Pool.Query(ctx, strings.ReplaceAll(q.Postgres, "{end}", literal))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		out = append(out, toStrings(values))
	}
	return out, rows.Err()
}

type ClickHouse struct{ Conn driver.Conn }

func (ClickHouse) Name() string { return "ClickHouse" }

const clickhouseBatch = 100_000

func (c ClickHouse) Load(ctx context.Context, gen *Generator) error {
	for _, s := range []string{
		`DROP TABLE IF EXISTS bench_events`,
		`CREATE TABLE bench_events (
			event_id    UUID,
			event_type  LowCardinality(String),
			occurred_at DateTime64(3, 'UTC'),
			received_at DateTime64(3, 'UTC'),
			session_id  String,
			user_id     String,
			product_id  String,
			category    LowCardinality(String),
			price       UInt64,
			country     LowCardinality(String),
			device      LowCardinality(String),
			url         String
		)
		ENGINE = MergeTree
		PARTITION BY toYYYYMM(occurred_at)
		ORDER BY (event_type, occurred_at)`,
	} {
		if err := c.Conn.Exec(ctx, s); err != nil {
			return fmt.Errorf("clickhouse: %w", err)
		}
	}
	for done := false; !done; {
		batch, err := c.Conn.PrepareBatch(ctx, "INSERT INTO bench_events")
		if err != nil {
			return fmt.Errorf("clickhouse: %w", err)
		}
		n := 0
		for n < clickhouseBatch {
			r, ok := gen.Next()
			if !ok {
				done = true
				break
			}
			if err := batch.Append(r.EventID, r.Type, r.OccurredAt, r.ReceivedAt, r.SessionID, r.UserID,
				r.ProductID, r.Category, r.Price, r.Country, r.Device, r.URL); err != nil {
				return fmt.Errorf("clickhouse append: %w", err)
			}
			n++
		}
		if n == 0 {
			_ = batch.Abort()
			break
		}
		if err := batch.Send(); err != nil {
			return fmt.Errorf("clickhouse insert: %w", err)
		}
	}
	if err := c.Conn.Exec(ctx, `OPTIMIZE TABLE bench_events FINAL`); err != nil {
		return fmt.Errorf("clickhouse optimize: %w", err)
	}
	return nil
}

func (c ClickHouse) Size(ctx context.Context) (int64, error) {
	var n uint64
	err := c.Conn.QueryRow(ctx, `
		SELECT sum(bytes_on_disk) FROM system.parts
		WHERE database = currentDatabase() AND table = 'bench_events' AND active`).Scan(&n)
	return int64(n), err
}

func (c ClickHouse) Newest(ctx context.Context) (time.Time, error) {
	var t time.Time
	err := c.Conn.QueryRow(ctx, `SELECT max(occurred_at) FROM bench_events`).Scan(&t)
	return t.UTC(), err
}

func (c ClickHouse) Query(ctx context.Context, q Query, end time.Time) ([][]string, error) {
	literal := "toDateTime64('" + end.UTC().Format("2006-01-02 15:04:05.000") + "', 3, 'UTC')"
	rows, err := c.Conn.Query(ctx, strings.ReplaceAll(q.ClickHouse, "{end}", literal))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	types := rows.ColumnTypes()
	var out [][]string
	for rows.Next() {
		dest := make([]any, len(types))
		for i, t := range types {
			dest[i] = reflect.New(t.ScanType()).Interface()
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		values := make([]any, len(dest))
		for i, d := range dest {
			values[i] = reflect.ValueOf(d).Elem().Interface()
		}
		out = append(out, toStrings(values))
	}
	return out, rows.Err()
}

func toStrings(values []any) []string {
	out := make([]string, len(values))
	for i, v := range values {
		if t, ok := v.(time.Time); ok {
			out[i] = t.UTC().Format(time.RFC3339)
			continue
		}
		out[i] = fmt.Sprint(v)
	}
	return out
}
