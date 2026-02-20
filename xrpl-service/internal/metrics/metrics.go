package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP metrics
	HTTPRequestTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "xrpl_validator_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "xrpl_validator_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	// WebSocket metrics
	WebSocketConnectionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "xrpl_validator_websocket_connections_total",
			Help: "Total number of WebSocket connections",
		},
	)

	WebSocketConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "xrpl_validator_websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)

	// Validator metrics
	ValidatorFetchTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "xrpl_validator_fetch_total",
			Help: "Total number of validator fetches",
		},
		[]string{"status"},
	)

	ValidatorsCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "xrpl_validator_count",
			Help: "Number of validators currently tracked",
		},
	)

	// Transaction metrics
	TransactionsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "xrpl_validator_transactions_processed_total",
			Help: "Total number of transactions processed",
		},
		[]string{"type"},
	)

	TransactionBufferSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "xrpl_validator_transaction_buffer_size",
			Help: "Current size of transaction buffer",
		},
	)

	// Geolocation metrics
	GeolocationEnrichTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "xrpl_validator_geolocation_enrich_total",
			Help: "Total number of geolocation enrichments",
		},
		[]string{"status"},
	)

	// XRPL upstream client metrics
	UpstreamCommandTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "xrpl_validator_upstream_command_total",
			Help: "Total number of XRPL commands",
		},
		[]string{"method", "status"},
	)
)
