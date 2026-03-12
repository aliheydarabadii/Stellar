package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry                   *prometheus.Registry
	handler                    http.Handler
	attemptsCounter            prometheus.Counter
	successesCounter           prometheus.Counter
	validationFailuresCounter  prometheus.Counter
	failuresCounter            prometheus.Counter
	sourceFailuresCounter      prometheus.Counter
	persistenceFailuresCounter prometheus.Counter
	collectionDuration         prometheus.Histogram
	sourceReadDuration         prometheus.Histogram
	persistenceDuration        prometheus.Histogram
	lastAttemptGauge           prometheus.Gauge
	lastSuccessGauge           prometheus.Gauge
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	metrics := &Metrics{
		registry: registry,
		attemptsCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "integration_service",
			Subsystem: "telemetry",
			Name:      "collection_attempts_total",
			Help:      "Total telemetry collection attempts.",
		}),
		successesCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "integration_service",
			Subsystem: "telemetry",
			Name:      "collection_success_total",
			Help:      "Total successful telemetry collections.",
		}),
		validationFailuresCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "integration_service",
			Subsystem: "telemetry",
			Name:      "collection_validation_failures_total",
			Help:      "Total telemetry collections rejected by domain validation.",
		}),
		failuresCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "integration_service",
			Subsystem: "telemetry",
			Name:      "collection_failures_total",
			Help:      "Total telemetry collection failures.",
		}),
		sourceFailuresCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "integration_service",
			Subsystem: "telemetry",
			Name:      "source_failures_total",
			Help:      "Total telemetry source failures.",
		}),
		persistenceFailuresCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "integration_service",
			Subsystem: "telemetry",
			Name:      "persistence_failures_total",
			Help:      "Total telemetry persistence failures.",
		}),
		collectionDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "integration_service",
			Subsystem: "telemetry",
			Name:      "collection_duration_seconds",
			Help:      "End-to-end telemetry collection duration in seconds.",
			Buckets:   durationBuckets(),
		}),
		sourceReadDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "integration_service",
			Subsystem: "telemetry",
			Name:      "source_read_duration_seconds",
			Help:      "Telemetry source read duration in seconds.",
			Buckets:   durationBuckets(),
		}),
		persistenceDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "integration_service",
			Subsystem: "telemetry",
			Name:      "persistence_duration_seconds",
			Help:      "Telemetry persistence duration in seconds.",
			Buckets:   durationBuckets(),
		}),
		lastAttemptGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "integration_service",
			Subsystem: "telemetry",
			Name:      "last_attempt_timestamp_seconds",
			Help:      "Unix timestamp of the last telemetry collection attempt.",
		}),
		lastSuccessGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "integration_service",
			Subsystem: "telemetry",
			Name:      "last_success_timestamp_seconds",
			Help:      "Unix timestamp of the last successful telemetry collection.",
		}),
	}

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		metrics.attemptsCounter,
		metrics.successesCounter,
		metrics.validationFailuresCounter,
		metrics.failuresCounter,
		metrics.sourceFailuresCounter,
		metrics.persistenceFailuresCounter,
		metrics.collectionDuration,
		metrics.sourceReadDuration,
		metrics.persistenceDuration,
		metrics.lastAttemptGauge,
		metrics.lastSuccessGauge,
	)

	metrics.handler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{})

	return metrics
}

func (m *Metrics) RecordAttempt(collectedAt time.Time) {
	if m == nil {
		return
	}

	m.attemptsCounter.Inc()
	if !collectedAt.IsZero() {
		m.lastAttemptGauge.Set(float64(collectedAt.Unix()))
	}
}

func (m *Metrics) RecordSuccess(collectedAt time.Time) {
	if m == nil {
		return
	}

	m.successesCounter.Inc()
	if !collectedAt.IsZero() {
		m.lastSuccessGauge.Set(float64(collectedAt.Unix()))
	}
}

func (m *Metrics) RecordValidationFailure() {
	if m == nil {
		return
	}

	m.validationFailuresCounter.Inc()
}

func (m *Metrics) RecordFailure() {
	if m == nil {
		return
	}

	m.failuresCounter.Inc()
}

func (m *Metrics) RecordSourceFailure() {
	if m == nil {
		return
	}

	m.sourceFailuresCounter.Inc()
}

func (m *Metrics) RecordPersistenceFailure() {
	if m == nil {
		return
	}

	m.persistenceFailuresCounter.Inc()
}

func (m *Metrics) ObserveCollectionDuration(duration time.Duration) {
	if m == nil {
		return
	}

	m.collectionDuration.Observe(duration.Seconds())
}

func (m *Metrics) ObserveSourceReadDuration(duration time.Duration) {
	if m == nil {
		return
	}

	m.sourceReadDuration.Observe(duration.Seconds())
}

func (m *Metrics) ObservePersistenceDuration(duration time.Duration) {
	if m == nil {
		return
	}

	m.persistenceDuration.Observe(duration.Seconds())
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m == nil || m.handler == nil {
		http.Error(w, "metrics unavailable", http.StatusInternalServerError)
		return
	}

	m.handler.ServeHTTP(w, r)
}

func durationBuckets() []float64 {
	return []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
}
