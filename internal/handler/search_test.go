package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

type fakeSearcher struct {
	got search.SearchParams
	res search.SearchResult
	err error
	// similar-jobs call recording
	similarLimit int
	similarHits  []search.JobDocument
	similarErr   error
	// embed-text call recording + canned result
	gotEmbedText string
	embedVec     []float64
	embedModel   string
	embedErr     error
	// recommend-by-vector call recording + canned result
	gotRecVec []float64
	recRes    search.SearchResult
	recErr    error
}

func (f *fakeSearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	f.got = p
	return f.res, f.err
}

func (f *fakeSearcher) SimilarJobs(_ context.Context, _ int64, limit int) ([]search.JobDocument, error) {
	f.similarLimit = limit
	return f.similarHits, f.similarErr
}

func (f *fakeSearcher) EmbedText(_ context.Context, _, text string) ([]float64, string, error) {
	f.gotEmbedText = text
	return f.embedVec, f.embedModel, f.embedErr
}

func (f *fakeSearcher) RecommendByVector(_ context.Context, v []float64, _, _ int) (search.SearchResult, error) {
	f.gotRecVec = v
	return f.recRes, f.recErr
}

func searchApp(s searcher) *fiber.App {
	h := &API{search: s}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/jobs/search", h.SearchJobs)
	return app
}

func doGet(t *testing.T, app *fiber.App, target string) (int, map[string]any) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, target, nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func TestSearchJobs_DisabledReturns503(t *testing.T) {
	app := searchApp(nil) // search not configured
	status, _ := doGet(t, app, "/jobs/search?q=go")
	if status != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}

func TestSearchJobs_PassesParamsAndShapesResponse(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{
		Hits:  []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "go-dev-acme-x", Title: "Go Dev"}}},
		Total: 5,
	}}
	app := searchApp(fake)

	status, body := doGet(t, app, "/jobs/search?q=golang&limit=10&offset=20&seniority=senior&regions=eu&semantic_ratio=0.3")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	// Params mapped onto SearchParams.
	if fake.got.Query != "golang" {
		t.Errorf("Query = %q, want golang", fake.got.Query)
	}
	if fake.got.Limit != 10 || fake.got.Offset != 20 {
		t.Errorf("limit/offset = %d/%d, want 10/20", fake.got.Limit, fake.got.Offset)
	}
	if fake.got.SemanticRatio != 0.3 {
		t.Errorf("SemanticRatio = %v, want 0.3", fake.got.SemanticRatio)
	}
	groups, ok := fake.got.Filter.([][]string)
	if !ok {
		t.Fatalf("Filter = %#v, want [][]string", fake.got.Filter)
	}
	if !filterHas(groups, `enrichment.seniority = "senior"`) || !filterHas(groups, `regions = "eu"`) {
		t.Errorf("Filter missing facets: %#v", groups)
	}

	// Response envelope: data carries the public view (no id), meta has totals.
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data len = %d, want 1", len(data))
	}
	first, _ := data[0].(map[string]any)
	if first["public_slug"] != "go-dev-acme-x" {
		t.Errorf("public_slug = %v", first["public_slug"])
	}
	if _, leaked := first["id"]; leaked {
		t.Errorf("internal id leaked in search result: %v", first)
	}
	meta, _ := body["meta"].(map[string]any)
	if meta["total"].(float64) != 5 || meta["limit"].(float64) != 10 || meta["offset"].(float64) != 20 {
		t.Errorf("meta = %v", meta)
	}
}

func TestSearchJobs_DeepPaginationRejected(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	// offset+limit past the window must be refused before the backend is touched.
	status, body := doGet(t, app, "/jobs/search?offset=10000&limit=100")
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if fake.got.Limit != 0 {
		t.Errorf("searcher was called despite deep pagination: %#v", fake.got)
	}
	if body["error"] == nil {
		t.Errorf("expected an error message, got %v", body)
	}
}

func TestSearchJobs_PaginationAtWindowBoundaryAllowed(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	// offset+limit exactly at the window is still allowed and reaches the backend.
	status, _ := doGet(t, app, "/jobs/search?offset=9900&limit=100")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if fake.got.Offset != 9900 || fake.got.Limit != 100 {
		t.Errorf("offset/limit = %d/%d, want 9900/100 (within window)", fake.got.Offset, fake.got.Limit)
	}
}

func TestSearchJobs_DefaultsToKeywordSemanticRatio(t *testing.T) {
	// Semantic search is opt-in: the default routes to the always-fresh facet
	// index (ratio 0), never the separate, optionally-built semantic index.
	fake := &fakeSearcher{}
	app := searchApp(fake)
	doGet(t, app, "/jobs/search?q=go")
	if fake.got.SemanticRatio != 0 {
		t.Errorf("default SemanticRatio = %v, want 0 (keyword)", fake.got.SemanticRatio)
	}
}

func TestSearchJobs_SemanticRatioOptIn(t *testing.T) {
	// An explicit semantic_ratio>0 still reaches the backend (routes to hybrid).
	fake := &fakeSearcher{}
	app := searchApp(fake)
	doGet(t, app, "/jobs/search?q=go&semantic_ratio=0.5")
	if fake.got.SemanticRatio != 0.5 {
		t.Errorf("opt-in SemanticRatio = %v, want 0.5", fake.got.SemanticRatio)
	}
}

func TestSearchJobs_BackendErrorMaps500(t *testing.T) {
	fake := &fakeSearcher{err: context.DeadlineExceeded}
	app := searchApp(fake)
	status, _ := doGet(t, app, "/jobs/search?q=go")
	if status != fiber.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
}

func TestSearchJobs_ExcludeAndAndMode(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	doGet(t, app, "/jobs/search?seniority=senior&seniority_exclude=junior&skills=go&skills=react&skills_mode=and")

	groups, ok := fake.got.Filter.([][]string)
	if !ok {
		t.Fatalf("Filter = %#v, want [][]string", fake.got.Filter)
	}
	if !filterHas(groups, `enrichment.seniority = "senior"`) {
		t.Error("missing include seniority=senior")
	}
	if !filterHas(groups, `enrichment.seniority != "junior"`) {
		t.Error("missing exclude seniority!=junior")
	}
	// AND mode: each skill must be its own single-element group (ANDed), not one
	// OR group of both.
	for _, skill := range []string{"go", "react"} {
		expr := `skills = "` + skill + `"`
		found := false
		for _, g := range groups {
			if len(g) == 1 && g[0] == expr {
				found = true
			}
		}
		if !found {
			t.Errorf("skills AND mode: %q not in its own group; groups=%#v", expr, groups)
		}
	}
}

func filterHas(groups [][]string, expr string) bool {
	for _, g := range groups {
		for _, e := range g {
			if e == expr {
				return true
			}
		}
	}
	return false
}

func TestSearchJobs_DefaultSortIsPostedAtForEmptyQuery(t *testing.T) {
	fake := &fakeSearcher{}
	app := searchApp(fake)

	// No query text, no sort → freshest by posting date first.
	if status, _ := doGet(t, app, "/jobs/search"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.got.Sort) != 1 || fake.got.Sort[0] != "posted_at:desc" {
		t.Errorf("Sort = %v, want [posted_at:desc] for empty q", fake.got.Sort)
	}

	// Text query, no sort → relevance (nil sort).
	if status, _ := doGet(t, app, "/jobs/search?q=golang"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if fake.got.Sort != nil {
		t.Errorf("Sort = %v, want nil (relevance) for a text query", fake.got.Sort)
	}

	// Explicit sort always wins, q or not — and created_at is still accepted.
	if status, _ := doGet(t, app, "/jobs/search?q=golang&sort=created_at&order=asc"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.got.Sort) != 1 || fake.got.Sort[0] != "created_at:asc" {
		t.Errorf("Sort = %v, want [created_at:asc] for explicit sort", fake.got.Sort)
	}
}
