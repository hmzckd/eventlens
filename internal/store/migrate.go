package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version uint32
	name    string
	sql     string
}

const createMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    UInt32,
    name       String,
    applied_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree
ORDER BY version`

func (s *Store) Migrate(ctx context.Context) error {
	all, err := loadMigrations()
	if err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, createMigrationsTable); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[uint32]bool{}
	rows, err := s.conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v uint32
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}

	for _, m := range all {
		if applied[m.version] {
			continue
		}
		if err := s.conn.Exec(ctx, m.sql); err != nil {
			return fmt.Errorf("migration %04d_%s: %w", m.version, m.name, err)
		}
		if err := s.conn.Exec(ctx, "INSERT INTO schema_migrations (version, name) VALUES (?, ?)", m.version, m.name); err != nil {
			return fmt.Errorf("record migration %04d: %w", m.version, err)
		}
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, path := range names {
		base := strings.TrimSuffix(strings.TrimPrefix(path, "migrations/"), ".sql")
		prefix, name, ok := strings.Cut(base, "_")
		if !ok {
			return nil, fmt.Errorf("migration %s: name must look like 0001_name.sql", path)
		}
		version, err := strconv.ParseUint(prefix, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("migration %s: bad version: %w", path, err)
		}
		body, err := migrationFiles.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sql := strings.TrimSpace(string(body))
		sql = strings.TrimSuffix(sql, ";")
		if strings.Contains(sql, ";") {
			return nil, fmt.Errorf("migration %s: one statement per file", path)
		}
		out = append(out, migration{version: uint32(version), name: name, sql: sql})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i := 1; i < len(out); i++ {
		if out[i].version == out[i-1].version {
			return nil, fmt.Errorf("two migrations with version %04d", out[i].version)
		}
	}
	return out, nil
}
