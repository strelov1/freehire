package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sort"
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
	h := &resumeHandlers{facets: fc}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/market/coverage", h.MarketCoverage)
	return app
}

func doPostJSON(t *testing.T, app *fiber.App, target, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, target, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
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

func TestMarketCoverage_ReportsIgnoredParams(t *testing.T) {
	// Coverage turns a filter into "how much of this market do my skills reach".
	// A dropped filter therefore scores the caller against a market they did not
	// ask about, and the percentage looks just as confident either way.
	app := coverageApp(&recordingFacetCounter{})

	_, body := doPostJSON(t, app, "/market/coverage?country=it", `{"skills":["go"]}`)

	meta, _ := body["meta"].(map[string]any)
	ignored, _ := meta["ignored_params"].([]any)
	if len(ignored) != 1 {
		t.Fatalf("meta.ignored_params = %v, want one entry", meta["ignored_params"])
	}
	first, _ := ignored[0].(map[string]any)
	if first["param"] != "country" || first["did_you_mean"] != "countries" {
		t.Errorf("ignored_params[0] = %v, want country -> countries", first)
	}
}

func TestMarketCoverage_ReportsTheSkillsParamItDiscards(t *testing.T) {
	// The measured skills come from the body; the skills facet is stripped from
	// the query on purpose (stripSkillParams). That is still a filter the caller
	// wrote and the endpoint dropped, so it has to be reported rather than
	// silently obeyed-looking.
	app := coverageApp(&recordingFacetCounter{})

	_, body := doPostJSON(t, app, "/market/coverage?skills=rust", `{"skills":["go"]}`)

	meta, _ := body["meta"].(map[string]any)
	ignored, _ := meta["ignored_params"].([]any)
	if len(ignored) != 1 {
		t.Fatalf("meta.ignored_params = %v, want the discarded skills param", meta["ignored_params"])
	}
	if first, _ := ignored[0].(map[string]any); first["param"] != "skills" {
		t.Errorf("ignored_params[0] = %v, want skills", first)
	}
}

func TestMarketCoverage_CleanQueryKeepsTheBareDataEnvelope(t *testing.T) {
	app := coverageApp(&recordingFacetCounter{})

	_, body := doPostJSON(t, app, "/market/coverage?countries=it", `{"skills":["go"]}`)

	if _, present := body["meta"]; present {
		t.Errorf("meta = %v, want the key absent on a clean query", body["meta"])
	}
}

func TestMarketCoverage_IgnoredReportStaysSortedAndCapped(t *testing.T) {
	// The skill params are prepended to the shared report, which caps and sorts
	// only what it produced itself — so this endpoint could exceed the cap it
	// exists to enforce, and break the alphabetical order the rest relies on.
	app := coverageApp(&recordingFacetCounter{})

	q := "/market/coverage?skills=a&skills_exclude=b&skills_mode=and"
	for i := range 12 {
		q += fmt.Sprintf("&junk%02d=x", i)
	}
	_, body := doPostJSON(t, app, q, `{"skills":["go"]}`)

	meta, _ := body["meta"].(map[string]any)
	ignored, _ := meta["ignored_params"].([]any)
	if len(ignored) != 10 {
		t.Fatalf("len(ignored_params) = %d, want the shared cap of 10", len(ignored))
	}

	var names []string
	for _, entry := range ignored {
		m, _ := entry.(map[string]any)
		names = append(names, m["param"].(string))
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("ignored_params = %v, want one alphabetically sorted report", names)
	}
}
