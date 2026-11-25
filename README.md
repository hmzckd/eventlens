# EventLens

Event analytics for an online shop: a Go collector receives click events, batches them into ClickHouse and answers
questions such as "most viewed products in the last hour" in milliseconds.

Work in progress. See [GLOSSARY.md](GLOSSARY.md) and [docs/adr](docs/adr).

## Run locally

Requires Go 1.27 and Docker.

1. Start ClickHouse, Prometheus and Grafana: `docker compose up -d --wait`
2. Start the service (it applies the migrations on start):
   ```
   $env:EVENTLENS_WRITE_KEY = "dev-write-key"   # PowerShell; export EVENTLENS_WRITE_KEY=dev-write-key on Linux/macOS
   go run ./cmd/eventlens
   ```
3. Open `http://localhost:8080/health`

### Stats

No write key needed; meant for dashboards on a private network.

| Endpoint | Default window | Answer |
|---|---|---|
| `GET /v1/stats/top-products?window=1h&limit=10` | `1h` | products by views, with distinct visitors |
| `GET /v1/stats/funnel?window=24h` | `24h` | sessions per step (view, cart, checkout, purchase) in order within one hour, with conversion rates |
| `GET /v1/stats/active-users?window=5m` | `5m` | distinct sessions with any event |

`window` takes Go durations (`15m`, `2h`) or days (`7d`), from 1 minute to 30 days.

## Observability

The collector exposes Prometheus metrics on `/metrics`. Prometheus (`http://localhost:9090`) scrapes the collector
running on your machine every 2 seconds, and Grafana (`http://localhost:3000`, no login, local use only) opens the
provisioned **EventLens - Pipeline** dashboard: accepted vs written events per second, queue depth against capacity,
rejected requests by reason, flush duration p50/p95, average batch size and failed insert attempts.

## Load generator

`cmd/loadgen` plays realistic shop sessions against the collector and retries on `503`:

```
go run ./cmd/loadgen -rate 5000 -duration 1m
```

| Flag | Default | Meaning |
|---|---|---|
| `-rate` | `2000` | events per second |
| `-batch` | `200` | events per request (max 500) |
| `-workers` | `8` | requests in flight |
| `-products` | `200` | products in the shop; popularity follows a Zipf distribution |
| `-duration` | `0` | stop after this long, `0` runs until Ctrl+C |
| `-key` | `$EVENTLENS_WRITE_KEY` | write key |

## Configuration

| Variable | Default |
|---|---|
| `EVENTLENS_ADDR` | `:8080` |
| `EVENTLENS_LOG_LEVEL` | `info` |
| `EVENTLENS_CLICKHOUSE_ADDR` | `localhost:9000` |
| `EVENTLENS_CLICKHOUSE_DATABASE` | `eventlens` |
| `EVENTLENS_CLICKHOUSE_USER` | `eventlens` |
| `EVENTLENS_CLICKHOUSE_PASSWORD` | `eventlens` |
| `EVENTLENS_WRITE_KEY` | required, no default |
| `EVENTLENS_QUEUE_CAPACITY` | `50000` events |
| `EVENTLENS_BATCH_SIZE` | `10000` events |
| `EVENTLENS_FLUSH_INTERVAL` | `1s` |

## API

`POST /v1/events` with header `X-Write-Key` and a body of up to 500 events:

```json
{
  "events": [
    {
      "event_id": "0193b4c2-7d1e-7a3b-9c4d-5e6f7a8b9c0d",
      "type": "product_view",
      "occurred_at": "2026-11-25T18:00:00Z",
      "session_id": "s-1",
      "product_id": "p-42",
      "category": "shoes",
      "country": "TR",
      "device": "mobile",
      "url": "/products/p-42"
    }
  ]
}
```

| Status | When |
|---|---|
| 202 | All events queued, body `{"accepted": n}`. They reach ClickHouse within about a second. |
| 400 | Bad JSON, no events, more than 500, or any invalid event (problem+json lists each one; none are stored) |
| 401 | Missing or wrong `X-Write-Key` |
| 413 | Body larger than 1 MiB |
| 503 | The queue is full (for example while ClickHouse is down). The `Retry-After: 1` header says when to retry. |

Event types: `page_view`, `product_view`, `add_to_cart`, `checkout_start`, `purchase`. `occurred_at` may be at most
5 minutes in the future and 24 hours in the past. Prices are in kuruş.

## Checks

Tests start a throwaway ClickHouse container, so Docker must be running.

```
go vet ./...
go test ./...
```
