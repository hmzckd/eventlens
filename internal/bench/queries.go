package bench

type Query struct {
	Name       string
	Postgres   string
	ClickHouse string
}

var Queries = []Query{
	{
		Name: "Revenue per category, last 7 days",
		Postgres: `SELECT category, sum(price)::bigint AS revenue
			FROM bench_events
			WHERE event_type = 'purchase' AND occurred_at >= {end} - interval '7 days'
			GROUP BY category ORDER BY revenue DESC, category`,
		ClickHouse: `SELECT category, toInt64(sum(price)) AS revenue
			FROM bench_events
			WHERE event_type = 'purchase' AND occurred_at >= {end} - INTERVAL 7 DAY
			GROUP BY category ORDER BY revenue DESC, category`,
	},
	{
		Name: "Top 10 products by views, last 24 hours",
		Postgres: `SELECT product_id, count(*) AS views
			FROM bench_events
			WHERE event_type = 'product_view' AND occurred_at >= {end} - interval '1 day'
			GROUP BY product_id ORDER BY views DESC, product_id LIMIT 10`,
		ClickHouse: `SELECT product_id, toInt64(count()) AS views
			FROM bench_events
			WHERE event_type = 'product_view' AND occurred_at >= {end} - INTERVAL 1 DAY
			GROUP BY product_id ORDER BY views DESC, product_id LIMIT 10`,
	},
	{
		Name: "Distinct sessions per day, last 30 days",
		Postgres: `SELECT to_char(date_trunc('day', occurred_at), 'YYYY-MM-DD') AS day, count(DISTINCT session_id) AS sessions
			FROM bench_events
			GROUP BY day ORDER BY day`,
		ClickHouse: `SELECT toString(toDate(occurred_at)) AS day, toInt64(uniqExact(session_id)) AS sessions
			FROM bench_events
			GROUP BY day ORDER BY day`,
	},
	{
		Name: "Events per type and device, whole table",
		Postgres: `SELECT event_type, device, count(*) AS events
			FROM bench_events
			GROUP BY event_type, device ORDER BY event_type, device`,
		ClickHouse: `SELECT toString(event_type), toString(device), toInt64(count()) AS events
			FROM bench_events
			GROUP BY event_type, device ORDER BY event_type, device`,
	},
}
