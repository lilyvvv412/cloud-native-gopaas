package metrics

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gopaas_http_requests_total",
		Help: "Total HTTP requests handled by the API gateway",
	}, []string{"method", "path", "status"})

	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gopaas_http_request_duration_seconds",
		Help:    "HTTP request latency",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	RateLimitedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gopaas_rate_limited_total",
		Help: "Requests rejected by rate limiting",
	})

	CircuitOpenTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gopaas_circuit_open_total",
		Help: "Calls short-circuited by the circuit breaker",
	}, []string{"endpoint"})

	RPCCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gopaas_rpc_calls_total",
		Help: "go-micro RPC calls",
	}, []string{"service", "endpoint", "result"})
)

// Boot starts a Prometheus /metrics HTTP endpoint on the given port.
func Boot(port int) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	addr := "0.0.0.0:" + strconv.Itoa(port)
	go func() {
		log.Printf("prometheus metrics listening on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("metrics server stopped: %v", err)
		}
	}()
}

// ObserveHTTP records gateway request metrics.
func ObserveHTTP(method, path, status string, started time.Time) {
	RequestsTotal.WithLabelValues(method, path, status).Inc()
	RequestDuration.WithLabelValues(method, path).Observe(time.Since(started).Seconds())
}
