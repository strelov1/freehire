package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/search/search"
)

type fakeAppliedCompanies struct {
	slugs  []string
	err    error
	called bool
}

func (f *fakeAppliedCompanies) AppliedCompanySlugs(context.Context, int64) ([]string, error) {
	f.called = true
	return f.slugs, f.err
}

// hideAppliedApp mounts /jobs/search with an applied-company reader. userID 0 is an
// anonymous caller — the route is public, so no session is attached.
func hideAppliedApp(s searcher, userID int64, applied appliedCompanyReader) *fiber.App {
	h := &searchHandlers{search: s, appliedCompanies: applied}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/jobs/search", func(c *fiber.Ctx) error {
		if userID != 0 {
			c.Locals(auth.LocalsUserID, userID)
		}
		return h.SearchJobs(c)
	})
	return app
}

// ignoresParam reports whether the response's meta.ignored_params names param.
func ignoresParam(body map[string]any, param string) bool {
	meta, _ := body["meta"].(map[string]any)
	list, _ := meta["ignored_params"].([]any)
	for _, e := range list {
		if m, _ := e.(map[string]any); m["param"] == param {
			return true
		}
	}
	return false
}

func TestSearchJobsHideAppliedCompanies_ExcludesInsideTheQuery(t *testing.T) {
	fake := &fakeSearcher{}
	app := hideAppliedApp(fake, 7, &fakeAppliedCompanies{slugs: []string{"acme", "globex"}})

	status, body := doGet(t, app, "/jobs/search?hide_applied_companies=true&seniority=senior")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	groups, ok := fake.got.Filter.([][]string)
	if !ok {
		t.Fatalf("Filter = %#v, want [][]string", fake.got.Filter)
	}
	// In the index query, not over the page: that is what keeps total and paging honest.
	if !filterHas(groups, `company_slug NOT IN ["acme", "globex"]`) || !filterHas(groups, `enrichment.seniority = "senior"`) {
		t.Errorf("Filter = %#v, want the facet and the company exclusion", groups)
	}
	if ignoresParam(body, hideAppliedCompaniesParam) {
		t.Error("honoured hide_applied_companies reported as ignored")
	}
}

func TestSearchJobsHideAppliedCompanies_ResponseIsPrivate(t *testing.T) {
	app := hideAppliedApp(&fakeSearcher{}, 7, &fakeAppliedCompanies{slugs: []string{"acme"}})
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/jobs/search?hide_applied_companies=true", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get(fiber.HeaderCacheControl); got != "private, no-store" {
		t.Errorf("Cache-Control = %q, want private — the body depends on who asked", got)
	}
}

func TestSearchJobsHideAppliedCompanies_NoApplicationsExcludesNothing(t *testing.T) {
	fake := &fakeSearcher{}
	app := hideAppliedApp(fake, 7, &fakeAppliedCompanies{})

	_, body := doGet(t, app, "/jobs/search?hide_applied_companies=true")
	if fake.got.Filter != nil {
		t.Errorf("Filter = %#v, want none", fake.got.Filter)
	}
	// Honoured, just with nothing to hide — not a param the endpoint failed to read.
	if ignoresParam(body, hideAppliedCompaniesParam) {
		t.Error("an empty applied set reported hide_applied_companies as ignored")
	}
}

func TestSearchJobsHideAppliedCompanies_AnonymousCallerIsToldItWasNotApplied(t *testing.T) {
	fake := &fakeSearcher{}
	applied := &fakeAppliedCompanies{slugs: []string{"acme"}}
	app := hideAppliedApp(fake, 0, applied)

	status, body := doGet(t, app, "/jobs/search?hide_applied_companies=true")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — a shared link carrying the param must not break", status)
	}
	if applied.called {
		t.Error("read an applied set for an anonymous caller")
	}
	if fake.got.Filter != nil {
		t.Errorf("Filter = %#v, want none", fake.got.Filter)
	}
	if !ignoresParam(body, hideAppliedCompaniesParam) {
		t.Error("anonymous hide_applied_companies not reported in ignored_params")
	}
}

func TestSearchJobsHideAppliedCompanies_UnreadableRecordWidensAndSaysSo(t *testing.T) {
	app := hideAppliedApp(&fakeSearcher{}, 7, &fakeAppliedCompanies{err: errors.New("db down")})

	status, body := doGet(t, app, "/jobs/search?hide_applied_companies=true")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !ignoresParam(body, hideAppliedCompaniesParam) {
		t.Error("unreadable applied set not reported in ignored_params")
	}
}

func TestSearchJobsHideAppliedCompanies_ValueRules(t *testing.T) {
	for _, tc := range []struct {
		query       string
		wantRead    bool
		wantIgnored bool
	}{
		{"hide_applied_companies=false", false, false},
		{"hide_applied_companies=1", true, false},
		{"hide_applied_companies=yes", false, true},
	} {
		t.Run(tc.query, func(t *testing.T) {
			applied := &fakeAppliedCompanies{slugs: []string{"acme"}}
			app := hideAppliedApp(&fakeSearcher{}, 7, applied)
			_, body := doGet(t, app, "/jobs/search?"+tc.query)
			if applied.called != tc.wantRead {
				t.Errorf("read applied set = %v, want %v", applied.called, tc.wantRead)
			}
			if ignoresParam(body, hideAppliedCompaniesParam) != tc.wantIgnored {
				t.Errorf("hide_applied_companies reported = %v, want %v", !tc.wantIgnored, tc.wantIgnored)
			}
		})
	}
}

// failOnceSearcher refuses the first query with err and answers every later one, so a
// test can watch what the filter-degrade retry sends.
type failOnceSearcher struct {
	err   error
	calls int
	last  search.SearchParams
}

func (f *failOnceSearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	f.calls++
	f.last = p
	if f.calls == 1 {
		return search.SearchResult{}, f.err
	}
	return search.SearchResult{}, nil
}

// The exclusion filters on company_slug, which the index has always declared, so it is
// never what Meilisearch refused during a filterable-attribute deploy window. The
// facet-dropping retry keeps it: the employers the caller hid must not come back
// because an unrelated facet was briefly undeclared.
func TestSearchJobsHideAppliedCompanies_SurvivesTheFilterDegradeRetry(t *testing.T) {
	fake := &failOnceSearcher{err: search.ErrBadQuery}
	app := hideAppliedApp(fake, 7, &fakeAppliedCompanies{slugs: []string{"acme"}})

	if status, _ := doGet(t, app, "/jobs/search?hide_applied_companies=true&seniority=senior"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	groups, _ := fake.last.Filter.([][]string)
	if len(groups) != 1 || !filterHas(groups, `company_slug NOT IN ["acme"]`) {
		t.Errorf("retry Filter = %#v, want only the company exclusion", fake.last.Filter)
	}
}

// The flag belongs to the feed, not to a saved search: nothing that builds a search
// from stored params reads it, so it narrows nothing on its own.
func TestHideAppliedCompaniesIsNotSearchVocabulary(t *testing.T) {
	vals := map[string][]string{hideAppliedCompaniesParam: {"true"}}
	if search.Narrows(vals) {
		t.Error("Narrows(hide_applied_companies=true) = true — a subscription to it would be an alert on the whole catalogue")
	}
	if search.FilterFromValues(vals) != nil {
		t.Error("FilterFromValues read hide_applied_companies — a digest has no caller to apply it for")
	}
}
