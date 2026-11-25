package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hmzckd/eventlens/internal/bench"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
}

func run() error {
	rows := flag.Int("rows", 20_000_000, "events to generate")
	runs := flag.Int("runs", 3, "timed runs per query (the median is reported)")
	seed := flag.Int64("seed", 42, "random seed, same data for both databases")
	skipLoad := flag.Bool("skip-load", false, "reuse the bench_events tables from the last run")
	pgURL := flag.String("pg", "postgres://eventlens:eventlens@localhost:5433/eventlens?sslmode=disable&timezone=UTC", "PostgreSQL URL")
	chAddr := flag.String("ch", "localhost:9000", "ClickHouse native address")
	out := flag.String("out", "", "also write the report to this file, for example docs/benchmark.md")
	flag.Parse()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *pgURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres at %s (start it with: docker compose --profile bench up -d --wait): %w", *pgURL, err)
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:        []string{*chAddr},
		Auth:        clickhouse.Auth{Database: "eventlens", Username: "eventlens", Password: "eventlens"},
		ReadTimeout: 10 * time.Minute,
	})
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("clickhouse at %s: %w", *chAddr, err)
	}

	end := time.Now()
	if *skipLoad {
		fmt.Println("reusing the bench_events tables, running the queries only")
	} else {
		fmt.Printf("loading %d events into PostgreSQL and ClickHouse (this can take a few minutes)\n", *rows)
	}
	pg, err := bench.Measure(ctx, bench.Postgres{Pool: pool}, *rows, *seed, end, *runs, !*skipLoad)
	if err != nil {
		return err
	}
	fmt.Println("PostgreSQL done")
	ch, err := bench.Measure(ctx, bench.ClickHouse{Conn: conn}, *rows, *seed, end, *runs, !*skipLoad)
	if err != nil {
		return err
	}
	fmt.Println("ClickHouse done")
	fmt.Println()

	if diffs := bench.SameAnswers(pg, ch); len(diffs) > 0 {
		fmt.Printf("warning: the databases disagree on: %s\n\n", strings.Join(diffs, ", "))
	}

	var w io.Writer = os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = io.MultiWriter(os.Stdout, f)
	}
	bench.Report(w, *rows, *runs, pg, ch)
	return nil
}
