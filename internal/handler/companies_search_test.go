package handler

import (
	"context"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search"
)

type fakeCompanySearcher struct {
	got    search.CompanySearchParams
	called bool
	res    search.CompanyResult
	err    error
}

func (f *fakeCompanySearcher) SearchCompanies(_ context.Context, p search.CompanySearchParams) (search.CompanyResult, error) {
	f.called = true
	f.got = p
	return f.res, f.err
}

// companyApp wires only the company searcher (no Postgres), so the Meili routing and
// projection can be exercised without a database — the branches that fall through to
// Postgres are covered by the integration tests.
func companyApp(cs companySearcher) *fiber.App {
	h := &companiesHandlers{companySearch: cs}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/companies", h.ListCompanies)
	return app
}

// hasCompanyGroup reports whether the AND-of-ORs filter contains a group holding
// the given fragment.
func hasCompanyGroup(gs [][]string, fragment string) bool {
	for _, g := range gs {
		for _, f := range g {
			if f == fragment {
				return true
			}
		}
	}
	return false
}

func TestListCompanies_QueryRoutesToMeiliPreservingRankOrder(t *testing.T) {
	// The handler relays Meilisearch's relevance ranking verbatim — the exact-name
	// match "arb" leads even though higher-volume substring matches follow.
	fake := &fakeCompanySearcher{res: search.CompanyResult{
		Hits: []search.CompanyDocument{
			{Slug: "arb", Name: "arb", JobCount: 2},
			{Slug: "arbor", Name: "Arbor", JobCount: 40},
			{Slug: "carbon", Name: "Carbon", JobCount: 99},
		},
		Total: 3,
	}}
	app := companyApp(fake)

	status, body := doGet(t, app, "/companies?q=arb")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if fake.got.Query != "arb" {
		t.Errorf("query passed to Meili = %q, want arb", fake.got.Query)
	}
	data, _ := body["data"].([]any)
	if len(data) != 3 {
		t.Fatalf("data len = %d, want 3", len(data))
	}
	first, _ := data[0].(map[string]any)
	if first["slug"] != "arb" {
		t.Errorf("first result slug = %v, want arb (handler must preserve Meili rank order)", first["slug"])
	}
	// The projected row keeps the ListCompaniesRow wire shape.
	if _, ok := first["job_count"]; !ok {
		t.Errorf("projected row missing job_count: %v", first)
	}
	meta, _ := body["meta"].(map[string]any)
	if got, _ := meta["total"].(float64); got != 3 {
		t.Errorf("meta.total = %v, want 3 (from Meili estimatedTotalHits)", meta["total"])
	}
}

func TestListCompanies_FacetsBuildMeiliFilterAndRoute(t *testing.T) {
	// A facet-only request (no q) still routes to Meili and each facet ANDs as its own
	// OR-group, with the param→attribute mapping applied (company_type → company_types).
	fake := &fakeCompanySearcher{}
	app := companyApp(fake)

	status, _ := doGet(t, app, "/companies?regions=europe&company_type=startup")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !fake.called {
		t.Fatal("a facet request should route to Meili")
	}
	gs, ok := fake.got.Filter.([][]string)
	if !ok {
		t.Fatalf("filter = %T, want [][]string", fake.got.Filter)
	}
	if !hasCompanyGroup(gs, `regions = "europe"`) || !hasCompanyGroup(gs, `company_types = "startup"`) {
		t.Errorf("filter %v missing expected facet groups", gs)
	}
}

func TestListCompanies_ReportsIgnoredParams(t *testing.T) {
	// The company filter has its own vocabulary: `seniority` is a jobs facet and
	// does nothing here. Without a warning the company list that comes back
	// reads as an answer to the narrower question that was asked.
	fake := &fakeCompanySearcher{}
	app := companyApp(fake)

	// `collections` is a real company facet, so the request routes to Meili
	// rather than falling through to the Postgres path this harness lacks.
	_, body := doGet(t, app, "/companies?collections=yc&seniority=senior")

	meta, _ := body["meta"].(map[string]any)
	ignored, _ := meta["ignored_params"].([]any)
	if len(ignored) != 1 {
		t.Fatalf("meta.ignored_params = %v, want one entry", meta["ignored_params"])
	}
	if first, _ := ignored[0].(map[string]any); first["param"] != "seniority" {
		t.Errorf("ignored_params[0] = %v, want seniority", first)
	}
}
