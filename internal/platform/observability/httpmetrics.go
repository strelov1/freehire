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

// httpRouteRequests counts every response by the ROUTE PATTERN it matched, and by nothing else.
//
// This is the separate metric httpRequests' comment reserves rather than the route label it
// refuses. Route ALONE is ~700 series; route x status x method is the tens of thousands that
// counter exists to avoid, so the two questions stay in two metrics: "is the error rate rising"
// is httpRequests, "where is the traffic going" is this one. Adding a second label here puts
// the cardinality back and defeats the split — TestHTTPRouteMetricLabelsAreBounded says so.
var httpRouteRequests = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_http_route_requests_total",
	Help: "API responses by the route pattern they matched (not the request path).",
}, []string{"route"})

// unmatchedRoute is the label for a request no registered route claimed.
//
// It is a constant rather than the path the caller asked for, and that is the whole point of
// routeLabel: see there.
const unmatchedRoute = "unmatched"

// routeLabel returns the registered PATTERN a request matched — "/api/v1/jobs/:id", never
// "/api/v1/jobs/senior-go-engineer-42".
//
// The pattern is a string the app built at registration time, so it is both bounded (the router
// decides the vocabulary, not the caller) and safe to store (it shares nothing with the recycled
// request buffer — the two properties methodLabel had to construct by hand).
//
// Both properties are lost in exactly one case, which is why the guard is here. When no route
// matched, Fiber's Ctx.Route() synthesises a Route whose Path is c.pathOriginal — the caller's
// raw path, aliasing the request buffer. Returning it would hand any caller the power to mint a
// series per request AND repeat the label-corruption failure that answered /metrics with a 500 on
// prod 2026-08-20. That synthetic route is marked by an empty Handlers slice (a registered route
// cannot have one — Fiber panics at registration), so that is what this tests.
//
// A request that reaches no HANDLER but does pass a middleware — a 404 — is a subtler case: Fiber
// leaves c.route pointing at the last middleware that ran, and app.Use registers at "/". Those
// land on the "/" series rather than on unmatchedRoute, which is correct enough (they are counted,
// bounded, and not confusable with a real endpoint) and not worth reaching into unexported state
// to refine.
func routeLabel(c *fiber.Ctx) string {
	route := c.Route()
	if route == nil || len(route.Handlers) == 0 {
		return unmatchedRoute
	}
	return route.Path
}

// methodLabel maps a request method onto one of a fixed set of package-level constants.
//
// It exists for two reasons, and the first one broke production. fasthttp backs c.Method() with
// the request buffer and RECYCLES that buffer between requests, so the string a counter stores
// as a label value mutates under it. On prod 2026-08-20 that surfaced as a label reading "GETT"
// and a /metrics endpoint answering 500 — "collected metric ... was collected before with the
// same name and label values" — because the corrupted label sets collided. Returning a constant
// that shares nothing with the request is the fix; utils.CopyString would also work, but see the
// second reason.
//
// The second: the method is CLIENT-SUPPLIED. Passing it through unbounded lets any caller mint
// label values at will, which is the cardinality hole this metric refuses a route label to
// avoid. Anything outside the set the API actually serves collapses to "other".
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
		httpRequests.WithLabelValues(methodLabel(c.Method()), strconv.Itoa(c.Response().StatusCode())).Inc()
		httpRouteRequests.WithLabelValues(routeLabel(c)).Inc()
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
		httpRequests.WithLabelValues(methodLabel(c.Method()), strconv.Itoa(c.Response().StatusCode())).Inc()
		httpRouteRequests.WithLabelValues(routeLabel(c)).Inc()
		return rendered
	}
}
