package observability

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// httpRequests counts every response the API sends, by method and status code.
//
// Labelled by method and status ONLY: a route label (~700 routes) would make tens of thousands
// of series. "Which endpoint" is answered by the request log; for route-level counts use
// httpRouteRequests. The status is the numeric code, not a 2xx/5xx class, because 429 vs 500
// is the distinction an incident turns on.
var httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_http_requests_total",
	Help: "API responses by method and HTTP status code.",
}, []string{"method", "status"})

// httpRouteRequests counts every response by the ROUTE PATTERN it matched, and by nothing else.
//
// Route alone is ~700 series; adding a second label brings back the cardinality httpRequests
// avoids (TestHTTPRouteMetricLabelsAreBounded enforces this).
var httpRouteRequests = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_http_route_requests_total",
	Help: "API responses by the route pattern they matched (not the request path).",
}, []string{"route"})

// httpDuration observes how long each response took, by route pattern.
//
// The counters above cannot see a request that takes two minutes and then succeeds.
// Route is the only label: a histogram multiplies series by its bucket count.
//
// Buckets run to 30s, the API pool's statement_timeout (cmd/server), and start at 5ms so the
// healthy single-digit-millisecond case is not already in the first bucket.
var httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "freehire_http_request_duration_seconds",
	Help:    "How long each API response took, by the route pattern it matched.",
	Buckets: []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
}, []string{"route"})

// unmatchedRoute is the label for a request no registered route claimed, so the caller's
// path never becomes a label value (see routeLabel).
const unmatchedRoute = "unmatched"

// routeLabel returns the registered PATTERN a request matched ("/api/v1/jobs/:id", never
// "/api/v1/jobs/senior-go-engineer-42").
//
// The pattern is bounded (the router decides the vocabulary) and safe to store (it does not
// alias the recycled request buffer). When no route matched, Fiber's Ctx.Route() synthesises
// a Route whose Path is the caller's raw path, which would mint a series per request and
// corrupt labels. That synthetic route has an empty Handlers slice (a registered route
// cannot), which is what this tests.
//
// A 404 that passes a middleware lands on the "/" series, since app.Use registers at "/";
// that is bounded and not worth reaching into unexported Fiber state to refine.
func routeLabel(c *fiber.Ctx) string {
	route := c.Route()
	if route == nil || len(route.Handlers) == 0 {
		return unmatchedRoute
	}
	return route.Path
}

// methodLabel maps a request method onto one of a fixed set of package-level constants.
//
// fasthttp backs c.Method() with the request buffer and RECYCLES it, so a stored label string
// mutates ("GETT") and corrupts /metrics. The method is also CLIENT-SUPPLIED, so anything
// outside the set the API serves collapses to "other" to keep cardinality bounded.
func methodLabel(m string) string {
	switch m {
	case fiber.MethodGet:
		return fiber.MethodGet
	case fiber.MethodPost:
		return fiber.MethodPost
	case fiber.MethodPut:
		return fiber.MethodPut
	case fiber.MethodPatch:
		return fiber.MethodPatch
	case fiber.MethodDelete:
		return fiber.MethodDelete
	case fiber.MethodHead:
		return fiber.MethodHead
	case fiber.MethodOptions:
		return fiber.MethodOptions
	default:
		return "other"
	}
}

// HTTPMetrics counts responses that completed without an error. A handler returning an error
// has no status yet when this unwinds (Fiber renders it in the ErrorHandler after the chain
// unwinds), so those are counted by CountErrors; the err != nil skip below prevents double
// counting.
//
// It complements the site-alert watchdog, which polls an endpoint but cannot see a RATE such
// as 0.5% of requests failing.
func HTTPMetrics() fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()
		if err != nil {
			// CountErrors will record it once the ErrorHandler has chosen the status.
			return err
		}
		recordResponse(c)
		return nil
	}
}

// recordResponse tallies the response Fiber has just sent into the Prometheus counters and
// the in-process request window ErrorRate reads. Shared by HTTPMetrics and CountErrors so a
// new consumer needs one call site.
func recordResponse(c *fiber.Ctx) {
	status := c.Response().StatusCode()
	route := routeLabel(c)
	httpRequests.WithLabelValues(methodLabel(c.Method()), strconv.Itoa(status)).Inc()
	httpRouteRequests.WithLabelValues(route).Inc()
	// fasthttp stamps the arrival time on the RequestCtx, so this measures from ACCEPT, not
	// from when a middleware first looked at the request.
	httpDuration.WithLabelValues(route).Observe(time.Since(c.Context().Time()).Seconds())
	observeOJCPResponse(c, route)
	RecordRequest(status)
}

// CountErrors wraps the app's ErrorHandler so an error-derived response is counted with the
// status that was actually sent.
//
// A decorator rather than a copy of the mapping: handler.RenderError resolves the status, and
// reproducing it here would be a second implementation of one rule.
func CountErrors(next fiber.ErrorHandler) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		rendered := next(c, err)
		recordResponse(c)
		return rendered
	}
}
