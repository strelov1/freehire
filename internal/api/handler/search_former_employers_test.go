package handler

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/search/search"
)

type fakeFormerEmployers struct {
	slugs  []string
	err    error
	called bool
	userID int64
}

func (f *fakeFormerEmployers) Slugs(_ context.Context, userID int64) ([]string, error) {
	f.called, f.userID = true, userID
	return f.slugs, f.err
}

// formerEmployersApp mounts /jobs/search with the lookup. userID 0 is an anonymous caller.
func formerEmployersApp(s searcher, userID int64, lookup formerEmployerLookup) *fiber.App {
	h := &searchHandlers{search: s, formerEmployers: lookup}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/jobs/search", func(c *fiber.Ctx) error {
		if userID != 0 {
			c.Locals(auth.LocalsUserID, userID)
		}
		return h.SearchJobs(c)
	})
	return app
}

func TestFormerEmployers_SignedInNarrowsToTheResolvedCompanies(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 12}}
	lookup := &fakeFormerEmployers{slugs: []string{"accenture", "globant"}}
	app := formerEmployersApp(fake, 7, lookup)

	status, body := doGet(t, app, "/jobs/search?former_employers=true&seniority=senior")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if lookup.userID != 7 {
		t.Errorf("looked up user %d, want the caller (7)", lookup.userID)
	}
	want := [][]string{{`enrichment.seniority = "senior"`}, {`company_slug IN ["accenture", "globant"]`}}
	if !reflect.DeepEqual(fake.got.Filter, want) {
		t.Errorf("filter = %#v, want %#v", fake.got.Filter, want)
	}
	if names := ignoredParamNames(body); len(names) != 0 {
		t.Errorf("ignored_params = %v, want none — the filter was read", names)
	}
	if total := body["meta"].(map[string]any)["total"]; total != float64(12) {
		t.Errorf("total = %v, want the filtered count (12)", total)
	}
}

func TestFormerEmployers_NothingResolvedAnswersEmptyNotTheCatalogue(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 999}}
	app := formerEmployersApp(fake, 7, &fakeFormerEmployers{})

	status, body := doGet(t, app, "/jobs/search?former_employers=true")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if fake.got.Limit != 0 {
		t.Error("the index was queried; with no former employer it must not be, or the answer is strangers")
	}
	if data, _ := body["data"].([]any); len(data) != 0 {
		t.Errorf("data = %v, want empty", data)
	}
	if total := body["meta"].(map[string]any)["total"]; total != float64(0) {
		t.Errorf("total = %v, want 0", total)
	}
}

func TestFormerEmployers_AnonymousIsReportedAsIgnored(t *testing.T) {
	fake := &fakeSearcher{}
	lookup := &fakeFormerEmployers{slugs: []string{"accenture"}}
	app := formerEmployersApp(fake, 0, lookup)

	status, body := doGet(t, app, "/jobs/search?former_employers=true")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — a shared link must not break for a signed-out visitor", status)
	}
	if lookup.called {
		t.Error("an anonymous request looked up an experience bank")
	}
	if fake.got.Filter != nil {
		t.Errorf("filter = %#v, want none", fake.got.Filter)
	}
	if names := ignoredParamNames(body); !reflect.DeepEqual(names, []string{formerEmployersParam}) {
		t.Errorf("ignored_params = %v, want [%s] — the catalogue must not pass for former employers", names, formerEmployersParam)
	}
}

func TestFormerEmployers_FalseIsNotAFilter(t *testing.T) {
	fake := &fakeSearcher{}
	lookup := &fakeFormerEmployers{slugs: []string{"accenture"}}
	app := formerEmployersApp(fake, 7, lookup)

	if status, _ := doGet(t, app, "/jobs/search?former_employers=false"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if lookup.called || fake.got.Filter != nil {
		t.Errorf("former_employers=false narrowed the search (filter %#v)", fake.got.Filter)
	}
}

func TestFormerEmployers_LookupFailureFailsTheRequest(t *testing.T) {
	// Searching without the filter on a failed lookup would answer with strangers.
	fake := &fakeSearcher{}
	app := formerEmployersApp(fake, 7, &fakeFormerEmployers{err: errors.New("db down")})

	if status, _ := doGet(t, app, "/jobs/search?former_employers=true"); status != fiber.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
	if fake.got.Limit != 0 {
		t.Error("the index was queried after the lookup failed")
	}
}

// degradingSearcher refuses the first query as a bad filter, the way Meilisearch answers a
// filterable attribute the live index does not declare yet, then records the retry.
type degradingSearcher struct {
	calls []search.SearchParams
}

func (s *degradingSearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	s.calls = append(s.calls, p)
	if len(s.calls) == 1 {
		return search.SearchResult{}, search.ErrBadQuery
	}
	return search.SearchResult{}, nil
}

func TestFormerEmployers_SurvivesTheBadFilterDegrade(t *testing.T) {
	fake := &degradingSearcher{}
	app := formerEmployersApp(fake, 7, &fakeFormerEmployers{slugs: []string{"accenture"}})

	if status, _ := doGet(t, app, "/jobs/search?former_employers=true&seniority=senior"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("searched %d times, want the query and one retry", len(fake.calls))
	}
	want := [][]string{{`company_slug IN ["accenture"]`}}
	if !reflect.DeepEqual(fake.calls[1].Filter, want) {
		t.Errorf("retry filter = %#v, want only the former-employer group %#v", fake.calls[1].Filter, want)
	}
}

func TestFormerEmployers_SeenExclusionSurvivesTheBadFilterDegrade(t *testing.T) {
	fake := &degradingSearcher{}
	h := &searchHandlers{
		search:          fake,
		formerEmployers: &fakeFormerEmployers{slugs: []string{"accenture"}},
		seen:            &fakeSeen{ids: []int64{11}},
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/jobs/search", func(c *fiber.Ctx) error {
		c.Locals(auth.LocalsUserID, int64(7))
		return h.SearchJobs(c)
	})
	status, body := doGet(t, app, "/jobs/search?former_employers=true&hide_seen=true&seniority=senior")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("searched %d times, want the query and one retry", len(fake.calls))
	}
	want := [][]string{{`company_slug IN ["accenture"]`}, {"id NOT IN [11]"}}
	if !reflect.DeepEqual(fake.calls[1].Filter, want) {
		t.Errorf("retry filter = %#v, want %#v", fake.calls[1].Filter, want)
	}
	if reportsParam(body, formerEmployersParam) || reportsParam(body, hideSeenParam) {
		t.Errorf("honoured personal filters reported as ignored: %v", ignoredParamNames(body))
	}
}
