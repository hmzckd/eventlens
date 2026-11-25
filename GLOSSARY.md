# Glossary

| Term | Meaning in EventLens |
|---|---|
| Event | One thing a visitor did on the shop: `page_view`, `product_view`, `add_to_cart`, `checkout_start` or `purchase`. Events are only ever appended, never updated. |
| Collector | The HTTP service that receives events from browsers and load generators and hands them to the pipeline. |
| OLTP | Online transaction processing. Many small reads and writes of single rows (an order, a user). PostgreSQL is built for this. |
| OLAP | Online analytical processing. Few large queries that scan and aggregate millions of rows ("views per product in the last hour"). ClickHouse is built for this. |
| Row store | Keeps all columns of a row next to each other on disk. Fast to fetch one whole row. |
| Column store | Keeps each column in its own file. A query reads only the columns it names, and similar values next to each other compress very well. |
