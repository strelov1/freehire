package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/search/feed"
	"github.com/strelov1/freehire/internal/search/search"
)

func (f *feedStateFake) FeedSource(context.Context, int64) (int64, bool, error) {
	return f.source, f.source != 0, nil
}

func (f *feedStateFake) SetFeedSource(_ context.Context, _ int64, id *int64) error {
	f.source = 0
	if id != nil {
		f.source = *id
	}
	return nil
}

func (f *feedStateFake) FeedSearch(_ context.Context, _, id int64) (feedSearch, bool, error) {
	sr, ok := f.searches[id]
	return sr, ok, nil
}

func putFeedSource(t *testing.T, app *fiber.App, iss *auth.Issuer, userID int64, body string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPut, "/api/v1/me/feed/source", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	token, _ := iss.Issue(userID, testTokenVersion)
	req.Header.Set("Cookie", auth.CookieName+"="+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func sourceApp(fake *fakeSearcher, st *feedStateFake) (*fiber.App, *auth.Issuer) {
	return feedStateApp(fake, &feedExcludedAt{}, feedUpdated, st)
}

func feedMetaOf(body map[string]any) map[string]any { return body["meta"].(map[string]any) }

func oneJob() *fakeSearcher {
	return &fakeSearcher{res: search.SearchResult{Total: 1, Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "a"}}}}}
}

func TestFeedSource_DefaultIsProfile(t *testing.T) {
	app, iss := sourceApp(oneJob(), &feedStateFake{})
	_, body := deckGet(t, app, iss, "/api/v1/me/feed")
	if m := feedMetaOf(body); m["source"] != "profile" || m["saved_search_id"] != nil {
		t.Errorf("meta = %v", m)
	}
}

func TestFeedSource_PickSearchThenBackToProfile(t *testing.T) {
	fake := oneJob()
	st := &feedStateFake{searches: map[int64]feedSearch{2: {Query: "seniority=senior&q=golang&q_fields=title", UpdatedAt: time.Now()}}}
	app, iss := sourceApp(fake, st)
	if got := putFeedSource(t, app, iss, 7, `{"source":"saved_search","saved_search_id":2}`); got != fiber.StatusNoContent {
		t.Fatalf("PUT = %d", got)
	}
	_, body := deckGet(t, app, iss, "/api/v1/me/feed")
	m := feedMetaOf(body)
	if m["source"] != "saved_search" || m["saved_search_id"] != float64(2) {
		t.Errorf("meta = %v", m)
	}
	groups, _ := fake.got.Filter.([][]string)
	if !filterHas(groups, `enrichment.seniority = "senior"`) {
		t.Errorf("search filter missing: %#v", fake.got.Filter)
	}
	// Text keeps the saved search eligible by keyword: no pure-semantic vector request.
	if fake.got.Query != "golang" || len(fake.got.QFields) != 1 || fake.got.Vector != nil {
		t.Errorf("q must be retrieved by keyword with its fields: %#v", fake.got)
	}
	if st.sources[len(st.sources)-1] != "search:2" {
		t.Errorf("watermark source = %v", st.sources)
	}
	if got := putFeedSource(t, app, iss, 7, `{"source":"profile"}`); got != fiber.StatusNoContent {
		t.Fatalf("PUT profile = %d", got)
	}
	_, body = deckGet(t, app, iss, "/api/v1/me/feed")
	if feedMetaOf(body)["source"] != "profile" {
		t.Errorf("meta = %v", feedMetaOf(body))
	}
}

func TestFeedSource_SearchWithoutTextUsesTheMatchVector(t *testing.T) {
	fake := oneJob()
	st := &feedStateFake{source: 2, searches: map[int64]feedSearch{2: {Query: "seniority=senior", UpdatedAt: time.Now()}}}
	app, iss := sourceApp(fake, st)
	deckGet(t, app, iss, "/api/v1/me/feed")
	if len(fake.got.Vector) == 0 {
		t.Errorf("profile skills should still rank a text-free search")
	}
}

func TestFeedSource_DeletedSearchNeverFallsBack(t *testing.T) {
	fake := oneJob()
	app, iss := sourceApp(fake, &feedStateFake{source: 2})
	_, body := deckGet(t, app, iss, "/api/v1/me/feed")
	m := feedMetaOf(body)
	if m["reason"] != "search_deleted" || len(body["data"].([]any)) != 0 || m["saved_search_id"] != float64(2) {
		t.Errorf("meta = %v", m)
	}
	if fake.got.Limit != 0 {
		t.Errorf("searched anyway: %#v", fake.got)
	}
}

func TestFeedSource_UnenforceableSearchIsRefusedNotWidened(t *testing.T) {
	for name, tc := range map[string]struct {
		query, ignored string
	}{
		"narrows nothing": {},
		"watchlist":       {query: "watchlist=true&seniority=senior"},
		"unknown param":   {"seniority=senior&sort=match&min_skill_match=70", "min_skill_match"},
		"only unknown":    {"min_skill_match=70", "min_skill_match"},
		"bad q_fields":    {"q=go&q_fields=nonsense", "q_fields"},
		"bad visa":        {"visa_sponsorship=maybe&seniority=senior", "visa_sponsorship"},
		"bad salary":      {"q=go&salary_min=oops", "salary_min"},
		"bad experience":  {"q=go&experience_years_max=-1", "experience_years_max"},
		"bad age":         {"q=go&posted_within_days=0", "posted_within_days"},
	} {
		t.Run(name, func(t *testing.T) {
			fake := oneJob()
			st := &feedStateFake{source: 2, searches: map[int64]feedSearch{2: {Query: tc.query, UpdatedAt: time.Now()}}}
			app, iss := sourceApp(fake, st)
			_, body := deckGet(t, app, iss, "/api/v1/me/feed")
			m := feedMetaOf(body)
			if len(body["data"].([]any)) != 0 || fake.got.Limit != 0 {
				t.Errorf("want an empty page with a reason and no search: meta=%v got=%#v", m, fake.got)
			}
			if tc.ignored == "" {
				if m["reason"] != feed.ReasonNoUsableFields || m["ignored"] != nil {
					t.Errorf("meta = %v, want no usable fields", m)
				}
				return
			}
			ignored, _ := m["ignored"].([]any)
			if m["reason"] != feedReasonIgnoredFilters || len(ignored) != 1 || ignored[0].(map[string]any)["param"] != tc.ignored {
				t.Errorf("meta = %v, want ignored %s", m, tc.ignored)
			}
		})
	}
}

func TestFeedSource_ForeignOrBadPutIsRejectedAndKeepsTheSource(t *testing.T) {
	st := &feedStateFake{source: 2, searches: map[int64]feedSearch{2: {Query: "q=go", UpdatedAt: time.Now()}}}
	app, iss := sourceApp(oneJob(), st)
	for body, want := range map[string]int{
		`{"source":"saved_search","saved_search_id":99}`: fiber.StatusNotFound,
		`{"source":"saved_search"}`:                      fiber.StatusBadRequest,
		`{"source":"nope"}`:                              fiber.StatusBadRequest,
		`not json`:                                       fiber.StatusBadRequest,
	} {
		if got := putFeedSource(t, app, iss, 7, body); got != want {
			t.Errorf("PUT %s = %d, want %d", body, got, want)
		}
	}
	if st.source != 2 {
		t.Errorf("stored source changed to %d", st.source)
	}
}

func TestFeedSource_CursorFromAnotherSourceIs400(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 5, Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "a"}}}}}
	st := &feedStateFake{searches: map[int64]feedSearch{2: {Query: "q=go", UpdatedAt: time.Now()}}}
	app, iss := sourceApp(fake, st)
	_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=1")
	cursor := nextCursorOf(body)
	if cursor == "" {
		t.Fatal("no cursor from the profile page")
	}
	st.source = 2
	if status, _ := deckGet(t, app, iss, "/api/v1/me/feed?limit=1&cursor="+cursor); status != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}

func TestFeedSource_SearchWithoutVectorSortsByDateKeepingText(t *testing.T) {
	fake := oneJob()
	st := &feedStateFake{source: 2, searches: map[int64]feedSearch{2: {Query: "q=golang&q_fields=title", UpdatedAt: time.Now()}}}
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &feedHandlers{search: fake, profiles: feedProfile{p: userprofile.Profile{}}, excluded: &feedExcludedAt{}, plans: feedPlansFake{}, state: st, cursors: feed.NewSigner([]byte("cursor-key"))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/feed", auth.RequireAuth(iss, testVersions), h.Feed)
	_, body := deckGet(t, app, iss, "/api/v1/me/feed")
	m := feedMetaOf(body)
	if m["degraded"] != "no_match_vector" || m["ranking"] != "date" {
		t.Errorf("meta = %v", m)
	}
	if len(fake.got.Sort) != 1 || fake.got.Sort[0] != "created_at:desc" || fake.got.Query != "golang" || fake.got.Vector != nil {
		t.Errorf("want date sort with the keyword query: %#v", fake.got)
	}
}

func TestFeedSource_ZeroMatchesIsNotAReason(t *testing.T) {
	st := &feedStateFake{source: 2, searches: map[int64]feedSearch{2: {Query: "seniority=senior", UpdatedAt: time.Now()}}}
	app, iss := sourceApp(&fakeSearcher{}, st)
	_, body := deckGet(t, app, iss, "/api/v1/me/feed")
	if m := feedMetaOf(body); m["reason"] != nil || len(body["data"].([]any)) != 0 || m["saved_search_id"] != float64(2) {
		t.Errorf("meta = %v", m)
	}
}

func TestFeedSource_ProfileEditRestartsSavedSearchPagination(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 5, Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "a"}}}}}
	st := &feedStateFake{source: 2, searches: map[int64]feedSearch{2: {Query: "seniority=senior", UpdatedAt: feedUpdated.Add(time.Hour)}}}
	app, iss := sourceApp(fake, st)
	_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=1")
	cursor := nextCursorOf(body)
	if cursor == "" {
		t.Fatal("no cursor from the saved search")
	}

	edited, editedIssuer := feedStateApp(fake, &feedExcludedAt{}, feedUpdated.Add(time.Second), st)
	status, body := deckGet(t, edited, editedIssuer, "/api/v1/me/feed?limit=1&cursor="+cursor)
	if status != fiber.StatusOK || feedMetaOf(body)["reason"] != "source_changed" {
		t.Fatalf("status = %d, body = %v; want source_changed", status, body)
	}
}

// hiddenFeedApp serves a feed whose profile hides every slug in hidden.
func hiddenFeedApp(s searcher, hidden []string, updated time.Time, st *feedStateFake) (*fiber.App, *auth.Issuer) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	prof := feedProfile{p: userprofile.Profile{Skills: []string{"go"}, ExcludedCompanies: hidden, UpdatedAt: &updated}}
	h := &feedHandlers{search: s, profiles: prof, excluded: &feedExcludedAt{}, plans: feedPlansFake{}, state: st, cursors: feed.NewSigner([]byte("cursor-key"))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/feed", auth.RequireAuth(iss, testVersions), h.Feed)
	return app, iss
}

// longSlugs is well past the profile query's 400-character company budget.
func longSlugs() []string {
	out := make([]string, 30)
	for i := range out {
		out[i] = strings.Repeat("c", 20) + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	return out
}

func TestFeed_HiddenCompaniesBindEverySourceAndPhase(t *testing.T) {
	hidden := longSlugs()
	want := search.NotInStrings("company_slug", hidden)
	seen := time.Now().Add(-time.Hour).Truncate(time.Second)
	for name, st := range map[string]*feedStateFake{
		"profile": {seen: &seen},
		"search":  {seen: &seen, source: 2, searches: map[int64]feedSearch{2: {Query: "seniority=senior", UpdatedAt: feedUpdated}}},
	} {
		fake := &scriptedSearcher{newHits: docs("n1"), restHits: docs("r1", "r2")}
		app, iss := hiddenFeedApp(fake, hidden, feedUpdated, st)
		deckGet(t, app, iss, "/api/v1/me/feed?limit=5")
		if len(fake.calls) != 2 {
			t.Fatalf("%s: calls = %d, want the new and the rest phase", name, len(fake.calls))
		}
		for i, call := range fake.calls {
			if !filterHas(call.Filter.([][]string), want) {
				t.Errorf("%s call %d lacks the full hidden-company exclusion: %#v", name, i, call.Filter)
			}
		}
	}
}

func TestFeed_HiddenCompanyChangeInvalidatesCursorAndRestoreDropsTheFilter(t *testing.T) {
	for name, st := range map[string]func() *feedStateFake{
		"profile": func() *feedStateFake { return &feedStateFake{} },
		// The saved search stays unchanged: only the profile version moves.
		"search": func() *feedStateFake {
			return &feedStateFake{source: 2, searches: map[int64]feedSearch{2: {Query: "seniority=senior", UpdatedAt: feedUpdated}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeSearcher{res: search.SearchResult{Total: 5, Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "a"}}}}}
			state := st()
			app, iss := hiddenFeedApp(fake, []string{"acme"}, feedUpdated, state)
			_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=1")
			cursor := nextCursorOf(body)
			if cursor == "" {
				t.Fatal("no cursor")
			}
			if !filterHas(fake.got.Filter.([][]string), search.NotInStrings("company_slug", []string{"acme"})) {
				t.Errorf("hidden company missing from the filter: %#v", fake.got.Filter)
			}

			// Restoring the company advances the profile version: the old cursor is refused.
			restored, restoredIssuer := hiddenFeedApp(fake, nil, feedUpdated.Add(time.Second), state)
			_, body = deckGet(t, restored, restoredIssuer, "/api/v1/me/feed?limit=1&cursor="+cursor)
			if feedMetaOf(body)["reason"] != "source_changed" {
				t.Fatalf("stale cursor reason = %v, want source_changed", feedMetaOf(body)["reason"])
			}
			deckGet(t, restored, restoredIssuer, "/api/v1/me/feed?limit=1")
			for _, g := range fake.got.Filter.([][]string) {
				for _, e := range g {
					if strings.HasPrefix(e, "company_slug") {
						t.Errorf("restored company still filtered: %q", e)
					}
				}
			}
		})
	}
}

// A profile that only hides companies states nothing the feed can match on: no catalogue dump.
func TestFeed_OnlyHiddenCompaniesIsNoUsableFieldsAndNeverSearches(t *testing.T) {
	fake := &fakeSearcher{}
	iss := auth.NewIssuer("test-secret", time.Hour)
	prof := feedProfile{p: userprofile.Profile{ExcludedCompanies: []string{"acme"}, UpdatedAt: &feedUpdated}}
	h := &feedHandlers{search: fake, profiles: prof, excluded: &feedExcludedAt{}, plans: feedPlansFake{}, state: &feedStateFake{}, cursors: feed.NewSigner([]byte("cursor-key"))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/feed", auth.RequireAuth(iss, testVersions), h.Feed)
	_, body := deckGet(t, app, iss, "/api/v1/me/feed")
	if got := feedMetaOf(body)["reason"]; got != feed.ReasonNoUsableFields {
		t.Errorf("reason = %v, want %s", got, feed.ReasonNoUsableFields)
	}
	if fake.got.Limit != 0 {
		t.Errorf("searched the index: %#v", fake.got)
	}
}
