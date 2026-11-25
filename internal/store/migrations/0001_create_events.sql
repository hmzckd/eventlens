CREATE TABLE IF NOT EXISTS events (
    event_id    UUID,
    event_type  LowCardinality(String),
    occurred_at DateTime64(3, 'UTC'),
    received_at DateTime64(3, 'UTC'),
    session_id  String,
    user_id     String,
    product_id  String,
    category    LowCardinality(String),
    price       UInt64,
    country     LowCardinality(String),
    device      LowCardinality(String),
    url         String
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (event_type, occurred_at)
TTL toDateTime(occurred_at) + INTERVAL 30 DAY;
