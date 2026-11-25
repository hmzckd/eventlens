CREATE MATERIALIZED VIEW IF NOT EXISTS product_views_1m_mv TO product_views_1m AS
SELECT
    toStartOfMinute(toDateTime(occurred_at)) AS minute,
    product_id,
    countState() AS views,
    uniqState(session_id) AS visitors
FROM events
WHERE event_type = 'product_view'
GROUP BY minute, product_id;
