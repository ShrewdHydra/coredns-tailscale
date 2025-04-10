package tailscale

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Variables declared for monitoring.
var (
	// RequestCount exports a prometheus metric that is incremented every time a DNS request is processed.
	RequestCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "tailscale",
		Name:      "requests_total",
		Help:      "Counter of DNS requests processed by record type.",
	}, []string{"server", "type"})

	// RcodeCount exports a prometheus metric that counts responses by return code.
	RcodeCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "tailscale",
		Name:      "responses_total",
		Help:      "Counter of DNS responses by return code.",
	}, []string{"rcode", "server"})

	// RequestDuration exports a prometheus metric that tracks the duration of DNS requests.
	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: "tailscale",
		Name:      "request_duration_seconds",
		Buckets:   plugin.TimeBuckets,
		Help:      "Histogram of the time each DNS request took to resolve.",
	}, []string{"server"})

	// NodeCount exports a prometheus metric that shows the number of Tailscale nodes in the Tailnet.
	NodeCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "tailscale",
		Name:      "nodes_total",
		Help:      "Number of Tailscale nodes in the Tailnet.",
	}, []string{"server"})
)
