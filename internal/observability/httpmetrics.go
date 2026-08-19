package observability

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// httpRequests counts every response the API sends, by method and status code.
//
// Labelled by method and status ONLY, deliberately. The obvious third label is the route, and
// it is the one this must not carry: the app registers ~700 routes, so route x status x method
// is tens of thousands of series on a single-target Prometheus. The question this metric exists
// to answer — "is the error rate rising" — does not need the route, and the question that does
// ("which endpoint") is already answered by the per-request log line beside it. If a future
// need justifies the cardinality, add a SEPARATE metric scoped to 5xx rather than widening this
// one.
//
// The status label is the numeric code rather than a 2xx/5xx class: the codes this API actually
// emits are a short list (200, 301, 400, 401, 403, 404, 429, 500, 503), so the cardinality is
// the same either way, and 429 vs 500 is exactly the distinction an incident turns on — the
// 2026-08-19 rate-limiter incident and the schema outage would have looked identical under a
// class label.
var httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_http_requests_total",
	Help: "API responses by method and HTTP status code.",
}, []string{"method", "status"})

// HTTPMetrics counts responses that completed without an error. It is HALF the wiring: a
// handler returning an error has no status yet when this unwinds, because Fiber renders it in
// the ErrorHandler AFTER the middleware chain has fully unwound. Reading the status here would
// record 200 for every 404 and every 500 — the responses the counter exists to see. Those are
// counted by CountErrors instead, and the err != nil skip below is what keeps the two from
// double-counting the same request.
//
// This does not replace the site-alert watchdog and is not the reason it exists. That check
// polls a real endpoint every two minutes and pages on two consecutive failures — it caught the
// 2026-08-19 outage in two minutes, which no scrape interval improves on. What it cannot see is
// a RATE: 0.5% of requests failing is invisible to a poll that keeps succeeding, and that is
// what this counter is for.
func HTTPMetrics() fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()
		if err != nil {
			// CountErrors will record it once the ErrorHandler has chosen the status.
			return err
		}
		httpRequests.WithLabelValues(c.Method(), strconv.Itoa(c.Response().StatusCode())).Inc()
		return nil
	}
}

// CountErrors wraps the app's ErrorHandler so an error-derived response is counted with the
// status that was actually sent.
//
// Deliberately a decorator rather than a copy of the mapping: handler.RenderError resolves the
// status through codedError and classify, and reproducing that here would be a second
// implementation of one rule — the failure mode this codebase has already paid for once, in the
// four disagreeing legal-form vocabularies. The wrapper asks the real handler what it did
// instead of predicting it.
func CountErrors(next fiber.ErrorHandler) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		rendered := next(c, err)
		httpRequests.WithLabelValues(c.Method(), strconv.Itoa(c.Response().StatusCode())).Inc()
		return rendered
	}
}
