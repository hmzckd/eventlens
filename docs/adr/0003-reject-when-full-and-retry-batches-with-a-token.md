# 0003. Reject when the queue is full, retry batches with a deduplication token

Date: 2026-11-25

## Status

Accepted

## Context

ClickHouse can be slow or down for a while (restart, network blip, overload). The flusher cannot write, but clients
keep sending events. Something has to give, and there are three options:

1. Block the request until there is room. Connections pile up, latency grows for everyone and the collector can run
   out of memory or file handles.
2. Drop the events silently. Nobody notices until the dashboards are wrong.
3. Refuse the request openly and let the client retry later.

A failed insert also has an unknown outcome: the connection may break after ClickHouse already stored the batch.
ClickHouse has no unique constraints, so a plain retry would store the batch twice.

## Decision

- When the queue cannot hold a whole request, answer `503 Service Unavailable` with `Retry-After: 1`. Clients wait
  and retry. This is backpressure: the slow part of the system slows down the producers instead of drowning.
- The flusher never drops a batch while running. It retries with exponential backoff (100 ms doubling up to 5 s).
- Every batch gets a random `insert_deduplication_token` once, and every retry of that batch sends the same token.
  The table setting `non_replicated_deduplication_window = 1000` makes ClickHouse remember the last 1000 tokens and
  skip a block it has already stored.
- On shutdown the flusher keeps retrying for at most 10 seconds, then gives up, logs how many events were lost and
  exits.

## Consequences

- No event that got a `202` is lost while the process keeps running, even across a ClickHouse outage, as long as
  the outage is shorter than the time it takes clients to fill the queue.
- A retried batch is stored exactly once.
- Clients must handle `503` and retry. The load generator and the demo page do.
- Duplicates sent by a client (the same event in two requests) are not removed by the token. That is a separate
  problem, measured in a later step.
