package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// windowedListRoutes are the list endpoints whose offset a caller chooses and whose cost
// therefore grows with it. Each is driven below through the register that really mounts it.
//
// The handlers behind them are ZERO-VALUED — no queries, no services. That is the assertion,
// not a shortcut: a request that reaches such a handler nil-dereferences into recover and
// answers 500. So a 400 proves the window refused the page BEFORE any database work, and a
// 500 on the control request proves the route was reached at all, i.e. that the test is
// driving something real rather than a 404 from a path typo.
var windowedListRoutes = []struct {
	name  string
	mount func() *fiber.App
	path  string
}{
	{"GET /jobs", func() *fiber.App { return mountWindowed((&jobsHandlers{}).register) }, "/api/v1/jobs"},
	{"GET /companies", func() *fiber.App { return mountWindowed((&companiesHandlers{}).register) }, "/api/v1/companies"},
	{"GET /companies/:slug", func() *fiber.App { return mountWindowed((&companiesHandlers{}).register) }, "/api/v1/companies/acme"},
	{"GET /jobs/:slug/copies", func() *fiber.App { return mountWindowed((&jobsHandlers{}).register) }, "/api/v1/jobs/a-job/copies"},
	{"GET /companies/:slug/feedback", func() *fiber.App {
		return mountWindowed((&companyFeedbackHandlers{}).registerPublic)
	}, "/api/v1/companies/acme/feedback"},
}

// mountWindowed builds an app from a real register. The throttler is nil, which
// ratelimit.Middleware treats as "no backend configured" and fails open, so the limiter can
// never be what refuses a request here — the window has to be.
func mountWindowed(register func(fiber.Router, middleware)) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(recover.New())
	passthrough := func(c *fiber.Ctx) error { return c.Next() }
	register(app.Group("/api/v1"), middleware{
		optional:  passthrough,
		key:       passthrough,
		cookie:    passthrough,
		moderator: passthrough,
	})
	return app
}

func windowedGet(t *testing.T, app *fiber.App, path, query string) int {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path+"?"+query, nil))
	if err != nil {
		t.Fatalf("GET %s?%s: %v", path, query, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// TestDeepOffsetIsRefusedBeforeAnyDatabaseWork is the regression test for the 2026-09-14
// outage, stated as the property that would have prevented it: a deep-offset request must
// not reach the database.
//
// Rate limiting cannot stand in for this and the incident proved it — the crawler collected
// 2,406 × 429 from the per-IP limiter and still exhausted the pool, because a budget in
// requests per minute says nothing about an endpoint whose work per request the caller sets.
func TestDeepOffsetIsRefusedBeforeAnyDatabaseWork(t *testing.T) {
	for _, route := range windowedListRoutes {
		t.Run(route.name, func(t *testing.T) {
			// The offset the crawler actually reached.
			got := windowedGet(t, route.mount(), route.path, "limit=100&offset=179500")
			if got != fiber.StatusBadRequest {
				t.Fatalf("deep offset = %d, want 400. A 500 means the handler was entered and the "+
					"page would have been queried; any 2xx means it was served.", got)
			}
		})
	}
}

// TestWindowedListRoutesAreReallyDriven is the control for the test above, and the reason it
// can be trusted. Without it, a mistyped path would 404 — not 400 — but a future edit that
// turned 404 into 400 would make the guard pass on nothing.
//
// A shallow request must NOT be refused: it gets past the window into the zero-valued
// handler, which recover turns into 500. Anything else means the test is not exercising the
// route it names.
func TestWindowedListRoutesAreReallyDriven(t *testing.T) {
	for _, route := range windowedListRoutes {
		t.Run(route.name, func(t *testing.T) {
			got := windowedGet(t, route.mount(), route.path, "limit=20&offset=0")
			if got == fiber.StatusBadRequest {
				t.Fatalf("first page = 400 — the window is refusing an ordinary request")
			}
			if got == fiber.StatusNotFound {
				t.Fatalf("first page = 404 — this test is not driving the route it names, so its " +
					"sibling's 400 would prove nothing")
			}
		})
	}
}

// TestWindowBoundaryIsServedOnEveryListRoute pins the other edge per route. A window that
// refused its own last page would be a silent off-by-one on five public endpoints at once.
func TestWindowBoundaryIsServedOnEveryListRoute(t *testing.T) {
	for _, route := range windowedListRoutes {
		t.Run(route.name, func(t *testing.T) {
			got := windowedGet(t, route.mount(), route.path, "limit=100&offset=9900")
			if got == fiber.StatusBadRequest {
				t.Fatalf("offset+limit == maxPageWindow (%d) = 400, want the page to be admitted", maxPageWindow)
			}
		})
	}
}
