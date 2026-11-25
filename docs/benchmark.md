# Benchmark: PostgreSQL vs ClickHouse

20000000 synthetic events over 30 days, 16 CPUs (windows/amd64), median of 3 runs after one warm-up run.
Both databases run in Docker on the same machine; PostgreSQL has an index on `(event_type, occurred_at)`,
ClickHouse uses the same columns as its sorting key.

| | PostgreSQL | ClickHouse | ClickHouse is |
|---|---:|---:|---:|
| Load time | 3m21.8s | 53.8s | 3.7x faster |
| Disk size | 4.15 GiB | 822.8 MiB | 5.2x smaller |
| Revenue per category, last 7 days | 422.7 ms | 7.5 ms | 56.6x faster |
| Top 10 products by views, last 24 hours | 729.2 ms | 6.5 ms | 112.2x faster |
| Distinct sessions per day, last 30 days | 28501.6 ms | 760.5 ms | 37.5x faster |
| Events per type and device, whole table | 1139.7 ms | 153.9 ms | 7.4x faster |
