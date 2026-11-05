package observability

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus"
)

func ojcpMetricSum(t *testing.T, name string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var total float64
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			if metric.Counter != nil {
				total += metric.Counter.GetValue()
			}
			if metric.Histogram != nil {
				total += float64(metric.Histogram.GetSampleCount())
			}
		}
	}
	return total
}

func TestOJCPMetricsObserveFinalResponsesExactlyOnce(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: CountErrors(fiber.DefaultErrorHandler)})
	app.Use(HTTPMetrics())
	app.Use(recover.New())
	app.Post("/api/v1/ojcp/v1/search", func(c *fiber.Ctx) error { return c.SendString("{}") })
	app.Get("/api/v1/ojcp/v1/jobs/:slug", func(c *fiber.Ctx) error {
		if c.Params("slug") == "panic" {
			panic("fixture")
		}
		return fiber.ErrNotFound
	})
	app.Get("/api/v1/ojcp/mcp", func(c *fiber.Ctx) error { return c.SendString("MCP") })
	for _, name := range []string{"freehire_ojcp_rest_requests_total", "freehire_ojcp_rest_response_bytes", "freehire_ojcp_rest_duration_seconds"} {
		before := ojcpMetricSum(t, name)
		for _, req := range []struct {
			method, path string
			status       int
		}{
			{"POST", "/api/v1/ojcp/v1/search", 200}, {"GET", "/api/v1/ojcp/v1/jobs/private-slug", 404},
			{"GET", "/api/v1/ojcp/v1/jobs/panic", 500}, {"GET", "/api/v1/ojcp/mcp", 200},
		} {
			resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), req.method, req.path, nil))
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != req.status {
				t.Fatalf("status %d, want %d", resp.StatusCode, req.status)
			}
		}
		if got := ojcpMetricSum(t, name) - before; got != 3 {
			t.Errorf("%s recorded %v responses, want 3 (REST only)", name, got)
		}
	}
	families, _ := prometheus.DefaultGatherer.Gather()
	statuses := map[string]bool{}
	for _, family := range families {
		if family.GetName() != "freehire_ojcp_rest_requests_total" {
			continue
		}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				switch label.GetName() {
				case "operation":
					if v := label.GetValue(); v != "search_jobs" && v != "get_job_detail" {
						t.Errorf("unbounded operation %q", v)
					}
				case "status":
					statuses[label.GetValue()] = true
				default:
					t.Errorf("unexpected label %q", label.GetName())
				}
			}
		}
	}
	for _, status := range []string{"200", "404", "500"} {
		if !statuses[status] {
			t.Errorf("missing final status %s", status)
		}
	}
}
