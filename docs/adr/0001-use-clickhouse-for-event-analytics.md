# 0001. Use ClickHouse for event analytics

Date: 2026-11-25

## Status

Accepted

## Context

EventLens receives thousands of click events per second and must answer questions such as "most viewed products in
the last hour" or "how many sessions that viewed a product went on to buy" over tens of millions of rows within a
second. Events are written once and never updated.

PostgreSQL is a row store built for transactions. Answering an aggregate over 20 million rows means reading every
column of every matching row, and indexes only help when the query is very selective.

## Decision

Store events in ClickHouse, a column-oriented OLAP database. Each query reads only the columns it needs, columns
compress well, and aggregations run vectorised over blocks of values. The service writes in large batches because
ClickHouse is built for few large inserts, not many small ones.

## Consequences

- Aggregate queries over the whole data set stay fast without hand-tuned indexes.
- There are no unique constraints and no cheap single-row updates or deletes. Duplicate handling has to be designed
  (see later ADRs).
- Writes must be batched by the application.
- One more piece of infrastructure to run; for local development it is a single container.
