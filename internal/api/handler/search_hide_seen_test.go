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

type fakeSeen struct {
	ids    []int64
	err    error
	called bool
}

func (f *fakeSeen) ExcludedJobIDs(context.Context, int64) ([]int64, error) {
	f.called = true
	return f.ids, f.err
}

// hideSeenApp mounts /jobs/search with a seen-jobs reader. userID 0 is an anonymous
// caller — the route is public, so no session is attached.
func hideSeenApp(s searcher, userID int64, seen seenJobs) *fiber.App {
	h := &searchHandlers{search: s, seen: seen}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/jobs/search", func(c *fiber.Ctx) error {
		if userID != 0 {
			c.Locals(auth.LocalsUserID, userID)
		}
		return h.SearchJobs(c)
	})
	return app
}

func ignoredParamNames(body map[string]any) []string {
	meta, _ := body["meta"].(map[string]any)
	list, _ := meta["ignored_params"].([]any)
	var out []string
	for _, e := range list {
		m, _ := e.(map[string]any)
		name, _ := m["param"].(string)
		out = append(out, name)
	}
	return out
}

func reportsParam(body map[string]any, param string) bool {
	for _, name := range ignoredParamNames(body) {
		if name == param {
			return true
		}
	}
	return false
}

func TestSearchJobsHideSeen_SignedInExcludesSeenInsideTheQuery(t *testing.T) {
	fake := &fakeSearcher{}
	app := hideSeenApp(fake, 7, &fakeSeen{ids: []int64{11, 12}})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/jobs/search?hide_seen=true&seniority=senior", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	var body map[string]any
	if status := decodeJSON(t, resp, &body); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	groups, ok := fake.got.Filter.([][]string)
	if !ok {
		t.Fatalf("Filter = %#v, want [][]string", fake.got.Filter)
	}
	// In the index query, not over the page: that is what keeps total and paging honest.
	if !filterHas(groups, "id NOT IN [11, 12]") || !filterHas(groups, `enrichment.seniority = "senior"`) {
		t.Errorf("Filter = %#v, want the facet and the seen exclusion", groups)
	}
	if reportsParam(body, hideSeenParam) {
		t.Errorf("honoured hide_seen reported as ignored: %v", ignoredParamNames(body))
	}
	if got := resp.Header.Get(fiber.HeaderCacheControl); got != "private, no-store" {
		t.Errorf("Cache-Control = %q, want private — the body depends on who asked", got)
	}
}

func TestSearchJobsHideSeen_AnonymousCallerIsToldItWasNotApplied(t *testing.T) {
	fake := &fakeSearcher{}
	seen := &fakeSeen{ids: []int64{11}}
	app := hideSeenApp(fake, 0, seen)

	status, body := doGet(t, app, "/jobs/search?hide_seen=true")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — a shared link carrying hide_seen must not break", status)
	}
	if seen.called {
		t.Error("read a seen set for an anonymous caller")
	}
	if fake.got.Filter != nil {
		t.Errorf("Filter = %#v, want none", fake.got.Filter)
	}
	if !reportsParam(body, hideSeenParam) {
		t.Errorf("ignored_params = %v, want hide_seen named", ignoredParamNames(body))
	}
}

func TestSearchJobsHideSeen_UnreadableRecordWidensAndSaysSo(t *testing.T) {
	fake := &fakeSearcher{}
	app := hideSeenApp(fake, 7, &fakeSeen{err: errors.New("db down")})

	status, body := doGet(t, app, "/jobs/search?hide_seen=true")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !reportsParam(body, hideSeenParam) {
		t.Errorf("ignored_params = %v, want hide_seen named", ignoredParamNames(body))
	}
}

func TestSearchJobsHideSeen_ValueRules(t *testing.T) {
	for _, tc := range []struct {
		query       string
		wantRead    bool
		wantIgnored bool
	}{
		{"hide_seen=false", false, false},
		{"hide_seen=1", true, false},
		{"hide_seen=yes", false, true},
	} {
		t.Run(tc.query, func(t *testing.T) {
			seen := &fakeSeen{ids: []int64{11}}
			app := hideSeenApp(&fakeSearcher{}, 7, seen)
			_, body := doGet(t, app, "/jobs/search?"+tc.query)
			if seen.called != tc.wantRead {
				t.Errorf("read seen set = %v, want %v", seen.called, tc.wantRead)
			}
			if reportsParam(body, hideSeenParam) != tc.wantIgnored {
				t.Errorf("ignored_params = %v, want hide_seen reported = %v", ignoredParamNames(body), tc.wantIgnored)
			}
		})
	}
}

// The exclusion is not what Meilisearch refuses during a filterable-attribute deploy
// window, so the facet-dropping retry keeps it: a daily visitor's feed must not fill
// back up with what they saw because an unrelated facet was briefly undeclared.
func TestSearchJobsHideSeen_SurvivesTheFilterDegradeRetry(t *testing.T) {
	fake := &retrySearcher{first: search.ErrBadQuery}
	app := hideSeenApp(fake, 7, &fakeSeen{ids: []int64{11}})

	if status, _ := doGet(t, app, "/jobs/search?hide_seen=true&seniority=senior"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	groups, _ := fake.last.Filter.([][]string)
	if len(groups) != 1 || !filterHas(groups, "id NOT IN [11]") {
		t.Errorf("retry Filter = %#v, want only the seen exclusion", fake.last.Filter)
	}
}

// The flag belongs to the feed, not to a saved search: nothing that builds a search
// from stored params reads it, so it narrows nothing on its own.
func TestHideSeenIsNotSearchVocabulary(t *testing.T) {
	vals := map[string][]string{hideSeenParam: {"true"}}
	if search.Narrows(vals) {
		t.Error("Narrows(hide_seen=true) = true — a subscription to it would be an alert on the whole catalogue")
	}
	if search.FilterFromValues(vals) != nil {
		t.Error("FilterFromValues read hide_seen — a digest has no 'seen' to apply")
	}
}

type retrySearcher struct {
	first error
	calls int
	last  search.SearchParams
}

func (r *retrySearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	r.calls++
	r.last = p
	if r.calls == 1 {
		return search.SearchResult{}, r.first
	}
	return search.SearchResult{}, nil
}
