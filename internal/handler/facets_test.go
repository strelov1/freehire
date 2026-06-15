package handler

import (
	"context"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search"
)

type fakeFacetCounter struct {
	got search.FacetParams
	res search.FacetResult
	err error
}

func (f *fakeFacetCounter) FacetCounts(_ context.Context, p search.FacetParams) (search.FacetResult, error) {
	f.got = p
	return f.res, f.err
}

func facetsApp(fc facetCounter) *fiber.App {
	h := &API{facets: fc}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/jobs/facets", h.JobFacets)
	return app
}

func TestJobFacets_DisabledReturns503(t *testing.T) {
	app := facetsApp(nil) // search not configured
	status, _ := doGet(t, app, "/jobs/facets")
	if status != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}

func TestJobFacets_PassesFiltersAndRequestsFacets(t *testing.T) {
	fake := &fakeFacetCounter{}
	app := facetsApp(fake)

	status, _ := doGet(t, app, "/jobs/facets?q=golang&seniority=senior&regions=eu")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if fake.got.Query != "golang" {
		t.Errorf("Query = %q, want golang", fake.got.Query)
	}
	groups, ok := fake.got.Filter.([][]string)
	if !ok {
		t.Fatalf("Filter = %#v, want [][]string", fake.got.Filter)
	}
	if !filterHas(groups, `enrichment.seniority = "senior"`) || !filterHas(groups, `regions = "eu"`) {
		t.Errorf("Filter missing facets: %#v", groups)
	}
	// The handler must request a distribution for the facetable attributes,
	// including the boolean and numeric-stat ones.
	for _, want := range []string{"regions", "enrichment.seniority", "enrichment.visa_sponsorship", "enrichment.salary_min"} {
		if !contains(fake.got.Facets, want) {
			t.Errorf("Facets requested = %v, missing %q", fake.got.Facets, want)
		}
	}
}

func TestJobFacets_PrunesNumericDistributionAndRekeys(t *testing.T) {
	// The backend returns facets keyed by the index attribute; the handler re-keys
	// to public param names. Continuous numeric attributes are requested for stats
	// only, so their per-value distribution must not reach the response; boolean
	// and string distributions stay.
	fake := &fakeFacetCounter{res: search.FacetResult{
		Total: 10,
		Facets: map[string]map[string]int64{
			"regions":                     {"eu": 8},
			"enrichment.visa_sponsorship": {"true": 6, "false": 4},
			"enrichment.salary_min":       {"50000": 3, "60000": 2},
		},
		Stats: map[string]search.FacetStat{"enrichment.salary_min": {Min: 50000, Max: 60000}},
	}}
	app := facetsApp(fake)

	_, body := doGet(t, app, "/jobs/facets")
	facets := body["data"].(map[string]any)["facets"].(map[string]any)

	if _, present := facets["salary_min"]; present {
		t.Error("numeric distribution salary_min should be pruned from facets")
	}
	if _, present := facets["regions"]; !present {
		t.Error("string distribution regions should be kept")
	}
	// Re-keyed from the index attribute to the public param name.
	if _, present := facets["visa_sponsorship"]; !present {
		t.Error("boolean distribution should be kept and re-keyed to visa_sponsorship")
	}
	// Numeric stats survive and are re-keyed to the public param name.
	stats := body["data"].(map[string]any)["stats"].(map[string]any)
	if _, present := stats["salary_min"]; !present {
		t.Error("stats should be kept and re-keyed to salary_min")
	}
}

func TestJobFacets_ShapesResponse(t *testing.T) {
	fake := &fakeFacetCounter{res: search.FacetResult{
		Total:  1234,
		Facets: map[string]map[string]int64{"regions": {"eu": 800}},
		Stats:  map[string]search.FacetStat{"enrichment.salary_min": {Min: 0, Max: 400000}},
	}}
	app := facetsApp(fake)

	status, body := doGet(t, app, "/jobs/facets")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("body.data = %#v, want object", body["data"])
	}
	if data["total"].(float64) != 1234 {
		t.Errorf("total = %v, want 1234", data["total"])
	}
	facets := data["facets"].(map[string]any)
	regions := facets["regions"].(map[string]any)
	if regions["eu"].(float64) != 800 {
		t.Errorf("facets.regions.eu = %v, want 800", regions["eu"])
	}
	// stats is re-keyed to the public param name (salary_min, not the dot-path).
	stats := data["stats"].(map[string]any)
	if stats["salary_min"].(map[string]any)["max"].(float64) != 400000 {
		t.Errorf("stats max = %v, want 400000", stats["salary_min"])
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
