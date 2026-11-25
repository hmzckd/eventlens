# Glossary

| Term | Meaning in EventLens |
|---|---|
| Event | One thing a visitor did on the shop: `page_view`, `product_view`, `add_to_cart`, `checkout_start` or `purchase`. Events are only ever appended, never updated. |
| Collector | The HTTP service that receives events from browsers and load generators and hands them to the pipeline. |
| OLTP | Online transaction processing. Many small reads and writes of single rows (an order, a user). PostgreSQL is built for this. |
| OLAP | Online analytical processing. Few large queries that scan and aggregate millions of rows ("views per product in the last hour"). ClickHouse is built for this. |
| Row store | Keeps all columns of a row next to each other on disk. Fast to fetch one whole row. |
| Column store | Keeps each column in its own file. A query reads only the columns it names, and similar values next to each other compress very well. |
| MergeTree | ClickHouse's main table engine. Every insert writes a new immutable part; background merges combine parts into bigger ones. |
| Part | A sorted, immutable chunk of a table on disk, created by one insert (or one merge). Thousands of tiny parts make reads and merges slow. |
| Partition | A group of parts that share a partition key value (here one calendar month). Old partitions can be dropped as a whole. |
| Sorting key | `ORDER BY` of a MergeTree table. Rows are stored in this order, and the sparse primary index lets a query skip blocks whose key range cannot match. The closest thing ClickHouse has to an index. |
| Granule | The smallest block a query reads (8192 rows by default). The primary index stores one entry per granule. |
| LowCardinality | A column wrapper that stores each distinct value once in a dictionary and keeps small numbers in the column. Good for columns with few distinct values (event type, country, device). |
| TTL | Time to live. ClickHouse deletes rows whose TTL expression is in the past during merges. Raw events live 30 days. |
| Migration | A numbered SQL file under `internal/store/migrations` applied once, in order, and recorded in `schema_migrations`. |
| Write key | A secret that lets a client send events but read nothing. Sent in the `X-Write-Key` header and compared in constant time. |
| `occurred_at` / `received_at` | When the visitor did it (client clock) and when the collector got it (server clock). The gap shows late events, e.g. a phone that was offline. |
| Batch / flush | A group of events written with one INSERT. The flusher writes a batch when it holds 10,000 events or once a second. |
| Bounded queue | The in-memory buffer between HTTP requests and the flusher. It has a fixed capacity, so memory cannot grow without limit. |
| Reservation | Before a request enqueues its events, it adds their count to a counter. If the total would exceed the capacity, it takes them back and the whole request is refused, so a request is never half accepted. |
| Graceful shutdown | On Ctrl+C or SIGTERM: stop taking requests, write everything still queued, then exit. |
| Loss window | What can be lost on a crash: the events still in the queue. Documented in ADR 0002. |
| Backpressure | A full queue pushes back on producers (`503` + `Retry-After`) instead of growing or silently dropping events. |
| Unknown outcome | An insert whose answer was lost. The data may or may not be stored, so the only safe move is to retry in a way that cannot duplicate. |
| Deduplication token | `insert_deduplication_token`: a random id per batch, reused on every retry. ClickHouse skips a block whose token it has seen within `non_replicated_deduplication_window`. |
| Exponential backoff | Waiting longer after each failed attempt (100 ms, 200 ms, 400 ms ... up to 5 s) so a struggling server is not hammered. |
