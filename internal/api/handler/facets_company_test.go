package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// fakeCompanyNames answers the display-name read from a fixed map and records the
// slugs it was asked for.
type fakeCompanyNames struct {
	names map[string]string
	got   [][]string
	err   error
}

func (f *fakeCompanyNames) CompanyNamesBySlugs(_ context.Context, slugs []string) ([]db.CompanyNamesBySlugsRow, error) {
	f.got = append(f.got, slugs)
	if f.err != nil {
		return nil, f.err
	}
	var out []db.CompanyNamesBySlugsRow
	for _, s := range slugs {
		if n, ok := f.names[s]; ok {
			out = append(out, db.CompanyNamesBySlugsRow{Slug: s, Name: n})
		}
	}
	return out, nil
}

// 60 companies: co-00 posts 60 jobs, co-01 59, ... co-59 one.
func sixtyCompanies() map[string]int64 {
	dist := make(map[string]int64, 60)
	for i := range 60 {
		dist[fmt.Sprintf("co-%02d", i)] = int64(60 - i)
	}
	return dist
}

func TestJobFacets_CompanySlugReportsExactTotalAndNamedTop50(t *testing.T) {
	fake := &fakeFacetCounter{distinct: 1540, res: search.FacetResult{
		Total:  9000,
		Facets: map[string]map[string]int64{"company_slug": sixtyCompanies(), "countries": {"BR": 10}},
	}}
	names := &fakeCompanyNames{names: map[string]string{"co-00": "Company Zero", "co-01": "Company One"}}
	h := &searchHandlers{facets: fake, companyNames: names}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/facets", newContract(contractHandlers{searchHandlers: h}))

	status, body := doGet(t, app, "/api/v1/jobs/facets?countries=BR&facets=countries,company_slug")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (%v)", status, body)
	}
	if !contains(fake.got.Facets, "company_slug") {
		t.Errorf("facet query = %v, want company_slug requested", fake.got.Facets)
	}
	if len(fake.gotDistinct) != 1 || fake.gotDistinct[0] != "company_slug" {
		t.Errorf("DistinctCount calls = %v, want one over company_slug", fake.gotDistinct)
	}

	data := body["data"].(map[string]any)
	if data["companies_total"].(float64) != 1540 {
		t.Errorf("companies_total = %v, want 1540", data["companies_total"])
	}
	// The trimmed list replaces the raw distribution rather than shipping both.
	if _, present := data["facets"].(map[string]any)["company_slug"]; present {
		t.Error("facets.company_slug should not carry the untrimmed distribution")
	}
	top, ok := data["top_companies"].([]any)
	if !ok || len(top) != 50 {
		t.Fatalf("top_companies = %#v, want 50 entries", data["top_companies"])
	}
	first := top[0].(map[string]any)
	if first["slug"] != "co-00" || first["name"] != "Company Zero" || first["count"].(float64) != 60 {
		t.Errorf("top_companies[0] = %v, want co-00 / Company Zero / 60", first)
	}
	for i := 1; i < len(top); i++ {
		if top[i].(map[string]any)["count"].(float64) > top[i-1].(map[string]any)["count"].(float64) {
			t.Fatalf("top_companies not in descending count at %d", i)
		}
	}
	// A slug the companies table does not name yet still carries a display name.
	if last := top[49].(map[string]any); last["slug"] != "co-49" || last["name"] != "co-49" {
		t.Errorf("top_companies[49] = %v, want co-49 named by its slug", last)
	}
	if len(names.got) != 1 || len(names.got[0]) != 50 {
		t.Errorf("name reads = %v, want ONE read of the 50 shown slugs", names.got)
	}
}

// The names only decorate the counts: a failed read must not cost the caller the
// facets and the exact total, so every company falls back to its slug.
func TestJobFacets_CompanyNameReadFailureFallsBackToSlugs(t *testing.T) {
	fake := &fakeFacetCounter{distinct: 2, res: search.FacetResult{
		Total:  3,
		Facets: map[string]map[string]int64{"company_slug": {"acme": 2, "globex": 1}},
	}}
	names := &fakeCompanyNames{err: errors.New("db down")}
	h := &searchHandlers{facets: fake, companyNames: names}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/facets", newContract(contractHandlers{searchHandlers: h}))

	status, body := doGet(t, app, "/api/v1/jobs/facets?facets=company_slug")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (%v)", status, body)
	}
	data := body["data"].(map[string]any)
	if data["companies_total"].(float64) != 2 {
		t.Errorf("companies_total = %v, want 2", data["companies_total"])
	}
	first := data["top_companies"].([]any)[0].(map[string]any)
	if first["slug"] != "acme" || first["name"] != "acme" {
		t.Errorf("top_companies[0] = %v, want acme named by its slug", first)
	}
}

func TestJobFacets_CompanySlugNotRequestedRunsNoDistinctCount(t *testing.T) {
	fake := &fakeFacetCounter{res: search.FacetResult{Total: 5}}
	app := facetsApp(fake)

	for _, target := range []string{"/api/v1/jobs/facets", "/api/v1/jobs/facets?facets=countries"} {
		status, body := doGet(t, app, target)
		if status != fiber.StatusOK {
			t.Fatalf("%s: status = %d, want 200", target, status)
		}
		data := body["data"].(map[string]any)
		if _, present := data["companies_total"]; present {
			t.Errorf("%s: companies_total present without being requested", target)
		}
		if _, present := data["top_companies"]; present {
			t.Errorf("%s: top_companies present without being requested", target)
		}
		if contains(fake.got.Facets, "company_slug") {
			t.Errorf("%s: company_slug in the default facet set", target)
		}
	}
	if len(fake.gotDistinct) != 0 {
		t.Errorf("DistinctCount called %d times, want none", len(fake.gotDistinct))
	}
}

func TestJobFacets_CompanySlugRefusedUnderDisjunctive(t *testing.T) {
	fake := &fakeFacetCounter{}
	app := facetsApp(fake)

	status, _ := doGet(t, app, "/api/v1/jobs/facets?disjunctive=1&facets=company_slug")
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if fake.gotReqs != nil || len(fake.gotDistinct) != 0 {
		t.Error("a refused request must reach neither counter")
	}
}

// The distinct count rides the one-minute facet cache with the distribution.
func TestJobFacets_CompanyCountIsCached(t *testing.T) {
	fake := &fakeFacetCounter{distinct: 7, res: search.FacetResult{Total: 9}}
	app := facetsAppCached(fake, cache.NewMemory())

	for i := 0; i < 2; i++ {
		status, body := doGet(t, app, "/api/v1/jobs/facets?facets=company_slug")
		if status != fiber.StatusOK {
			t.Fatalf("request %d: status = %d", i, status)
		}
		if got := body["data"].(map[string]any)["companies_total"]; got != float64(7) {
			t.Fatalf("request %d: companies_total = %v, want 7", i, got)
		}
	}
	if fake.calls != 1 || len(fake.gotDistinct) != 1 {
		t.Errorf("FacetCounts %d / DistinctCount %d calls, want 1 each", fake.calls, len(fake.gotDistinct))
	}
}
