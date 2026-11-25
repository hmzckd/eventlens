# 0004. Precompute product views per minute with a materialized view

Date: 2026-11-25

## Status

Accepted

## Context

"Most viewed products in the last hour" is asked by every dashboard refresh. Answering it from the raw `events`
table scans every product view in the window: millions of rows at a few thousand events per second, again and again
for the same answer.

## Decision

Keep a rollup table `product_views_1m` with one row per product and minute, filled by a materialized view:

- The materialized view is an insert trigger. Every block inserted into `events` is grouped by minute and product,
  and the result is inserted into `product_views_1m`.
- The table uses `AggregatingMergeTree`. Columns hold partial aggregate states (`countState()`,
  `uniqState(session_id)`), not final numbers, because a minute can arrive in several inserts and distinct visitors
  cannot be added up. Background merges combine states for the same key; queries finish them with `countMerge` and
  `uniqMerge`.
- Rollup rows live one year, raw events 30 days.
- A one-off migration backfills the rollup from events that existed before the view. It runs on start, before the
  collector takes traffic, so no event is counted twice.
- Retried batches send `deduplicate_blocks_in_dependent_materialized_views = 1`, and the rollup table keeps its own
  deduplication window, otherwise a deduplicated retry would still be counted in the rollup. A test shows this.

`/v1/stats/top-products` reads the rollup. The funnel and active sessions still read raw events, because they need
per-session order and short windows.

## Consequences

- The top products query reads a few thousand rollup rows instead of millions of events.
- Windows are rounded down to whole minutes, so a "1h" answer can include up to 59 extra seconds.
- Every new question needs its own rollup and backfill; the raw table stays the source of truth.
- If the backfill ever has to run while events are flowing, it must split on a cutoff time (view for newer events,
  backfill for older ones) to avoid counting events twice.
