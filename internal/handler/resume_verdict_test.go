package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/userprofile"
)

// twoQueryFacets returns canned results in call order (query A = role total, query
// B = uncovered total + distribution) and records the params so tests can assert
// the filters the handler built.
type twoQueryFacets struct {
	calls   []search.FacetParams
	results []search.FacetResult
	err     error
}

func (f *twoQueryFacets) FacetCounts(_ context.Context, p search.FacetParams) (search.FacetResult, error) {
	idx := len(f.calls)
	f.calls = append(f.calls, p)
	if f.err != nil {
		return search.FacetResult{}, f.err
	}
	if idx < len(f.results) {
		return f.results[idx], nil
	}
	return search.FacetResult{}, nil
}

// coverageFacets: role has 1000 open vacancies (query A), 370 uncovered (query B)
// with kubernetes the biggest gap.
func coverageFacets() *twoQueryFacets {
	return &twoQueryFacets{results: []search.FacetResult{
		{Total: 1000},
		{Total: 370, Facets: map[string]map[string]int64{"skills": {"kubernetes": 190, "kafka": 120, "grpc": 60}}},
	}}
}

func ownedProfile() *fakeProfileRepo {
	return &fakeProfileRepo{getRet: db.UserProfile{
		UserID: 1, Specializations: []string{"backend"}, Skills: []string{"go"},
	}}
}

func verdictApp(t *testing.T, repo *fakeProfileRepo, fc facetCounter) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &API{issuer: iss, userProfile: userprofile.New(repo), facets: fc}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/profile/verdict", auth.RequireAuth(iss), h.GetResumeVerdict)
	return app, token
}

func getVerdict(t *testing.T, app *fiber.App, target, token string) (int, map[string]any) {
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

func dataOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	d, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %v", body)
	}
	return d
}

func TestGetVerdict_FacetsUnconfigured503(t *testing.T) {
	app, token := verdictApp(t, ownedProfile(), nil)
	status, _ := getVerdict(t, app, "/me/profile/verdict", token)
	if status != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}

func TestGetVerdict_ProfileNotFound404(t *testing.T) {
	repo := &fakeProfileRepo{getErr: userprofile.ErrNotFound}
	app, token := verdictApp(t, repo, coverageFacets())
	status, _ := getVerdict(t, app, "/me/profile/verdict", token)
	if status != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestGetVerdict_CoverageFromFacets(t *testing.T) {
	app, token := verdictApp(t, ownedProfile(), coverageFacets())
	status, body := getVerdict(t, app, "/me/profile/verdict", token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	d := dataOf(t, body)
	if d["total"].(float64) != 1000 || d["covered"].(float64) != 630 || d["coverage_percent"].(float64) != 63 {
		t.Errorf("coverage = total %v covered %v pct %v, want 1000/630/63", d["total"], d["covered"], d["coverage_percent"])
	}
	gaps := d["gaps"].([]any)
	if len(gaps) != 3 {
		t.Fatalf("gaps len = %d, want 3", len(gaps))
	}
	top := gaps[0].(map[string]any)
	if top["name"] != "kubernetes" || top["new_vacancies"].(float64) != 190 || top["unlock_percent"].(float64) != 19 {
		t.Errorf("top gap = %v, want kubernetes/190/19", top)
	}
}

func TestGetVerdict_DefaultsToProfileSpecializations(t *testing.T) {
	fc := coverageFacets()
	app, token := verdictApp(t, ownedProfile(), fc)
	if status, _ := getVerdict(t, app, "/me/profile/verdict", token); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	want := [][]string{{`enrichment.category = "backend"`}}
	if !reflect.DeepEqual(fc.calls[0].Filter, want) {
		t.Errorf("role filter = %#v, want %#v", fc.calls[0].Filter, want)
	}
}

func TestGetVerdict_FilterOverridesRole(t *testing.T) {
	fc := coverageFacets()
	app, token := verdictApp(t, ownedProfile(), fc)
	if status, _ := getVerdict(t, app, "/me/profile/verdict?category=data", token); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	want := [][]string{{`enrichment.category = "data"`}}
	if !reflect.DeepEqual(fc.calls[0].Filter, want) {
		t.Errorf("role filter = %#v, want %#v (profile spec must not apply)", fc.calls[0].Filter, want)
	}
}

func TestGetVerdict_RequestSkillsAreNotAFilter(t *testing.T) {
	fc := coverageFacets()
	app, token := verdictApp(t, ownedProfile(), fc)
	// ?skills=rust must not filter the role by rust, and rust is not an owned skill:
	// the role stays the profile's default and the uncovered query excludes only "go".
	if status, _ := getVerdict(t, app, "/me/profile/verdict?skills=rust", token); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	roleWant := [][]string{{`enrichment.category = "backend"`}}
	if !reflect.DeepEqual(fc.calls[0].Filter, roleWant) {
		t.Errorf("role filter = %#v, want %#v (request skills stripped)", fc.calls[0].Filter, roleWant)
	}
	uncoveredWant := [][]string{{`enrichment.category = "backend"`}, {`skills != "go"`}}
	if !reflect.DeepEqual(fc.calls[1].Filter, uncoveredWant) {
		t.Errorf("uncovered filter = %#v, want %#v (excludes owned go, not request rust)", fc.calls[1].Filter, uncoveredWant)
	}
}
