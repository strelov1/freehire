package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/searchprofile"
	"github.com/strelov1/freehire/internal/verdict"
)

var errBoom = errors.New("boom")

// fakeLLM is a stub llms.Model returning a canned JSON response.
type fakeLLM struct {
	resp string
	err  error
}

func (f fakeLLM) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: f.resp}}}, nil
}
func (fakeLLM) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

// verdictApp mounts the GET/POST verdict routes on a handler backed by the given
// fakes. analyzer may wrap a nil client (LLM off) or a fake model.
func verdictApp(t *testing.T, repo *fakeProfileRepo, fc facetCounter, analyzer *verdict.Analyzer) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &API{issuer: iss, searchProfile: searchprofile.New(repo), facets: fc, verdictAnalyzer: analyzer}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/profiles/:id/verdict", auth.RequireAuth(iss), h.GetResumeVerdict)
	app.Post("/me/profiles/:id/verdict", auth.RequireAuth(iss), h.ResumeVerdict)
	return app, token
}

// marketFacets is a canned skills distribution: go 60%, python 50% (both must-have),
// docker 30% (a non-must-have gap). Total 100.
func marketFacets() *fakeFacetCounter {
	return &fakeFacetCounter{res: search.FacetResult{
		Total:  100,
		Facets: map[string]map[string]int64{"skills": {"go": 60, "python": 50, "docker": 30}},
	}}
}

func ownedProfile() *fakeProfileRepo {
	return &fakeProfileRepo{getRet: db.SearchProfile{
		ID: 5, UserID: 1, Specializations: []string{"backend"}, Skills: []string{"go"},
	}}
}

func doVerdict(t *testing.T, app *fiber.App, method, target, body, token string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	}
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

func dataOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	d, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %v", body)
	}
	return d
}

func TestGetVerdict_FacetsUnconfigured503(t *testing.T) {
	app, token := verdictApp(t, ownedProfile(), nil, verdict.NewAnalyzer(nil))
	status, _ := doVerdict(t, app, fiber.MethodGet, "/me/profiles/5/verdict", "", token)
	if status != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}

func TestGetVerdict_ProfileNotFound404(t *testing.T) {
	repo := &fakeProfileRepo{getErr: searchprofile.ErrNotFound}
	app, token := verdictApp(t, repo, marketFacets(), verdict.NewAnalyzer(nil))
	status, _ := doVerdict(t, app, fiber.MethodGet, "/me/profiles/999/verdict", "", token)
	if status != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestGetVerdict_DeterministicNoAnalysis(t *testing.T) {
	app, token := verdictApp(t, ownedProfile(), marketFacets(), verdict.NewAnalyzer(nil))
	status, body := doVerdict(t, app, fiber.MethodGet, "/me/profiles/5/verdict", "", token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	d := dataOf(t, body)
	// go covered of {go,python,docker} → 33%.
	if d["stack_match"].(float64) != 33 {
		t.Errorf("stack_match = %v, want 33", d["stack_match"])
	}
	if len(d["skills"].([]any)) != 3 {
		t.Errorf("skills len = %d, want 3", len(d["skills"].([]any)))
	}
	if _, ok := d["coherence"]; ok {
		t.Errorf("coherence should be omitted without analysis, got %v", d["coherence"])
	}
}

func TestGetVerdict_MergesStoredAnalysis(t *testing.T) {
	repo := ownedProfile()
	repo.getRet.ResumeAnalysis = []byte(`{"coherence":88,"advice":{"python":"Show a Python service."},"analyzed_at":"2026-01-01T00:00:00Z"}`)
	app, token := verdictApp(t, repo, marketFacets(), verdict.NewAnalyzer(nil))
	status, body := doVerdict(t, app, fiber.MethodGet, "/me/profiles/5/verdict", "", token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	d := dataOf(t, body)
	if d["coherence"].(float64) != 88 {
		t.Errorf("coherence = %v, want 88", d["coherence"])
	}
	// python is a must-have gap → advice attached.
	for _, s := range d["skills"].([]any) {
		sk := s.(map[string]any)
		if sk["name"] == "python" && sk["advice"] == nil {
			t.Errorf("python gap should carry stored advice")
		}
	}
}

func TestPostVerdict_EmptyText400(t *testing.T) {
	app, token := verdictApp(t, ownedProfile(), marketFacets(), verdict.NewAnalyzer(nil))
	status, _ := doVerdict(t, app, fiber.MethodPost, "/me/profiles/5/verdict", `{"text":"   "}`, token)
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestPostVerdict_LLMUnconfiguredDegrades(t *testing.T) {
	repo := ownedProfile()
	app, token := verdictApp(t, repo, marketFacets(), verdict.NewAnalyzer(nil))
	status, body := doVerdict(t, app, fiber.MethodPost, "/me/profiles/5/verdict", `{"text":"my resume"}`, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	d := dataOf(t, body)
	if _, ok := d["coherence"]; ok {
		t.Errorf("coherence should be omitted when LLM is off")
	}
	if repo.setAnalysis.ResumeAnalysis != nil {
		t.Errorf("nothing should be persisted when LLM is off")
	}
}

func TestPostVerdict_AnalyzesAndPersists(t *testing.T) {
	repo := ownedProfile()
	model := fakeLLM{resp: `{"coherence":77,"advice":{"python":"Ship a Python project and quantify impact."}}`}
	analyzer := verdict.NewAnalyzer(llm.NewWithModel(model))
	app, token := verdictApp(t, repo, marketFacets(), analyzer)

	status, body := doVerdict(t, app, fiber.MethodPost, "/me/profiles/5/verdict", `{"text":"my resume text"}`, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	d := dataOf(t, body)
	if d["coherence"].(float64) != 77 {
		t.Errorf("coherence = %v, want 77", d["coherence"])
	}
	if repo.setAnalysis.ResumeAnalysis == nil {
		t.Fatalf("derived analysis should be persisted")
	}
	if !strings.Contains(string(repo.setAnalysis.ResumeAnalysis), "coherence") {
		t.Errorf("persisted blob should contain coherence, got %s", repo.setAnalysis.ResumeAnalysis)
	}
}

func TestPostVerdict_LLMErrorDegrades(t *testing.T) {
	repo := ownedProfile()
	// LLM configured but the call errors: the verdict still renders deterministically
	// (200, no coherence) and nothing is persisted.
	analyzer := verdict.NewAnalyzer(llm.NewWithModel(fakeLLM{err: errBoom}))
	app, token := verdictApp(t, repo, marketFacets(), analyzer)

	status, body := doVerdict(t, app, fiber.MethodPost, "/me/profiles/5/verdict", `{"text":"my resume"}`, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (degraded)", status)
	}
	if _, ok := dataOf(t, body)["coherence"]; ok {
		t.Errorf("coherence should be omitted when the LLM errors")
	}
	if repo.setAnalysis.ResumeAnalysis != nil {
		t.Errorf("nothing should be persisted when the LLM errors")
	}
}

func TestPostVerdict_ProfileNotFound404(t *testing.T) {
	repo := &fakeProfileRepo{getErr: searchprofile.ErrNotFound}
	app, token := verdictApp(t, repo, marketFacets(), verdict.NewAnalyzer(nil))
	status, _ := doVerdict(t, app, fiber.MethodPost, "/me/profiles/999/verdict", `{"text":"r"}`, token)
	if status != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}
