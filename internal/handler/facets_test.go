package handler

import (
	"context"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/cache"
	"github.com/strelov1/freehire/internal/search"
)

type fakeFacetCounter struct {
	got      search.FacetParams
	gotReqs  []search.FacetReq
	gotTotal any
	res      search.FacetResult
	err      error
	// calls counts FacetCounts invocations, so a test can prove the cache served a
	// repeated request without recomputing.
	calls int
}

func (f *fakeFacetCounter) FacetCounts(_ context.Context, p search.FacetParams) (search.FacetResult, error) {
	f.got = p
	f.calls++
	return f.res, f.err
}

func (f *fakeFacetCounter) DisjunctiveFacetCounts(_ context.Context, _ string, reqs []search.FacetReq, totalFilter any) (search.FacetResult, error) {
	f.gotReqs = reqs
	f.gotTotal = totalFilter
	return f.res, f.err
}

// reqFilter returns the reduced filter the disjunctive path built for one index
// attribute (nil if absent).
func (f *fakeFacetCounter) reqFilter(attr string) any {
	for _, r := range f.gotReqs {
		if r.Attr == attr {
			return r.Filter
		}
	}
	return nil
}

func facetsApp(fc facetCounter) *fiber.App {
	h := &searchHandlers{facets: fc}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/jobs/facets", h.JobFacets)
	return app
}

// facetsAppCached is facetsApp with a live cache wired in, for the caching tests.
func facetsAppCached(fc facetCounter, c cache.Cache) *fiber.App {
	h := &searchHandlers{facets: fc, cache: c}
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

func TestJobFacets_DisjunctiveReducesEachFacetsOwnFilter(t *testing.T) {
	fake := &fakeFacetCounter{}
	app := facetsApp(fake)

	status, _ := doGet(t, app, "/jobs/facets?disjunctive=1&seniority=senior&regions=eu")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	// The seniority facet is counted WITHOUT its own selection, but WITH the others.
	senGroups, ok := fake.reqFilter("enrichment.seniority").([][]string)
	if !ok {
		t.Fatalf("no reduced filter for enrichment.seniority: %#v", fake.gotReqs)
	}
	if filterHas(senGroups, `enrichment.seniority = "senior"`) {
		t.Errorf("seniority facet should drop its own selection, got %#v", senGroups)
	}
	if !filterHas(senGroups, `regions = "eu"`) {
		t.Errorf("seniority facet should keep other facets (regions), got %#v", senGroups)
	}

	// A different facet keeps seniority in its own reduced filter.
	regGroups, _ := fake.reqFilter("regions").([][]string)
	if !filterHas(regGroups, `enrichment.seniority = "senior"`) {
		t.Errorf("regions facet should keep seniority, got %#v", regGroups)
	}
	if filterHas(regGroups, `regions = "eu"`) {
		t.Errorf("regions facet should drop its own selection, got %#v", regGroups)
	}

	// The grand total uses the full filter (both facets).
	totalGroups, _ := fake.gotTotal.([][]string)
	if !filterHas(totalGroups, `enrichment.seniority = "senior"`) || !filterHas(totalGroups, `regions = "eu"`) {
		t.Errorf("total filter should include all facets, got %#v", totalGroups)
	}
}

func TestJobFacets_DisjunctiveDropsWholeLocationGroup(t *testing.T) {
	// regions/countries/cities share one OR group, so a location facet's reduced
	// filter must drop the WHOLE group — else selecting a country zeroes sibling
	// regions.
	fake := &fakeFacetCounter{}
	app := facetsApp(fake)

	status, _ := doGet(t, app, "/jobs/facets?disjunctive=1&countries=BR&seniority=senior")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	// The regions facet is counted with the entire location group removed (no BR),
	// but keeps the non-location seniority selection.
	regGroups, ok := fake.reqFilter("regions").([][]string)
	if !ok {
		t.Fatalf("no reduced filter for regions: %#v", fake.gotReqs)
	}
	if filterHas(regGroups, `countries = "BR"`) {
		t.Errorf("regions facet should drop the whole location group, got %#v", regGroups)
	}
	if !filterHas(regGroups, `enrichment.seniority = "senior"`) {
		t.Errorf("regions facet should keep seniority, got %#v", regGroups)
	}

	// A non-location facet keeps the country selection.
	senGroups, _ := fake.reqFilter("enrichment.seniority").([][]string)
	if !filterHas(senGroups, `countries = "BR"`) {
		t.Errorf("seniority facet should keep the country selection, got %#v", senGroups)
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
	for _, want := range []string{"regions", "roles", "enrichment.seniority", "enrichment.visa_sponsorship", "enrichment.salary_min"} {
		if !contains(fake.got.Facets, want) {
			t.Errorf("Facets requested = %v, missing %q", fake.got.Facets, want)
		}
	}
}

func TestJobFacets_RoleFilterAndDistribution(t *testing.T) {
	// The public `role` param filters on the bare `roles` attribute, and a `roles`
	// distribution returned by the backend is re-keyed to the public `role` param.
	fake := &fakeFacetCounter{res: search.FacetResult{
		Total:  5,
		Facets: map[string]map[string]int64{"roles": {"senior_backend": 3, "founding_engineer": 2}},
	}}
	app := facetsApp(fake)

	_, body := doGet(t, app, "/jobs/facets?role=senior_backend")

	groups, ok := fake.got.Filter.([][]string)
	if !ok || !filterHas(groups, `roles = "senior_backend"`) {
		t.Errorf("Filter missing role facet: %#v", fake.got.Filter)
	}
	facets := body["data"].(map[string]any)["facets"].(map[string]any)
	role, present := facets["role"].(map[string]any)
	if !present {
		t.Fatalf("roles distribution should be re-keyed to public param role, got %v", facets)
	}
	if role["senior_backend"].(float64) != 3 {
		t.Errorf("facets.role.senior_backend = %v, want 3", role["senior_backend"])
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

// Narrowing must map public params to index attributes and count nothing else —
// that saving is the whole point of the param (see facetAttributes).
func TestJobFacets_FacetsParamNarrowsTheRequest(t *testing.T) {
	fake := &fakeFacetCounter{}
	app := facetsApp(fake)

	status, _ := doGet(t, app, "/jobs/facets?work_mode=remote&facets=seniority,regions")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	want := []string{"enrichment.seniority", "regions"}
	if len(fake.got.Facets) != len(want) {
		t.Fatalf("Facets = %v, want exactly %v", fake.got.Facets, want)
	}
	for _, w := range want {
		if !contains(fake.got.Facets, w) {
			t.Errorf("Facets = %v, missing %q", fake.got.Facets, w)
		}
	}

	// Narrowing what is COUNTED must not narrow what it is counted UNDER, or the
	// numbers answer a different question.
	groups, ok := fake.got.Filter.([][]string)
	if !ok {
		t.Fatalf("Filter = %#v, want [][]string", fake.got.Filter)
	}
	if !filterHas(groups, `work_mode = "remote"`) {
		t.Errorf("Filter lost the request's own filters: %#v", groups)
	}
}

// A typo must not read as "that value has no count": callers cannot tell a
// missing key from one Meili's per-facet cap dropped, so silence would hide it.
func TestJobFacets_UnknownFacetIsRejected(t *testing.T) {
	for _, name := range []string{"nonsense", "company_slug"} {
		fake := &fakeFacetCounter{}
		app := facetsApp(fake)
		status, _ := doGet(t, app, "/jobs/facets?facets="+name)
		if status != fiber.StatusBadRequest {
			t.Errorf("facets=%s: status = %d, want 400", name, status)
		}
		if fake.got.Facets != nil {
			t.Errorf("facets=%s: reached the counter with %v, want no call", name, fake.got.Facets)
		}
	}
}

// Disjunctive derives its queries from the selection, so narrowing the output
// saves none of them. Refuse rather than silently ignore.
func TestJobFacets_FacetsParamRejectedWithDisjunctive(t *testing.T) {
	fake := &fakeFacetCounter{}
	app := facetsApp(fake)

	status, _ := doGet(t, app, "/jobs/facets?disjunctive=1&facets=seniority")
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if fake.gotReqs != nil {
		t.Errorf("reached the counter with %d reqs, want no call", len(fake.gotReqs))
	}
}

func TestJobFacets_ReportsIgnoredParams(t *testing.T) {
	// The counts endpoint is where a filter becomes a number someone quotes as
	// the size of a market, so a silently dropped filter is worse here than on
	// the listing: it does not look like a long list, it looks like a fact.
	app := facetsApp(&fakeFacetCounter{})

	_, body := doGet(t, app, "/jobs/facets?country=it")

	meta, _ := body["meta"].(map[string]any)
	ignored, _ := meta["ignored_params"].([]any)
	if len(ignored) != 1 {
		t.Fatalf("meta.ignored_params = %v, want one entry", meta["ignored_params"])
	}
	first, _ := ignored[0].(map[string]any)
	if first["param"] != "country" || first["did_you_mean"] != "countries" {
		t.Errorf("ignored_params[0] = %v, want country -> countries", first)
	}
}

func TestJobFacets_CleanQueryKeepsTheBareDataEnvelope(t *testing.T) {
	// This endpoint answers {"data": ...} with no meta. A clean request must
	// keep that shape rather than grow an empty meta block.
	app := facetsApp(&fakeFacetCounter{})

	_, body := doGet(t, app, "/jobs/facets?q=go&facets=skills&countries=it")

	if _, present := body["meta"]; present {
		t.Errorf("meta = %v, want the key absent on a clean query", body["meta"])
	}
}

func TestJobFacets_CacheServesRepeatedRequestWithoutRecomputing(t *testing.T) {
	fake := &fakeFacetCounter{res: search.FacetResult{Total: 42}}
	app := facetsAppCached(fake, cache.NewMemory())

	// Two identical requests. The first computes and populates the cache; the
	// second must be served from it, leaving FacetCounts called exactly once.
	for i := 0; i < 2; i++ {
		status, _ := doGet(t, app, "/jobs/facets?q=go&seniority=senior")
		if status != fiber.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i, status)
		}
	}
	if fake.calls != 1 {
		t.Errorf("FacetCounts calls = %d, want 1 (second request should hit cache)", fake.calls)
	}
}

func TestJobFacets_CacheKeyStableAcrossMultiFacetRequests(t *testing.T) {
	// The filter is built by ranging over a map, so its group order is randomized
	// per request. The cache key must canonicalize it: the SAME multi-facet request
	// fired repeatedly must hit one cache entry, not miss on a reordered filter.
	// Without canonicalization this fails flakily (map order occasionally repeats).
	fake := &fakeFacetCounter{res: search.FacetResult{Total: 42}}
	app := facetsAppCached(fake, cache.NewMemory())

	const q = "/jobs/facets?seniority=senior&work_mode=remote&category=backend&regions=eu"
	for i := 0; i < 8; i++ {
		status, _ := doGet(t, app, q)
		if status != fiber.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i, status)
		}
	}
	if fake.calls != 1 {
		t.Errorf("FacetCounts calls = %d, want 1 (multi-facet key must be order-stable)", fake.calls)
	}
}

func TestJobFacets_CacheKeyVariesByFilter(t *testing.T) {
	// Different filters are different results and must not collide on one key.
	fake := &fakeFacetCounter{res: search.FacetResult{Total: 42}}
	app := facetsAppCached(fake, cache.NewMemory())

	doGet(t, app, "/jobs/facets?seniority=senior")
	doGet(t, app, "/jobs/facets?seniority=junior")

	if fake.calls != 2 {
		t.Errorf("FacetCounts calls = %d, want 2 (distinct filters must not share a cache key)", fake.calls)
	}
}

func TestJobFacets_DisjunctiveIsNotCached(t *testing.T) {
	// The disjunctive path is the interactive live-modal experience and is left
	// uncached, so two identical disjunctive requests both recompute.
	fake := &fakeFacetCounter{res: search.FacetResult{Total: 42}}
	app := facetsAppCached(fake, cache.NewMemory())

	doGet(t, app, "/jobs/facets?disjunctive=1&seniority=senior")
	doGet(t, app, "/jobs/facets?disjunctive=1&seniority=senior")

	if fake.calls != 0 {
		t.Fatalf("FacetCounts calls = %d; disjunctive should not use the FacetCounts path at all", fake.calls)
	}
}

func TestJobFacets_NilCacheStillComputes(t *testing.T) {
	// A handler with no cache configured must fall straight through to compute,
	// every time — this is the shape every other facet test runs under.
	fake := &fakeFacetCounter{res: search.FacetResult{Total: 42}}
	app := facetsApp(fake) // nil cache

	doGet(t, app, "/jobs/facets?seniority=senior")
	doGet(t, app, "/jobs/facets?seniority=senior")

	if fake.calls != 2 {
		t.Errorf("FacetCounts calls = %d, want 2 (no cache = always compute)", fake.calls)
	}
}
