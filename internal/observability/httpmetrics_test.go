package observability

import (
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
