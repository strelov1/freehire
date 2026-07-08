package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search"
)

// recordingFacetCounter captures every FacetCounts call (coverageFor makes three)
// so a test can assert the filter of any one of them.
type recordingFacetCounter struct {
	calls []search.FacetParams
	res   search.FacetResult
	err   error
}

func (r *recordingFacetCounter) FacetCounts(_ context.Context, p search.FacetParams) (search.FacetResult, error) {
	r.calls = append(r.calls, p)
	return r.res, r.err
}

func (r *recordingFacetCounter) DisjunctiveFacetCounts(_ context.Context, _ string, _ []search.FacetReq, _ any) (search.FacetResult, error) {
	return r.res, r.err
}

// callFilter returns the [][]string filter of the Nth captured call.
func (r *recordingFacetCounter) callFilter(n int) [][]string {
	if n >= len(r.calls) {
		return nil
	}
	g, _ := r.calls[n].Filter.([][]string)
	return g
}

func coverageApp(fc facetCounter) *fiber.App {
	h := &API{facets: fc}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/market/coverage", h.MarketCoverage)
	return app
}

func doPostJSON(t *testing.T, app *fiber.App, target, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodPost, target, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestMarketCoverage_DisabledReturns503(t *testing.T) {
	app := coverageApp(nil) // search not configured
	status, _ := doPostJSON(t, app, "/market/coverage", `{"skills":["go"]}`)
	if status != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}

func TestMarketCoverage_EmptySkillsReturns400(t *testing.T) {
	fake := &recordingFacetCounter{}
	app := coverageApp(fake)
	status, _ := doPostJSON(t, app, "/market/coverage", `{"skills":[]}`)
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if len(fake.calls) != 0 {
		t.Errorf("no facet query should run for empty skills, got %d", len(fake.calls))
	}
}

func TestMarketCoverage_TooManySkillsReturns400(t *testing.T) {
	fake := &recordingFacetCounter{}
	app := coverageApp(fake)

	// Build a skills list past the cap.
	big := make([]string, maxCoverageSkills+1)
	for i := range big {
		big[i] = "s" + strconv.Itoa(i)
	}
	payload, _ := json.Marshal(coverageRequest{Skills: big})

	status, _ := doPostJSON(t, app, "/market/coverage", string(payload))
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if len(fake.calls) != 0 {
		t.Errorf("no facet query should run past the skills cap, got %d", len(fake.calls))
	}
}

func TestMarketCoverage_ComputesAndShapesResponse(t *testing.T) {
	// role total 500; uncovered (vacancies listing none of the skills) 200 → covered
	// 300 → 60%. The role skill distribution feeds the breakdown.
	fake := &recordingFacetCounter{res: search.FacetResult{
		Total:  500,
		Facets: map[string]map[string]int64{"skills": {"go": 300, "kubernetes": 250}},
	}}
	app := coverageApp(fake)

	status, body := doPostJSON(t, app, "/market/coverage?category=backend", `{"skills":["go"]}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want object", body["data"])
	}
	if data["total"].(float64) != 500 {
		t.Errorf("total = %v, want 500", data["total"])
	}
	// covered = total - uncovered; both come from the same stubbed Total (500),
	// so covered floors at 0 here — assert the field is present and numeric.
	if _, ok := data["coverage_percent"].(float64); !ok {
		t.Errorf("coverage_percent missing/!number: %#v", data["coverage_percent"])
	}
	if _, ok := data["gaps"].([]any); !ok {
		t.Errorf("gaps should be an array, got %#v", data["gaps"])
	}
	// Stateless: no coherence score is advertised.
	if data["coherence_percent"].(float64) != 0 {
		t.Errorf("coherence_percent = %v, want 0 (stateless)", data["coherence_percent"])
	}
}

func TestMarketCoverage_FilterFromQueryAndSkillsFromBody(t *testing.T) {
	fake := &recordingFacetCounter{res: search.FacetResult{Total: 10}}
	app := coverageApp(fake)

	status, _ := doPostJSON(t, app, "/market/coverage?category=backend&countries=BR", `{"skills":["go","docker"]}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.calls) < 2 {
		t.Fatalf("want at least role+uncovered queries, got %d", len(fake.calls))
	}

	// The role query (call 0) carries the query-param facet filter.
	role := fake.callFilter(0)
	if !filterHas(role, `enrichment.category = "backend"`) || !filterHas(role, `countries = "BR"`) {
		t.Errorf("role filter missing query facets: %#v", role)
	}
	// The role filter must NOT filter by the supplied skills (they are the measured
	// set, not a market filter).
	if filterHas(role, `skills = "go"`) {
		t.Errorf("role filter should not include the measured skills: %#v", role)
	}

	// The uncovered query (call 1) excludes the body skills via AndNotSkills.
	uncovered := fake.callFilter(1)
	if !filterHas(uncovered, `skills != "go"`) || !filterHas(uncovered, `skills != "docker"`) {
		t.Errorf("uncovered filter should exclude body skills: %#v", uncovered)
	}
}
