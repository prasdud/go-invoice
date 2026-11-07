package api

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests by endpoint and status.",
	}, []string{"endpoint", "status"})

	renderDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "render_duration_seconds",
		Help:    "Render duration by engine.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"engine"})

	uploadDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "upload_duration_seconds",
		Help:    "Storage upload duration.",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	})

	renderWait = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "render_wait_seconds",
		Help:    "Time spent waiting for a render slot.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1},
	})

	rendersInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "renders_in_flight",
		Help: "Renders currently holding a semaphore slot.",
	})
)
