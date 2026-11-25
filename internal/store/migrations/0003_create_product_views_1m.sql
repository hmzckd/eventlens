CREATE TABLE IF NOT EXISTS product_views_1m (
    minute     DateTime('UTC'),
    product_id String,
    views      AggregateFunction(count),
    visitors   AggregateFunction(uniq, String)
)
ENGINE = AggregatingMergeTree
PARTITION BY toYYYYMM(minute)
ORDER BY (minute, product_id)
TTL minute + INTERVAL 1 YEAR
SETTINGS non_replicated_deduplication_window = 1000;
