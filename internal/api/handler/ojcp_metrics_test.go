package handler

import (
	"context"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/strelov1/freehire/internal/api/ojcp"
	"github.com/strelov1/freehire/internal/platform/observability"
)

func ojcpQualityCount(t *testing.T, signal string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != "freehire_ojcp_rest_search_quality_total" {
			continue
		}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "signal" && label.GetValue() == signal {
					return metric.Counter.GetValue()
				}
			}
		}
	}
	return 0
}

func TestOJCPRESTSearchQualityExcludesInvalidRequestsAndCountsFallback(t *testing.T) {
	signals := []string{"all", "empty", "ignored_filters", "semantic_requested", "semantic_fallback"}
	before := map[string]float64{}
	for _, signal := range signals {
		before[signal] = ojcpQualityCount(t, signal)
	}
	app := ojcpApp(&fakeSearcher{}, fakeOJCPStore{})
	doPost(t, app, "/ojcp/v1/search", `{"query":"go","semantic":true,"location":{"radius_miles":20}}`)
	doPost(t, app, "/ojcp/v1/search", `{`)
	doPost(t, ojcpApp(nil, fakeOJCPStore{}), "/ojcp/v1/search", `{}`)
	// The shared business method is also used by MCP, which has its own counters.
	h := newOJCPHandlers(&fakeSearcher{}, fakeOJCPStore{}, "https://freehire.me", nil)
	if _, err := h.SearchJobs(context.Background(), ojcp.SearchInput{}); err != nil {
		t.Fatal(err)
	}
	for _, signal := range signals {
		if got := ojcpQualityCount(t, signal) - before[signal]; got != 1 {
			t.Errorf("%s = %v, want 1 successful search", signal, got)
		}
	}
}

func TestOJCPRegisteredRESTRouteIsObserved(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: observability.CountErrors(RenderError)})
	app.Use(observability.HTTPMetrics())
	h := newOJCPHandlers(&fakeSearcher{}, fakeOJCPStore{}, "https://freehire.me", nil)
	h.register(app.Group("/api/v1"), middleware{})
	status, _ := doPost(t, app, "/api/v1/ojcp/v1/search", `{}`)
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != "freehire_ojcp_rest_requests_total" {
			continue
		}
		for _, metric := range family.Metric {
			operation, status := "", ""
			for _, label := range metric.Label {
				if label.GetName() == "operation" {
					operation = label.GetValue()
				}
				if label.GetName() == "status" {
					status = label.GetValue()
				}
			}
			if operation == "search_jobs" && status == "200" && metric.Counter.GetValue() > 0 {
				return
			}
		}
	}
	t.Fatal("registered OJCP search route was not observed")
}
