package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"slices"
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
	// This does NOT prove kubernetes is the skill missing from the CV: the shared
	// fake returns the same canned facet map for all three queries regardless of
	// filter, so it cannot distinguish "the right role" from any other. What it
	// does prove is that the facet map survives the plumbing into verdict.Compute
	// intact and ordered — rankGaps sorts by raw count, and kubernetes (300) only
	// outranks go (250) here because the fixture says so.
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

func TestRoastCV_SearchDownStillScoresTheCV(t *testing.T) {
	app := roastApp(nil) // facet backend unconfigured
	body, _ := json.Marshal(map[string]string{"text": "Backend Engineer\n\nSkills\nGo, Docker"})

	status, out := doPostJSON(t, app, "/cv/roast", string(body))
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — the ATS half needs no index", status)
	}
	data := out["data"].(map[string]any)
	if data["report"] == nil {
		t.Error("report is nil, want the ATS score")
	}
	if avail, _ := data["market_available"].(bool); avail {
		t.Error("market_available = true with no facet backend")
	}
	if _, present := data["market"]; present {
		t.Error("market is present, want it omitted rather than reported as zero")
	}
}

func TestRoastCV_SearchErrorStillScoresTheCV(t *testing.T) {
	fake := &recordingFacetCounter{err: errors.New("meili down")}
	app := roastApp(fake)
	body, _ := json.Marshal(map[string]string{"text": "Backend Engineer\n\nSkills\nGo"})

	status, out := doPostJSON(t, app, "/cv/roast", string(body))
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data := out["data"].(map[string]any)
	if avail, _ := data["market_available"].(bool); avail {
		t.Error("market_available = true after a failed facet query")
	}
}

func TestRoastCV_UnresolvableRoleMeasuresTheWholeCatalogue(t *testing.T) {
	fake := &recordingFacetCounter{res: search.FacetResult{Total: 900}}
	app := roastApp(fake)
	// A headline no category alias matches.
	body, _ := json.Marshal(map[string]string{"text": "Zebra Wrangler\n\nSkills\nGo"})

	_, out := doPostJSON(t, app, "/cv/roast", string(body))
	data := out["data"].(map[string]any)
	if data["role"] != "" {
		t.Errorf("role = %v, want empty — nothing resolved and we do not guess", data["role"])
	}
	if scoped, _ := data["market_scoped"].(bool); scoped {
		t.Error("market_scoped = true with no resolved role")
	}
	if data["market"] == nil {
		t.Error("market is nil, want the whole-catalogue reading")
	}
}

func TestRoastCV_AnUnreadableCVIsAnAnswerNotAnError(t *testing.T) {
	fake := &recordingFacetCounter{res: search.FacetResult{Total: 100}}
	app := roastApp(fake)
	// Far under atscheck's minReadableWords (30) — what a scanned, image-only CV
	// extracts to. This is the single most valuable thing the page can tell someone,
	// so it must be a 200 carrying a failed item, never a 4xx.
	body, _ := json.Marshal(map[string]string{"text": "Jane Doe"})

	status, out := doPostJSON(t, app, "/cv/roast", string(body))
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	data := out["data"].(map[string]any)
	report := data["report"].(map[string]any)
	if report["overall"] == nil {
		t.Fatal("overall is nil, want a real (low) score")
	}

	// atscheck.Score always returns a non-nil Overall, so "overall is present" alone
	// proves nothing about the number's magnitude — the actual pin is that a two-word
	// CV scores LOW. Verified against atscheck.Score("Jane Doe", nil, nil) directly:
	// every one of the five categories reads for structure, dates, bullets, contact
	// info, or length this text has none of, so today it comes back exactly 0/100.
	// The threshold is set well above that measured floor (not pinned to 0) so the
	// test survives a future reweighting of the categories without losing what it
	// pins: a 2-word CV must still read as a bad score, not a mediocre or good one.
	overall, _ := report["overall"].(float64)
	if overall > 15 {
		t.Errorf("overall = %v, want a low score for a 2-word CV", overall)
	}

	// The machine-readability check is format_compliance's own item for the emptiness
	// signal atscheck.go documents as "the single biggest ATS killer" (see its package
	// doc). It is the ONLY item in that category whose failStatus is StatusFail — every
	// other item there fails to StatusWarn — so locating the category's first item and
	// asserting its status is "fail" pins the exact regression the reviewer named: a
	// change that stopped marking this item failed for an unreadable CV. (Its Text
	// switches between the pass copy "Text is machine-readable" and a fail-specific
	// remediation string, so status — not text — is what a test can rely on.)
	categories, _ := report["categories"].([]any)
	var formatItems []any
	for _, c := range categories {
		cat, _ := c.(map[string]any)
		if cat["id"] == "format_compliance" {
			formatItems, _ = cat["items"].([]any)
		}
	}
	if len(formatItems) == 0 {
		t.Fatal("format_compliance category has no items")
	}
	machineReadable, _ := formatItems[0].(map[string]any)
	if machineReadable["status"] != "fail" {
		t.Errorf("format_compliance's machine-readability item status = %v, want fail", machineReadable["status"])
	}
}

// TestRoastCV_RealPDFFromAnAnonymousCaller is the spec's headline scenario driven end to
// end: a genuine PDF, no session, a real score. The package already ships the fixture and
// TestExtractResumeProfile_PDF already proves pdftotext is present in this environment.
func TestRoastCV_RealPDFFromAnAnonymousCaller(t *testing.T) {
	pdf, err := os.ReadFile("testdata/resume_sample.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	fake := &recordingFacetCounter{res: search.FacetResult{
		Total:  500,
		Facets: map[string]map[string]int64{"skills": {"go": 300}},
	}}
	app := roastApp(fake)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "resume.pdf")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := part.Write(pdf); err != nil {
		t.Fatalf("write part: %v", err)
	}
	_ = mw.Close()

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/cv/roast", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := out["data"].(map[string]any)
	if !ok || data["report"] == nil {
		t.Fatalf("no report in response: %v", out)
	}
}

func TestRoastCV_UndecodablePDFIsA400NotA500(t *testing.T) {
	fake := &recordingFacetCounter{res: search.FacetResult{Total: 100}}
	app := roastApp(fake)

	// Bytes that are not a PDF, posted the multipart way the browser posts a file.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "cv.pdf")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := part.Write([]byte("this is not a PDF")); err != nil {
		t.Fatalf("write part: %v", err)
	}
	_ = mw.Close()

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/cv/roast", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400 — bad client input, not a server fault", resp.StatusCode)
	}
}

// TestRoastCV_TouchesNoStore reads RoastCV's own body, in this file, and fails if it
// contains a direct h.<field> selector naming one of resumeHandlers' persisting
// collaborators. "Stores nothing" is a promise made to an anonymous visitor about their
// CV, and it is exactly the kind of promise a later well-meaning edit breaks — adding a
// "just cache the report" line looks harmless and is not. A behavioural test cannot see
// this: the fields are nil in every unit test, so a handler that used them would simply
// panic in prod and pass here.
//
// Scope, stated plainly: this walks only RoastCV's own body in cv_roast.go. It does NOT
// see a write introduced inside a helper RoastCV calls — roleFacet, coverageWithRole — or
// inside a shared response helper like dataResponseWithIgnored or coverageIgnoredParams.
// coverageWithRole in particular is shared code with callers outside this feature, so a
// guard that followed calls across files would be brittle and could fail for reasons that
// have nothing to do with this endpoint. Extending the scope was considered and rejected
// for that reason; this guard's job is only to catch a write added directly to RoastCV.
func TestRoastCV_TouchesNoStore(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "cv_roast.go", nil, 0)
	if err != nil {
		t.Fatalf("parse cv_roast.go: %v", err)
	}
	forbidden := []string{"resume", "atsCache", "atsAnalyzer", "bank", "structuredExtractor", "llm"}

	var body *ast.BlockStmt
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "RoastCV" {
			body = fn.Body
		}
	}
	if body == nil {
		t.Fatal("RoastCV not found in cv_roast.go — this guard would pass on anything")
	}
	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "h" && slices.Contains(forbidden, sel.Sel.Name) {
			t.Errorf("RoastCV reaches h.%s — the public roast must store nothing and call no model", sel.Sel.Name)
		}
		return true
	})
}
