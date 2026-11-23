package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/search/feed"
	"github.com/strelov1/freehire/internal/search/search"
)

func feedSourceApp(fake *fakeSearcher, st *feedStateFake) (*fiber.App, *auth.Issuer) {
	return feedStateApp(fake, &feedExcludedAt{}, feedUpdated, st)
}

func feedPut(t *testing.T, app *fiber.App, iss *auth.Issuer, body string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPut, "/api/v1/me/feed/source", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	token, _ := iss.Issue(7, testTokenVersion)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestFeed_DefaultSourceIsProfile(t *testing.T) {
	app, iss := feedSourceApp(&fakeSearcher{}, &feedStateFake{})
	_, body := deckGet(t, app, iss, "/api/v1/me/feed")
	meta := body["meta"].(map[string]any)
	if meta["source"] != "profile" || meta["saved_search_id"] != nil {
		t.Errorf("meta = %v", meta)
	}
}

func TestFeed_SavedSearchSourceUsesItsFilters(t *testing.T) {
	fake := &fakeSearcher{}
	st := &feedStateFake{searches: map[int64]feedSearch{2: {Query: "q=golang&seniority=senior", UpdatedAt: feedUpdated}}}
	app, iss := feedSourceApp(fake, st)
	if code := feedPut(t, app, iss, `{"source":"saved_search","saved_search_id":2}`); code != fiber.StatusNoContent {
		t.Fatalf("put = %d", code)
	}
	_, body := deckGet(t, app, iss, "/api/v1/me/feed")
	meta := body["meta"].(map[string]any)
	if meta["source"] != "saved_search" || meta["saved_search_id"] != float64(2) {
		t.Errorf("meta = %v", meta)
	}
	groups, _ := fake.got.Filter.([][]string)
	if fake.got.Query != "golang" || !filterHas(groups, `enrichment.seniority = "senior"`) {
		t.Errorf("saved-search query not applied: %#v", fake.got)
	}
	if len(fake.got.Vector) != 0 {
		t.Errorf("a vector makes the search semantic-only, which would drop q")
	}
	if len(st.sources) == 0 || st.sources[0] != "search:2" {
		t.Errorf("watermark source = %v", st.sources)
	}
	// Back to profile.
	if code := feedPut(t, app, iss, `{"source":"profile"}`); code != fiber.StatusNoContent || st.source != 0 {
		t.Errorf("put profile = %d, stored %d", code, st.source)
	}
}

func TestFeed_SavedSearchWithoutQRanksByProfileVector(t *testing.T) {
	fake := &fakeSearcher{}
	st := &feedStateFake{source: 2, searches: map[int64]feedSearch{2: {Query: "seniority=senior", UpdatedAt: feedUpdated}}}
	app, iss := feedSourceApp(fake, st)
	deckGet(t, app, iss, "/api/v1/me/feed")
	if len(fake.got.Vector) == 0 {
		t.Errorf("profile skills should rank a saved search without q")
	}
}

func TestFeed_DeletedSearchNeverFallsBackToProfile(t *testing.T) {
	fake := &fakeSearcher{}
	app, iss := feedSourceApp(fake, &feedStateFake{source: 2})
	status, body := deckGet(t, app, iss, "/api/v1/me/feed")
	meta := body["meta"].(map[string]any)
	if status != 200 || meta["reason"] != feedReasonSearchDeleted || len(body["data"].([]any)) != 0 {
		t.Errorf("status %d body %v", status, body)
	}
	if fake.got.Limit != 0 {
		t.Errorf("searched the index for a deleted search")
	}
}

func TestFeed_SavedSearchThatNarrowsNothing(t *testing.T) {
	for _, q := range []string{"", "watchlist=true&q=go"} {
		fake := &fakeSearcher{}
		app, iss := feedSourceApp(fake, &feedStateFake{source: 2, searches: map[int64]feedSearch{2: {Query: q, UpdatedAt: feedUpdated}}})
		_, body := deckGet(t, app, iss, "/api/v1/me/feed")
		if got := body["meta"].(map[string]any)["reason"]; got != feed.ReasonNoUsableFields || fake.got.Limit != 0 {
			t.Errorf("query %q: reason %v, searched %#v", q, got, fake.got)
		}
	}
}

func TestFeed_SavedSearchWithNoMatchesIsAnEmptyPageWithoutReason(t *testing.T) {
	app, iss := feedSourceApp(&fakeSearcher{}, &feedStateFake{source: 2, searches: map[int64]feedSearch{2: {Query: "q=golang", UpdatedAt: feedUpdated}}})
	_, body := deckGet(t, app, iss, "/api/v1/me/feed")
	meta := body["meta"].(map[string]any)
	if _, has := meta["reason"]; has || len(body["data"].([]any)) != 0 {
		t.Errorf("body = %v", body)
	}
}

func TestFeed_PutSourceRejectsForeignDerivedAndBadBodies(t *testing.T) {
	st := &feedStateFake{source: 0, searches: map[int64]feedSearch{3: {Query: "q=go", Derived: true}}}
	app, iss := feedSourceApp(&fakeSearcher{}, st)
	for body, want := range map[string]int{
		`{"source":"saved_search","saved_search_id":9}`: fiber.StatusNotFound, // not the caller's
		`{"source":"saved_search","saved_search_id":3}`: fiber.StatusNotFound, // profile-derived
		`{"source":"saved_search"}`:                     fiber.StatusBadRequest,
		`{"source":"nope"}`:                             fiber.StatusBadRequest,
		`not json`:                                      fiber.StatusBadRequest,
	} {
		if code := feedPut(t, app, iss, body); code != want {
			t.Errorf("%s: %d, want %d", body, code, want)
		}
	}
	if st.source != 0 {
		t.Errorf("stored source changed: %d", st.source)
	}
}

func TestFeed_CursorFromAnotherSourceIs400(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 5, Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "a"}}}}}
	st := &feedStateFake{searches: map[int64]feedSearch{2: {Query: "q=golang", UpdatedAt: feedUpdated}}}
	app, iss := feedSourceApp(fake, st)
	_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=1")
	cursor := nextCursorOf(body)
	if cursor == "" {
		t.Fatal("no cursor")
	}
	st.source = 2
	if status, _ := deckGet(t, app, iss, "/api/v1/me/feed?cursor="+cursor); status != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}

func TestFeed_ProfileEditRestartsASavedSearchRankedByIt(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 9, Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "a"}}}}}
	st := &feedStateFake{source: 2, searches: map[int64]feedSearch{2: {Query: "seniority=senior", UpdatedAt: feedUpdated.Add(-time.Hour)}}}
	app, iss := feedStateApp(fake, &feedExcludedAt{}, feedUpdated, st)
	_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=1")
	cur := nextCursorOf(body)

	edited, iss2 := feedStateApp(fake, &feedExcludedAt{}, feedUpdated.Add(time.Second), st)
	_, body = deckGet(t, edited, iss2, "/api/v1/me/feed?cursor="+cur)
	if body["meta"].(map[string]any)["reason"] != feedReasonSourceChanged {
		t.Fatalf("meta = %v, want source_changed", body["meta"])
	}
}
