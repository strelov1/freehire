package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

// deckRepo is stubTrackingRepo with a controllable exclusion set, so the deck
// test can assert the caller's judged ids reach the search filter.
type deckRepo struct {
	stubTrackingRepo
	excluded []int64
}

func (r deckRepo) ExcludedJobIDs(context.Context, int64, int32) ([]int64, error) {
	return r.excluded, nil
}

func deckApp(s searcher, excluded []int64) (*fiber.App, *auth.Issuer) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &trackingHandlers{search: s, tracking: jobtracking.New(deckRepo{excluded: excluded})}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/tracking/swipe", auth.RequireAuth(iss, testVersions), h.SwipeDeck)
	return app, iss
}

func deckGet(t *testing.T, app *fiber.App, iss *auth.Issuer, target string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, target, nil)
	if iss != nil {
		token, _ := iss.Issue(7, testTokenVersion)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	var body map[string]any
	return decodeJSON(t, resp, &body), body
}

func TestSwipeDeck_ExcludesJudgedJobsAndForwardsFilters(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{
		Hits:  []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "go-dev-acme-x", Title: "Go Dev"}}},
		Total: 3,
	}}
	app, iss := deckApp(fake, []int64{10, 20})

	status, body := deckGet(t, app, iss, "/api/v1/me/tracking/swipe?q=golang&seniority=senior")
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
	if !filterHas(groups, `enrichment.seniority = "senior"`) {
		t.Errorf("facet filter not forwarded: %#v", groups)
	}
	if !filterHas(groups, "id NOT IN [10, 20]") {
		t.Errorf("exclusion filter missing: %#v", groups)
	}
	// Response envelope carries the public view, never the internal id.
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data len = %d, want 1", len(data))
	}
	if first, _ := data[0].(map[string]any); first["public_slug"] != "go-dev-acme-x" {
		t.Errorf("public_slug = %v", first["public_slug"])
	}
}

func TestSwipeDeck_NoExclusionWhenNothingJudged(t *testing.T) {
	fake := &fakeSearcher{}
	app, iss := deckApp(fake, nil)

	if status, _ := deckGet(t, app, iss, "/api/v1/me/tracking/swipe?regions=eu"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	groups, _ := fake.got.Filter.([][]string)
	for _, g := range groups {
		for _, e := range g {
			if len(e) >= 2 && e[:2] == "id" {
				t.Errorf("unexpected id exclusion with empty judged set: %#v", groups)
			}
		}
	}
	if !filterHas(groups, `regions = "eu"`) {
		t.Errorf("facet filter not forwarded: %#v", groups)
	}
}

func TestSwipeDeck_RequiresAuth(t *testing.T) {
	fake := &fakeSearcher{}
	app, _ := deckApp(fake, nil)

	status, _ := deckGet(t, app, nil, "/api/v1/me/tracking/swipe")
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
	if fake.got.Limit != 0 {
		t.Errorf("searcher called on an unauthenticated request: %#v", fake.got)
	}
}

func TestSwipeDeck_DisabledReturns503(t *testing.T) {
	app, iss := deckApp(nil, nil)
	status, _ := deckGet(t, app, iss, "/api/v1/me/tracking/swipe")
	if status != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}
