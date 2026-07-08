package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/search"
)

func recsApp(t *testing.T, store *resume.Store, s searcher) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &API{issuer: iss, resume: store, search: s}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/recommendations", auth.RequireAuth(iss), h.Recommendations)
	return app, token
}

func getRecs(t *testing.T, app *fiber.App, token string) (status, count int) {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodGet, "/me/recommendations", nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, len(out.Data)
}

// A fresh CV vector (model matches the current embedder) drives a vector-ranked feed.
func TestRecommendations_FreshVectorRanks(t *testing.T) {
	repo := &fakeResumeRepo{embVec: []float64{0.1, 0.2}, embModel: search.CurrentEmbedderModel()}
	store := resume.New(newFakeResumeBlobs(), repo)
	fs := &fakeSearcher{recRes: search.SearchResult{Hits: []search.JobDocument{{}}, Total: 1}}
	app, token := recsApp(t, store, fs)

	status, n := getRecs(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if n != 1 {
		t.Errorf("returned %d jobs, want 1", n)
	}
	if len(fs.gotRecVec) != 2 {
		t.Errorf("searcher got vector %v, want the CV vector", fs.gotRecVec)
	}
}

// A vector from a superseded embedder is stale → empty feed, and no vector search runs.
func TestRecommendations_StaleVectorEmpty(t *testing.T) {
	repo := &fakeResumeRepo{embVec: []float64{0.1, 0.2}, embModel: "old-embedder"}
	store := resume.New(newFakeResumeBlobs(), repo)
	fs := &fakeSearcher{recRes: search.SearchResult{Hits: []search.JobDocument{{}}, Total: 1}}
	app, token := recsApp(t, store, fs)

	status, n := getRecs(t, app, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if n != 0 {
		t.Errorf("returned %d jobs, want 0 (stale vector ignored)", n)
	}
	if fs.gotRecVec != nil {
		t.Error("vector search must not run on a stale vector")
	}
}

// No stored CV vector → a successful empty feed.
func TestRecommendations_NoVectorEmpty(t *testing.T) {
	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
	app, token := recsApp(t, store, &fakeSearcher{})
	if status, n := getRecs(t, app, token); status != fiber.StatusOK || n != 0 {
		t.Errorf("status=%d count=%d, want 200 and 0", status, n)
	}
}

// A facet param on the request is translated to a Meilisearch filter and passed to
// the vector search, so only jobs matching the facet are ranked by the CV vector.
func TestRecommendations_FacetFilterReachesSearch(t *testing.T) {
	repo := &fakeResumeRepo{embVec: []float64{0.1, 0.2}, embModel: search.CurrentEmbedderModel()}
	store := resume.New(newFakeResumeBlobs(), repo)
	fs := &fakeSearcher{recRes: search.SearchResult{Hits: []search.JobDocument{{}}, Total: 1}}
	app, token := recsApp(t, store, fs)

	req := httptest.NewRequest(fiber.MethodGet, "/me/recommendations?work_mode=remote", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	resp.Body.Close()

	want := search.FilterFromValues(url.Values{"work_mode": {"remote"}})
	if !reflect.DeepEqual(fs.gotRecFilter, want) {
		t.Errorf("searcher filter = %#v, want %#v", fs.gotRecFilter, want)
	}
}

// A fresh CV vector whose filtered search matches nothing yields a successful empty
// feed (not an error) — the filtered-empty case the SPA renders as "no matches".
func TestRecommendations_FilterNoMatchEmpty(t *testing.T) {
	repo := &fakeResumeRepo{embVec: []float64{0.1, 0.2}, embModel: search.CurrentEmbedderModel()}
	store := resume.New(newFakeResumeBlobs(), repo)
	fs := &fakeSearcher{recRes: search.SearchResult{}} // no hits
	app, token := recsApp(t, store, fs)

	req := httptest.NewRequest(fiber.MethodGet, "/me/recommendations?work_mode=remote", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Data) != 0 {
		t.Errorf("returned %d jobs, want 0 for a filter that matches nothing", len(out.Data))
	}
	// The vector search still ran (a fresh vector), just with a filter that excluded all.
	if len(fs.gotRecVec) == 0 {
		t.Error("vector search should run for a fresh vector even when the filter matches nothing")
	}
}

func TestRecommendations_Unauthenticated(t *testing.T) {
	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
	app, _ := recsApp(t, store, &fakeSearcher{})
	if status, _ := getRecs(t, app, ""); status != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", status)
	}
}
