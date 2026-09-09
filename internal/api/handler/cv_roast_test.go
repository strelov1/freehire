package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/search"
)

// roastCtxFor runs fn with a Fiber context carrying the given query string, since
// roastRoleValues reads the request's params.
func roastCtxFor(t *testing.T, query string, fn func(c *fiber.Ctx)) {
	t.Helper()
	app := fiber.New()
	app.Get("/probe", func(c *fiber.Ctx) error {
		fn(c)
		return nil
	})
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe"+query, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("probe request: %v", err)
	}
	defer resp.Body.Close()
}

func TestRoastRoleValues_UsesTheFirstInferredCategory(t *testing.T) {
	roastCtxFor(t, "", func(c *fiber.Ctx) {
		vals, role := roastRoleValues(c, []string{"backend", "ml"})
		if role != "backend" {
			t.Errorf("role = %q, want backend (the primary category)", role)
		}
		if got := vals["category"]; len(got) != 1 || got[0] != "backend" {
			t.Errorf("category filter = %v, want [backend]", got)
		}
	})
}

func TestRoastRoleValues_AnExplicitCategoryWins(t *testing.T) {
	roastCtxFor(t, "?category=devops", func(c *fiber.Ctx) {
		vals, role := roastRoleValues(c, []string{"backend"})
		if role != "devops" {
			t.Errorf("role = %q, want devops (the caller's override)", role)
		}
		if got := vals["category"]; len(got) != 1 || got[0] != "devops" {
			t.Errorf("category filter = %v, want [devops]", got)
		}
	})
}

func TestRoastRoleValues_NoCategoryResolvedMeansNoFilter(t *testing.T) {
	roastCtxFor(t, "", func(c *fiber.Ctx) {
		vals, role := roastRoleValues(c, nil)
		if role != "" {
			t.Errorf("role = %q, want empty — the dictionary resolved nothing and we do not guess", role)
		}
		if hasNonEmpty(vals["category"]) {
			t.Errorf("category filter = %v, want none", vals["category"])
		}
	})
}

func TestRoastRoleValues_DropsACallerSuppliedSkillsFilter(t *testing.T) {
	roastCtxFor(t, "?skills=rust", func(c *fiber.Ctx) {
		vals, _ := roastRoleValues(c, []string{"backend"})
		if hasNonEmpty(vals["skills"]) {
			t.Errorf("skills = %v, want stripped — the CV's skills are the measured set, not a filter", vals["skills"])
		}
	})
}

// roastApp mounts RoastCV on a bare app with the given facet counter, the way
// coverageApp does for MarketCoverage.
func roastApp(fc facetCounter) *fiber.App {
	h := &resumeHandlers{facets: fc}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/cv/roast", h.RoastCV)
	return app
}

func TestRoastCV_ScoresTextAndReadsTheMarket(t *testing.T) {
	// kubernetes outranks go by count so it is the top gap: rankGaps sorts
	// UncoveredSkills purely by NewVacancies count, with no notion of what the CV
	// declares, so the fixture itself has to carry the "higher-demand, unnamed skill
	// wins" property the assertion below checks for.
	fake := &recordingFacetCounter{res: search.FacetResult{
		Total:  500,
		Facets: map[string]map[string]int64{"skills": {"go": 250, "kubernetes": 300}},
	}}
	app := roastApp(fake)

	// A headline classify resolves ("backend engineer") plus a Skills section, so the
	// role is inferred and the skill sets are real.
	cv := `Jane Doe
Senior Backend Engineer

Skills
Go, PostgreSQL, Docker

Experience
Built services in Go against PostgreSQL for four years, deployed with Docker.`
	body, _ := json.Marshal(map[string]string{"text": cv})

	status, out := doPostJSON(t, app, "/cv/roast", string(body))
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data, ok := out["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %v", out)
	}
	if data["report"] == nil {
		t.Error("report is nil, want the deterministic ATS report")
	}
	if data["role"] != "backend" {
		t.Errorf("role = %v, want backend", data["role"])
	}
	market, ok := data["market"].(map[string]any)
	if !ok {
		t.Fatalf("market is not an object: %v", data["market"])
	}
	// The highest-yield missing skill is the line the page is built around: the role
	// demands kubernetes, this CV does not name it, so it must come back first.
	gaps, _ := market["gaps"].([]any)
	if len(gaps) == 0 {
		t.Fatal("gaps is empty, want the missing skill that unlocks the most vacancies")
	}
	if top, _ := gaps[0].(map[string]any); top["name"] != "kubernetes" {
		t.Errorf("gaps[0].name = %v, want kubernetes", top["name"])
	}
	// The role facet is fetched once and shared: three queries in total (role,
	// uncovered, skill-bearing), not four.
	if len(fake.calls) != 3 {
		t.Errorf("FacetCounts calls = %d, want 3", len(fake.calls))
	}
}

func TestRoastCV_NeverRunsTheModelReview(t *testing.T) {
	fake := &recordingFacetCounter{res: search.FacetResult{Total: 10}}
	app := roastApp(fake)
	body, _ := json.Marshal(map[string]string{"text": "Backend Engineer\n\nSkills\nGo"})

	_, out := doPostJSON(t, app, "/cv/roast", string(body))
	data := out["data"].(map[string]any)
	report := data["report"].(map[string]any)
	if reviewed, _ := report["reviewed"].(bool); reviewed {
		t.Error("report is marked reviewed — the public roast must never call the model")
	}
}

func TestRoastCV_EmptyTextIsRefused(t *testing.T) {
	fake := &recordingFacetCounter{}
	app := roastApp(fake)
	status, _ := doPostJSON(t, app, "/cv/roast", `{"text":"   "}`)
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if len(fake.calls) != 0 {
		t.Errorf("no facet query should run for an empty CV, got %d", len(fake.calls))
	}
}
