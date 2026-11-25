package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type Options struct {
	Addr     string
	Database string
	Username string
	Password string
}

type Store struct {
	conn driver.Conn
}

func Open(ctx context.Context, opts Options) (*Store, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{opts.Addr},
		Auth: clickhouse.Auth{
			Database: opts.Database,
			Username: opts.Username,
			Password: opts.Password,
		},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping clickhouse at %s: %w", opts.Addr, err)
	}
	return &Store{conn: conn}, nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.conn.Ping(ctx)
}

func (s *Store) Close() error {
	return s.conn.Close()
}
