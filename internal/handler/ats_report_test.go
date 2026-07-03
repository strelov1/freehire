package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/atscheck"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/userprofile"
)

// fakeATSCache is an in-memory atsReviewStore for the DB-less handler tests.
type fakeATSCache struct{ blob map[int64][]byte }

func newFakeATSCache() *fakeATSCache { return &fakeATSCache{blob: map[int64][]byte{}} }

func (f *fakeATSCache) GetUserATSAnalysis(_ context.Context, id int64) ([]byte, error) {
	return f.blob[id], nil
}
func (f *fakeATSCache) SetUserATSAnalysis(_ context.Context, arg db.SetUserATSAnalysisParams) error {
	f.blob[arg.ID] = arg.ResumeAtsAnalysis
	return nil
}

// atsModel is a canned llms.Model for the LLM review path.
type atsModel struct{ resp string }

func (m atsModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: m.resp}}}, nil
}
func (atsModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

// atsFacets returns a role skills distribution for the keyword-match.
func atsFacets() *fakeFacetCounter {
	return &fakeFacetCounter{res: search.FacetResult{
		Total:  1000,
		Facets: map[string]map[string]int64{"skills": {"go": 600, "kubernetes": 400, "kafka": 300}},
	}}
}

func atsApp(t *testing.T, repo *fakeProfileRepo, fc facetCounter, store *resume.Store) (*fiber.App, string) {
	t.Helper()
	return atsAppWith(t, repo, fc, store, atscheck.NewAnalyzer(nil), newFakeATSCache())
}

func atsAppWith(t *testing.T, repo *fakeProfileRepo, fc facetCounter, store *resume.Store, analyzer *atscheck.Analyzer, cache atsReviewStore) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &API{
		issuer:      iss,
		userProfile: userprofile.New(repo),
		facets:      fc,
		resume:      store,
		atsAnalyzer: analyzer,
		atsCache:    cache,
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss)
	app.Get("/me/profile/ats-report", g, h.GetATSReport)
	app.Post("/me/profile/ats-report", g, h.PostATSReport)
	return app, token
}

func postATS(t *testing.T, app *fiber.App, target, token string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodPost, target, nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func getATS(t *testing.T, app *fiber.App, target, token string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodGet, target, nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func storeWithCV(t *testing.T, text string) *resume.Store {
	t.Helper()
	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
	if _, err := store.Put(context.Background(), 1, "text/plain", []byte(text)); err != nil {
		t.Fatalf("seed CV: %v", err)
	}
	return store
}

func TestGetATS_FacetsUnconfigured503(t *testing.T) {
	app, token := atsApp(t, ownedProfile(), nil, storeWithCV(t, "x"))
	if status, _ := getATS(t, app, "/me/profile/ats-report", token); status != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}

func TestGetATS_NotOwned404(t *testing.T) {
	repo := &fakeProfileRepo{getErr: userprofile.ErrNotFound}
	app, token := atsApp(t, repo, atsFacets(), storeWithCV(t, "x"))
	if status, _ := getATS(t, app, "/me/profile/ats-report", token); status != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestGetATS_NoCVStored(t *testing.T) {
	// Storage enabled but nothing stored → 200 with has_cv=false.
	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
	app, token := atsApp(t, ownedProfile(), atsFacets(), store)
	status, body := getATS(t, app, "/me/profile/ats-report", token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	d := body["data"].(map[string]any)
	if d["has_cv"] != false {
		t.Errorf("has_cv = %v, want false", d["has_cv"])
	}
}

func TestGetATS_HappyPathScoresStoredCV(t *testing.T) {
	// "Golang" (not bare "Go") so skilltag unambiguously extracts "go" — bare "go" is
	// deliberately not tagged, which is the matcher's precision guard.
	cv := `Ilya Ivanov
ilya@example.com  +1 415 555 0134

Experience
Senior Backend Engineer (2021 - 2026)
- Built distributed systems in Golang and Kafka
- Ran services on Kubernetes

Skills
Golang, Kafka, Kubernetes, PostgreSQL`
	app, token := atsApp(t, ownedProfile(), atsFacets(), storeWithCV(t, cv))
	status, body := getATS(t, app, "/me/profile/ats-report", token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	d := body["data"].(map[string]any)
	if d["has_cv"] != true {
		t.Fatalf("has_cv = %v, want true", d["has_cv"])
	}
	report := d["report"].(map[string]any)
	// CV has go+kafka+kubernetes; role top skills are the same three → keyword_match 100.
	if report["keyword_match"].(float64) != 100 {
		t.Errorf("keyword_match = %v, want 100", report["keyword_match"])
	}
	if report["overall"].(float64) <= 0 {
		t.Errorf("overall = %v, want > 0", report["overall"])
	}
	// Phase 1 (no LLM review): no content-quality field.
	if _, ok := report["content_quality"]; ok {
		t.Errorf("content_quality present without an LLM review")
	}
}

func TestPostATS_RunsLLMReviewAndCaches(t *testing.T) {
	cache := newFakeATSCache()
	analyzer := atscheck.NewAnalyzer(llm.NewWithModel(atsModel{
		resp: `{"content_quality":80,"findings":["Quantify your impact."]}`,
	}))
	app, token := atsAppWith(t, ownedProfile(), atsFacets(), storeWithCV(t, "some cv text here"), analyzer, cache)

	status, body := postATS(t, app, "/me/profile/ats-report", token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	report := dataOf(t, body)["report"].(map[string]any)
	if report["content_quality"].(float64) != 80 {
		t.Errorf("content_quality = %v, want 80", report["content_quality"])
	}
	if len(report["findings"].([]any)) != 1 {
		t.Errorf("findings = %v, want 1", report["findings"])
	}
	if len(cache.blob[1]) == 0 {
		t.Errorf("review was not cached for the user")
	}
}

func TestPostATS_NoLLMDegrades(t *testing.T) {
	// Nil analyzer (LLM off) → 200 deterministic, no content-quality, nothing cached.
	cache := newFakeATSCache()
	app, token := atsAppWith(t, ownedProfile(), atsFacets(), storeWithCV(t, "some cv text here"),
		atscheck.NewAnalyzer(nil), cache)
	status, body := postATS(t, app, "/me/profile/ats-report", token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if _, ok := dataOf(t, body)["report"].(map[string]any)["content_quality"]; ok {
		t.Errorf("content_quality present with LLM off")
	}
	if len(cache.blob[1]) != 0 {
		t.Errorf("nothing should be cached with LLM off")
	}
}
