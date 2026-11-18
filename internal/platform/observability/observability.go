// Package observability wires optional OpenTelemetry exception logs and metrics
// for the HTTP server and standalone workers. Error reporting is opt-in: without
// an OTLP endpoint it is a no-op.
package observability

import (
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metricsServerTimeout bounds every phase of a connection to the metrics
// listener (header read, full read, write, and idle keep-alive), so a client
// that opens a connection and stalls cannot hold it — and the goroutines and
// file descriptors backing it — open indefinitely.
const metricsServerTimeout = 10 * time.Second

// metricsServer builds the *http.Server StartMetricsServer runs, split out so
// its timeout configuration is unit-testable without opening a real listener.
func metricsServer(port string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: metricsServerTimeout,
		ReadTimeout:       metricsServerTimeout,
		WriteTimeout:      metricsServerTimeout,
		IdleTimeout:       metricsServerTimeout,
	}
}

// StartMetricsServer serves Prometheus /metrics on its own listener, separate
// from the main API port. A dedicated port (rather than a route on the public
// API) means a firewall rule scoped to a scraper's IP exposes only metrics,
// never the rest of the API surface. No-op when port is empty.
func StartMetricsServer(port string) {
	if port == "" {
		return
	}
	srv := metricsServer(port)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("metrics server on :%s stopped: %v", port, err)
		}
	}()
}
