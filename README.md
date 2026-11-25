# EventLens

Event analytics for an online shop: a Go collector receives click events, batches them into ClickHouse and answers
questions such as "most viewed products in the last hour" in milliseconds.

Work in progress. See [GLOSSARY.md](GLOSSARY.md) and [docs/adr](docs/adr).

## Run locally

Requires Go 1.27 and Docker.

1. Start ClickHouse: `docker compose up -d --wait`
2. Start the service: `go run ./cmd/eventlens` (it applies the migrations on start)
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

## Checks

Tests start a throwaway ClickHouse container, so Docker must be running.

```
go vet ./...
go test ./...
```
