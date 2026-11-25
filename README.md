# EventLens

Event analytics for an online shop: a Go collector receives click events, batches them into ClickHouse and answers
questions such as "most viewed products in the last hour" in milliseconds.

Work in progress. See [GLOSSARY.md](GLOSSARY.md) and [docs/adr](docs/adr).

## Run locally

Requires Go 1.27 and Docker.

1. Start ClickHouse: `docker compose up -d --wait`
2. Start the service (it applies the migrations on start):
   ```
   $env:EVENTLENS_WRITE_KEY = "dev-write-key"   # PowerShell; export EVENTLENS_WRITE_KEY=dev-write-key on Linux/macOS
   go run ./cmd/eventlens
   ```
3. Open `http://localhost:8080/health`

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
| 200 | All events stored, body `{"accepted": n}` |
| 400 | Bad JSON, no events, more than 500, or any invalid event (problem+json lists each one; none are stored) |
| 401 | Missing or wrong `X-Write-Key` |
| 413 | Body larger than 1 MiB |
| 503 | ClickHouse could not store the events |

Event types: `page_view`, `product_view`, `add_to_cart`, `checkout_start`, `purchase`. `occurred_at` may be at most
5 minutes in the future and 24 hours in the past. Prices are in kuruş.

## Checks

Tests start a throwaway ClickHouse container, so Docker must be running.

```
go vet ./...
go test ./...
```
