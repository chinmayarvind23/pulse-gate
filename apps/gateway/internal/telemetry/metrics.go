package telemetry

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	Requests        *prometheus.CounterVec
	Duration        prometheus.Histogram
	Duplicates      prometheus.Counter
	AdmissionErrors prometheus.Counter
}

// NewMetrics keeps the metric vocabulary small enough to explain in an interview.
// Histograms are used for latency because averages hide the tail behavior that drives
// webhook timeout and retry risk.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		Requests:        prometheus.NewCounterVec(prometheus.CounterOpts{Name: "pulsegate_http_requests_total", Help: "HTTP requests by result."}, []string{"code"}),
		Duration:        prometheus.NewHistogram(prometheus.HistogramOpts{Name: "pulsegate_ack_duration_seconds", Help: "Webhook acknowledgement latency.", Buckets: prometheus.DefBuckets}),
		Duplicates:      prometheus.NewCounter(prometheus.CounterOpts{Name: "pulsegate_duplicates_total", Help: "Duplicate events suppressed before enqueue."}),
		AdmissionErrors: prometheus.NewCounter(prometheus.CounterOpts{Name: "pulsegate_admission_errors_total", Help: "Redis admission failures."}),
	}
	reg.MustRegister(m.Requests, m.Duration, m.Duplicates, m.AdmissionErrors)
	return m
}
