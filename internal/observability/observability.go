// Package observability wires optional error reporting (Sentry) for the Go HTTP
// server and the standalone workers. It is opt-in and env-gated: without a DSN it
// is a no-op, mirroring the other optional integrations (search, blobstore) so an
// unconfigured deployment runs unchanged.
package observability

import (
	"log"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// flushTimeout bounds how long the returned flush waits for buffered events to
// reach Sentry. Short-lived cron workers call it on exit, so it stays small enough
// not to stall shutdown yet large enough for one delivery round trip.
const flushTimeout = 2 * time.Second

// Init initializes Sentry error reporting and returns a flush to run before the
// process exits (delivers buffered events). When dsn is empty it initializes
// nothing and returns a no-op flush, so an unconfigured deployment is unaffected.
// A malformed DSN is returned as an error so a misconfigured process fails fast.
//
// PII is off by default (SendDefaultPII false) and tracing is disabled: this is an
// errors-only integration — no request bodies, cookies, or auth headers are shipped.
func Init(dsn, environment string) (flush func(), err error) {
	if dsn == "" {
		return func() {}, nil
	}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:            dsn,
		Environment:    environment,
		EnableTracing:  false,
		SendDefaultPII: false,
	}); err != nil {
		return nil, err
	}
	return func() { sentry.Flush(flushTimeout) }, nil
}

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
