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

// StartMetricsServer serves Prometheus /metrics on its own listener, separate
// from the main API port. A dedicated port (rather than a route on the public
// API) means a firewall rule scoped to a scraper's IP exposes only metrics,
// never the rest of the API surface. No-op when port is empty.
func StartMetricsServer(port string) {
	if port == "" {
		return
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	go func() {
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Printf("metrics server on :%s stopped: %v", port, err)
		}
	}()
}
