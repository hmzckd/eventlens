package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Queue interface {
	Depth() int
	Capacity() int
	Dropped() int64
}

type Metrics struct {
	registry      *prometheus.Registry
	accepted      prometheus.Counter
	rejected      *prometheus.CounterVec
	written       prometheus.Counter
	flushFailures prometheus.Counter
	flushDuration *prometheus.HistogramVec
	batchSize     prometheus.Histogram
}

func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		accepted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "eventlens_events_accepted_total",
			Help: "Events accepted into the queue (answered 202).",
		}),
		rejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "eventlens_requests_rejected_total",
			Help: "Event requests refused, by reason.",
		}, []string{"reason"}),
		written: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "eventlens_events_written_total",
			Help: "Events written to ClickHouse.",
		}),
		flushFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "eventlens_flush_failures_total",
			Help: "Failed insert attempts; each one is retried.",
		}),
		flushDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "eventlens_flush_duration_seconds",
			Help:    "Time of one insert attempt.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"result"}),
		batchSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "eventlens_flush_batch_size",
			Help:    "Events per successful insert.",
			Buckets: []float64{1, 10, 100, 500, 1000, 2500, 5000, 10000},
		}),
	}
	for _, reason := range []string{"unauthorized", "invalid", "queue_full"} {
		m.rejected.WithLabelValues(reason)
	}
	m.registry.MustRegister(
		m.accepted, m.rejected, m.written, m.flushFailures, m.flushDuration, m.batchSize,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

func (m *Metrics) WatchQueue(q Queue) {
	m.registry.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "eventlens_queue_depth",
			Help: "Events waiting in the queue.",
		}, func() float64 { return float64(q.Depth()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "eventlens_queue_capacity",
			Help: "Events the queue can hold.",
		}, func() float64 { return float64(q.Capacity()) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "eventlens_events_dropped_total",
			Help: "Events given up on during shutdown.",
		}, func() float64 { return float64(q.Dropped()) }),
	)
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) EventsAccepted(n int) {
	m.accepted.Add(float64(n))
}

func (m *Metrics) RequestRejected(reason string) {
	m.rejected.WithLabelValues(reason).Inc()
}

func (m *Metrics) FlushAttempt(events int, took time.Duration, err error) {
	if err != nil {
		m.flushFailures.Inc()
		m.flushDuration.WithLabelValues("error").Observe(took.Seconds())
		return
	}
	m.flushDuration.WithLabelValues("ok").Observe(took.Seconds())
	m.batchSize.Observe(float64(events))
	m.written.Add(float64(events))
}
