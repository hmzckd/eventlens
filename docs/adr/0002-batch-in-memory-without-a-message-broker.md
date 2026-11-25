# 0002. Batch events in memory, without a message broker

Date: 2026-11-25

## Status

Accepted

## Context

Every INSERT into a MergeTree table creates a new part on disk. Thousands of single-event inserts per second create
thousands of tiny parts, background merges fall behind and ClickHouse eventually refuses inserts with
"too many parts". Events have to reach ClickHouse in large batches.

Options considered:

1. Put Kafka in front and let a consumer (or ClickHouse's Kafka engine) insert in batches. Nothing is lost when the
   collector crashes, but it adds a broker to run and operate.
2. Let ClickHouse batch on the server with `async_insert`. Simple for the client, but the batching rules live in
   server settings and are harder to observe and test.
3. Batch inside the collector: a bounded in-memory queue and one goroutine that flushes on size or time.

## Decision

Option 3. Requests put their events into a bounded queue and get `202 Accepted` at once. A single flusher writes a
batch when it reaches 10,000 events or every second, whichever comes first. A request is accepted whole or not at
all: the queue reserves room for all of its events before any of them is enqueued. On shutdown the HTTP server stops
first, then the flusher writes everything still queued.

## Consequences

- One INSERT per second instead of thousands, so ClickHouse creates few, large parts.
- `202` means "queued", not "stored". Events in the queue are lost if the process crashes or is killed without a
  clean shutdown: at most one second of traffic plus what is waiting in the queue. For click analytics that is an
  accepted trade-off. If losing events ever becomes unacceptable (for example billing events), option 1 is the
  upgrade path and the HTTP API stays the same.
- Memory use is bounded by the queue capacity (50,000 events by default).
