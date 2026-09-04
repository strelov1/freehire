package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// count reads one series of the counter, or 0 when that label pair has never been touched.
func count(t *testing.T, method, status string) float64 {
	t.Helper()
	return testutil.ToFloat64(httpRequests.WithLabelValues(method, status))
}

func TestHTTPMetricsCountsByMethodAndStatus(t *testing.T) {
	app := fiber.New()
	app.Use(HTTPMetrics())
	app.Get("/ok", func(c *fiber.Ctx) error { return c.SendString("fine") })
	app.Post("/created", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusCreated) })

	before200 := count(t, "GET", "200")
	before201 := count(t, "POST", "201")

	for range 3 {
		resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/ok", nil))
		if err != nil {
			t.Fatalf("GET /ok: %v", err)
		}
		resp.Body.Close()
	}
	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), fiber.MethodPost, "/created", nil))
	if err != nil {
		t.Fatalf("POST /created: %v", err)
	}
	resp.Body.Close()

	if got := count(t, "GET", "200") - before200; got != 3 {
		t.Errorf("GET 200 counted %v, want 3", got)
	}
	if got := count(t, "POST", "201") - before201; got != 1 {
		t.Errorf("POST 201 counted %v, want 1", got)
	}
}

// TestHTTPMetricsCountsAPanicAs500 is the reason the middleware is mounted outside recover.New.
// A panic is not a status until recover has turned it into one; a counter mounted inside would
// observe whatever the response held before the handler blew up — undercounting exactly the
// failures worth counting.
func TestHTTPMetricsCountsAPanicAs500(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: CountErrors(fiber.DefaultErrorHandler)})
	app.Use(HTTPMetrics())
	app.Use(recover.New())
	app.Get("/boom", func(c *fiber.Ctx) error { panic("boom") })

	before := count(t, "GET", "500")

	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/boom", nil))
	if err != nil {
		t.Fatalf("GET /boom: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("fixture: status %d, want 500", resp.StatusCode)
	}

	if got := count(t, "GET", "500") - before; got != 1 {
		t.Errorf("panic counted as 500 %v times, want 1 — the middleware is probably mounted "+
			"inside recover.New, where it cannot see the status the client got", got)
	}
}

// TestHTTPMetricsCountsAnErrorHandlerStatus covers the shape this API actually uses: handlers
// return an error and Fiber's ErrorHandler renders the envelope. The status is set during that
// render, after the handler returned, so reading it before c.Next() unwinds would record 200.
func TestHTTPMetricsCountsAnErrorHandlerStatus(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: CountErrors(fiber.DefaultErrorHandler)})
	app.Use(HTTPMetrics())
	app.Get("/missing", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotFound, "no such job")
	})

	before := count(t, "GET", "404")

	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/missing", nil))
	if err != nil {
		t.Fatalf("GET /missing: %v", err)
	}
	resp.Body.Close()

	if got := count(t, "GET", "404") - before; got != 1 {
		t.Errorf("error-handler 404 counted %v times, want 1", got)
	}
}

// TestHTTPMetricsCountsEachRequestOnce pins the split: the middleware skips a request that
// returned an error so CountErrors can record it with the rendered status, and exactly one of
// the two fires. A request counted twice would double every error rate the metric reports.
func TestHTTPMetricsCountsEachRequestOnce(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: CountErrors(fiber.DefaultErrorHandler)})
	app.Use(HTTPMetrics())
	app.Get("/fail", func(c *fiber.Ctx) error { return fiber.NewError(fiber.StatusBadRequest, "no") })

	before400 := count(t, "GET", "400")
	before200 := count(t, "GET", "200")

	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/fail", nil))
	if err != nil {
		t.Fatalf("GET /fail: %v", err)
	}
	resp.Body.Close()

	if got := count(t, "GET", "400") - before400; got != 1 {
		t.Errorf("400 counted %v times, want exactly 1", got)
	}
	if got := count(t, "GET", "200") - before200; got != 0 {
		t.Errorf("the same request also counted as 200 %v times — the middleware did not skip "+
			"the error path, so every error inflates the success count too", got)
	}
}

// TestHTTPMetricsLabelsAreBounded guards the decision NOT to label by route. The app registers
// ~700 routes; route x status x method would be tens of thousands of series on a single-target
// Prometheus. This asserts the metric's label set, so widening it becomes a deliberate edit
// rather than a convenience someone adds mid-incident.
func TestHTTPMetricsLabelsAreBounded(t *testing.T) {
	app := fiber.New()
	app.Use(HTTPMetrics())
	app.Get("/jobs/:slug", func(c *fiber.Ctx) error { return c.SendString("ok") })

	for _, slug := range []string{"a-1", "b-2", "c-3"} {
		resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/jobs/"+slug, nil))
		if err != nil {
			t.Fatalf("GET /jobs/%s: %v", slug, err)
		}
		resp.Body.Close()
	}

	// Three distinct paths, one series: the path is not a label.
	if n := testutil.CollectAndCount(httpRequests); n > 24 {
		t.Errorf("the counter holds %d series after three distinct paths — a path or route label "+
			"has crept in, and at ~700 routes that is tens of thousands of series", n)
	}
}

// routeCount reads one series of the route counter, or 0 when that route has never been touched.
func routeCount(t *testing.T, route string) float64 {
	t.Helper()
	return testutil.ToFloat64(httpRouteRequests.WithLabelValues(route))
}

// TestRouteMetricCountsThePatternNotThePath is the property that makes a route label affordable
// at all: three requests to three different job slugs are three increments of ONE series, because
// the label is the registered pattern and not what the caller typed.
func TestRouteMetricCountsThePatternNotThePath(t *testing.T) {
	app := fiber.New()
	app.Use(HTTPMetrics())
	app.Get("/jobs/:slug", func(c *fiber.Ctx) error { return c.SendString("ok") })

	before := routeCount(t, "/jobs/:slug")

	for _, slug := range []string{"a-1", "b-2", "c-3"} {
		resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/jobs/"+slug, nil))
		if err != nil {
			t.Fatalf("GET /jobs/%s: %v", slug, err)
		}
		resp.Body.Close()
	}

	if got := routeCount(t, "/jobs/:slug") - before; got != 3 {
		t.Errorf("/jobs/:slug counted %v, want 3 — the label is probably the request path, which "+
			"is one series per slug and unbounded", got)
	}
	if got := routeCount(t, "/jobs/a-1"); got != 0 {
		t.Errorf("a literal path minted its own series (%v) — the label is not the route pattern", got)
	}
}

// TestRouteMetricCountsAnErrorHandlerResponse covers the half that does not pass through the
// middleware. Fiber renders a returned error in the ErrorHandler after the chain has unwound, so
// without CountErrors doing its own increment every failing endpoint would read as no traffic.
func TestRouteMetricCountsAnErrorHandlerResponse(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: CountErrors(fiber.DefaultErrorHandler)})
	app.Use(HTTPMetrics())
	app.Get("/gone", func(c *fiber.Ctx) error { return fiber.NewError(fiber.StatusNotFound, "no") })

	before := routeCount(t, "/gone")

	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/gone", nil))
	if err != nil {
		t.Fatalf("GET /gone: %v", err)
	}
	resp.Body.Close()

	if got := routeCount(t, "/gone") - before; got != 1 {
		t.Errorf("/gone counted %v, want exactly 1", got)
	}
}

// TestRouteLabelRefusesAnUnmatchedPath is the cardinality guard AND the buffer-aliasing guard in
// one. When nothing matched, Fiber's Ctx.Route() hands back a synthetic Route carrying the
// caller's raw path — so passing it through would let any caller mint a series per request and
// would store a string backed by the recycled request buffer, which is what answered /metrics
// with a 500 on prod 2026-08-20.
func TestRouteLabelRefusesAnUnmatchedPath(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: CountErrors(fiber.DefaultErrorHandler)})
	app.Get("/known", func(c *fiber.Ctx) error { return c.SendString("ok") })

	before := routeCount(t, unmatchedRoute)

	for _, path := range []string{"/nope-1", "/nope-2"} {
		resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Fatalf("fixture: GET %s answered %d, want 404", path, resp.StatusCode)
		}
	}

	if got := routeCount(t, unmatchedRoute) - before; got != 2 {
		t.Errorf("unmatched requests counted %v on the %q series, want 2", got, unmatchedRoute)
	}
	if got := routeCount(t, "/nope-1"); got != 0 {
		t.Errorf("an unmatched path minted its own series (%v) — the caller can now grow the "+
			"metric without bound, one request at a time", got)
	}
}

// TestRouteMetricLabelsAreBounded pins the split between this metric and httpRequests. Route is
// ~700 series on its own; the moment a status or method label joins it, it becomes the tens of
// thousands that the other counter refuses a route label to avoid, and having two metrics stops
// buying anything.
func TestRouteMetricLabelsAreBounded(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: CountErrors(fiber.DefaultErrorHandler)})
	app.Use(HTTPMetrics())
	app.Get("/one", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Post("/one", func(c *fiber.Ctx) error { return fiber.NewError(fiber.StatusBadRequest, "no") })

	before := testutil.CollectAndCount(httpRouteRequests)

	for _, req := range []*http.Request{
		httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/one", nil),
		httptest.NewRequestWithContext(t.Context(), fiber.MethodPost, "/one", nil),
	} {
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s /one: %v", req.Method, err)
		}
		resp.Body.Close()
	}

	// Two methods, two statuses, one route: one new series.
	if got := testutil.CollectAndCount(httpRouteRequests) - before; got > 1 {
		t.Errorf("one route produced %d new series — a status or method label has crept in, and "+
			"at ~700 routes that multiplies into the cardinality this metric was split to avoid", got)
	}
}

// TestMethodLabelDoesNotAliasTheRequestBuffer is the regression test for a live prod failure.
//
// fasthttp backs c.Method() with the request buffer and recycles it, so a label value taken
// straight from it mutates after the counter has stored it. On 2026-08-20 that produced a label
// reading "GETT" and a /metrics endpoint answering 500, because the corrupted label sets
// collided with the intact ones. The check is that the returned string shares no backing array
// with the input: mutating the caller's bytes must not change the label.
func TestMethodLabelDoesNotAliasTheRequestBuffer(t *testing.T) {
	buf := []byte("GET")
	label := methodLabel(string(buf))
	buf[2] = 'X' // fasthttp reusing the buffer for the next request

	if label != fiber.MethodGet {
		t.Errorf("label became %q after the caller's buffer changed — it aliases the request "+
			"buffer, which is what broke /metrics on prod", label)
	}
}

// TestMethodLabelBoundsClientSuppliedValues closes the cardinality hole the method label opens:
// the method comes from the client, so an unbounded passthrough lets any caller mint label
// values — exactly what refusing a route label was meant to prevent.
func TestMethodLabelBoundsClientSuppliedValues(t *testing.T) {
	for _, m := range []string{fiber.MethodGet, fiber.MethodPost, fiber.MethodDelete} {
		if got := methodLabel(m); got != m {
			t.Errorf("methodLabel(%q) = %q, want it preserved", m, got)
		}
	}
	for _, m := range []string{"BREW", "", "get", "GETT", "PROPFIND"} {
		if got := methodLabel(m); got != "other" {
			t.Errorf("methodLabel(%q) = %q, want \"other\" — an unknown method must not become "+
				"its own series", m, got)
		}
	}
}
