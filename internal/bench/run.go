package bench

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"slices"
	"time"
)

type Result struct {
	Target   string
	LoadTime time.Duration
	Bytes    int64
	Medians  []time.Duration
	Answers  [][][]string
}

// Measure loads the data (unless load is false) and times every query. Windows such as "last 7 days" are counted
// back from the newest event in the table, not from now(), so both databases answer the same question even when
// they are measured minutes apart.
func Measure(ctx context.Context, t Target, rows int, seed int64, end time.Time, runs int, load bool) (Result, error) {
	res := Result{Target: t.Name()}
	if load {
		start := time.Now()
		if err := t.Load(ctx, NewGenerator(seed, rows, end)); err != nil {
			return res, err
		}
		res.LoadTime = time.Since(start)
	}
	size, err := t.Size(ctx)
	if err != nil {
		return res, fmt.Errorf("%s size: %w", t.Name(), err)
	}
	res.Bytes = size

	newest, err := t.Newest(ctx)
	if err != nil {
		return res, fmt.Errorf("%s newest event: %w", t.Name(), err)
	}

	for _, q := range Queries {
		answer, err := t.Query(ctx, q, newest)
		if err != nil {
			return res, fmt.Errorf("%s %q: %w", t.Name(), q.Name, err)
		}
		times := make([]time.Duration, runs)
		for i := range times {
			start := time.Now()
			if _, err := t.Query(ctx, q, newest); err != nil {
				return res, fmt.Errorf("%s %q: %w", t.Name(), q.Name, err)
			}
			times[i] = time.Since(start)
		}
		slices.Sort(times)
		res.Medians = append(res.Medians, times[len(times)/2])
		res.Answers = append(res.Answers, answer)
	}
	return res, nil
}

func SameAnswers(a, b Result) []string {
	var diffs []string
	for i, q := range Queries {
		if !reflect.DeepEqual(a.Answers[i], b.Answers[i]) {
			diffs = append(diffs, q.Name)
		}
	}
	return diffs
}

func Report(w io.Writer, rows int, runs int, pg, ch Result) {
	fmt.Fprintf(w, "# Benchmark: PostgreSQL vs ClickHouse\n\n")
	fmt.Fprintf(w, "%d synthetic events over 30 days, %d CPUs (%s/%s), median of %d runs after one warm-up run.\n",
		rows, runtime.NumCPU(), runtime.GOOS, runtime.GOARCH, runs)
	fmt.Fprintf(w, "Both databases run in Docker on the same machine; PostgreSQL has an index on `(event_type, occurred_at)`,\n")
	fmt.Fprintf(w, "ClickHouse uses the same columns as its sorting key.\n\n")
	fmt.Fprintf(w, "| | PostgreSQL | ClickHouse | ClickHouse is |\n|---|---:|---:|---:|\n")
	if pg.LoadTime > 0 && ch.LoadTime > 0 {
		fmt.Fprintf(w, "| Load time | %s | %s | %s |\n", round(pg.LoadTime), round(ch.LoadTime), ratio(pg.LoadTime, ch.LoadTime))
	}
	fmt.Fprintf(w, "| Disk size | %s | %s | %.1fx smaller |\n", bytes(pg.Bytes), bytes(ch.Bytes), float64(pg.Bytes)/float64(max(ch.Bytes, 1)))
	for i, q := range Queries {
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n", q.Name, ms(pg.Medians[i]), ms(ch.Medians[i]), ratio(pg.Medians[i], ch.Medians[i]))
	}
}

func ratio(pg, ch time.Duration) string {
	if ch <= 0 {
		return "-"
	}
	r := float64(pg) / float64(ch)
	if r >= 1 {
		return fmt.Sprintf("%.1fx faster", r)
	}
	return fmt.Sprintf("%.1fx slower", 1/r)
}

func ms(d time.Duration) string {
	return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
}

func round(d time.Duration) string {
	return d.Round(100 * time.Millisecond).String()
}

func bytes(n int64) string {
	const mib = 1 << 20
	if n >= 1<<30 {
		return fmt.Sprintf("%.2f GiB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%.1f MiB", float64(n)/mib)
}
