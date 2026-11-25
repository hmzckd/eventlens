# EventLens

Event analytics for an online shop: a Go collector receives click events, batches them into ClickHouse and answers
questions such as "most viewed products in the last hour" in milliseconds.

Work in progress. See [GLOSSARY.md](GLOSSARY.md) and [docs/adr](docs/adr).

## Run locally

Requires Go 1.27.

```
go run ./cmd/eventlens
```

Then open `http://localhost:8080/health`.

## Checks

```
go vet ./...
go test ./...
```
