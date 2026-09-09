# Public CV Roast Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps
> use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give an anonymous visitor a deterministic ATS score and a live market-coverage
reading for an uploaded CV, at a public URL, with nothing stored and no model called.

**Architecture:** One new handler on the existing `resumeHandlers`, composed entirely from
functions that are already account-free: `readResumeUpload` → `resumeProfile` →
`cvsection.Parse` → one shared `FacetCounts` → `atscheck.Score` + `coverageWithRole`. The
only shared code that moves is `coverageFor`, which is split so its caller can supply the
role facet it would otherwise fetch a second time.

**Tech Stack:** Go + Fiber v2, Meilisearch (via `facetCounter`), SvelteKit (`web/`).

## Global Constraints

- **English only** in all code, comments, identifiers and commit messages.
- **Response shapes:** single items answer `{"data": ...}` via `dataResponseWithIgnored`.
- **`gofmt -w` the touched Go paths, then `go vet ./...` and `go test ./...` before every
  commit.** `gofmt -l .` must print nothing.
- **`go vet -tags=integration ./...` before pushing** — `go test ./...` compiles no
  `//go:build integration` file and `internal/api/handler` holds 78 of them.
- **Nothing about this feature may write** to S3, Postgres, or a cache.
- **The LLM analyzer is never invoked on this path.**
- Layering: `internal/api/handler` is layer 8 and already imports `candidate`, `search` and
  `dict`. No new block edge, so `internal/platform/arch/layering/blocks.go` is untouched.

---

### Task 1: Let a caller supply the role facet instead of refetching it

`coverageFor` opens with `FacetCounts({Filter: roleFilter, Facets: ["skills"]})`, and
`deterministicReport` issues the identical query for its `roleTopSkills`. A handler that
wants both readings would run it twice and could, on a slow index, get two different
answers for the same question. Split the fetch off so one result feeds both.

Pure refactor: no behaviour change, and `coverageFor` keeps its signature and its callers.

**Files:**
- Modify: `internal/api/handler/resume_verdict.go:61-94`
- Test: `internal/api/handler/market_coverage_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `func (h *resumeHandlers) coverageWithRole(ctx context.Context, roleFilter any,
  role search.FacetResult, coverageSkills, declared, body, all []string) (verdict.Verdict,
  error)` — Task 3 calls it.

- [ ] **Step 1: Write the failing test**

In `internal/api/handler/market_coverage_test.go`, append:

```go
// TestCoverageWithRole_DoesNotRefetchTheRoleFacet pins the reason the split exists:
// the caller has already asked for the role's skill distribution, and asking again
// both costs a query and lets the two readings disagree about the same role.
func TestCoverageWithRole_DoesNotRefetchTheRoleFacet(t *testing.T) {
	fake := &recordingFacetCounter{res: search.FacetResult{
		Total:  500,
		Facets: map[string]map[string]int64{"skills": {"go": 300}},
	}}
	h := &resumeHandlers{facets: fake}

	role := search.FacetResult{
		Total:  500,
		Facets: map[string]map[string]int64{"skills": {"go": 300, "kubernetes": 250}},
	}
	v, err := h.coverageWithRole(context.Background(), nil, role, []string{"go"}, []string{"go"}, nil, []string{"go"})
	if err != nil {
		t.Fatalf("coverageWithRole: %v", err)
	}
	// Two queries, not three: the uncovered set and the skill-bearing total. The role
	// facet came from the argument.
	if len(fake.calls) != 2 {
		t.Errorf("FacetCounts calls = %d, want 2 (role facet supplied by the caller)", len(fake.calls))
	}
	// The supplied role result is what was scored, not the counter's canned one: the
	// counter knows nothing of kubernetes, the argument does.
	if v.Total != 500 {
		t.Errorf("Total = %d, want 500", v.Total)
	}
	if len(v.Skills) < 2 {
		t.Errorf("Skills = %d rows, want the supplied role distribution's 2", len(v.Skills))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handler/ -run TestCoverageWithRole -v`
Expected: FAIL — `h.coverageWithRole undefined`.

- [ ] **Step 3: Split the fetch off**

In `internal/api/handler/resume_verdict.go`, replace the body of `coverageFor` and add the
new method. Keep `coverageFor`'s existing doc comment where it is and add the note about
the split:

```go
// (existing coverageFor doc comment stays, with this paragraph appended)
//
// The role query (A) is split into coverageWithRole so a caller that has already asked
// for the role's skill distribution — the public roast, which needs it for the ATS
// keyword score too — can supply it instead of asking twice. Two answers to the same
// question can differ while the index is being written; one answer cannot.
func (h *resumeHandlers) coverageFor(ctx context.Context, roleFilter any, coverageSkills, declared, body, all []string) (verdict.Verdict, error) {
	role, err := h.facets.FacetCounts(ctx, search.FacetParams{
		Filter: roleFilter,
		Facets: []string{"skills"},
	})
	if err != nil {
		return verdict.Verdict{}, err
	}
	return h.coverageWithRole(ctx, roleFilter, role, coverageSkills, declared, body, all)
}

// coverageWithRole is coverageFor with query A already answered: it issues the uncovered
// (B) and skill-bearing-total (C) queries and computes the verdict.
func (h *resumeHandlers) coverageWithRole(ctx context.Context, roleFilter any, role search.FacetResult, coverageSkills, declared, body, all []string) (verdict.Verdict, error) {
	uncovered, err := h.facets.FacetCounts(ctx, search.FacetParams{
		Filter: search.AndNotSkills(roleFilter, coverageSkills),
		Facets: []string{"skills"},
	})
	if err != nil {
		return verdict.Verdict{}, err
	}
	// Skill-bearing total: the role's vacancies that list at least one tagged skill.
	// Skill frequency (and the must-have flag) is measured against this, not the raw
	// role total, so postings the tagger left skill-less don't deflate frequencies.
	skilled, err := h.facets.FacetCounts(ctx, search.FacetParams{
		Filter: search.AndSkillsPresent(roleFilter),
	})
	if err != nil {
		return verdict.Verdict{}, err
	}
	return verdict.Compute(verdict.Input{
		Total:           role.Total,
		SkilledTotal:    skilled.Total,
		UncoveredTotal:  uncovered.Total,
		UncoveredSkills: uncovered.Facets["skills"],
		RoleSkills:      role.Facets["skills"],
		Declared:        declared,
		Body:            body,
		All:             all,
	}), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/api/handler/ -run 'TestCoverageWithRole|TestMarketCoverage' -v`
Expected: PASS — including every pre-existing `TestMarketCoverage_*`, which is what proves
the refactor changed no behaviour.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/api/handler/resume_verdict.go internal/api/handler/market_coverage_test.go
go vet ./... && go test ./internal/api/handler/
git add internal/api/handler/resume_verdict.go internal/api/handler/market_coverage_test.go
git commit -m "refactor(handler): let a coverage caller supply the role facet"
```

---

### Task 2: Infer the role from the CV, and say when nothing resolved

The market filter comes from `classify.Categories` over the CV's headline, which
`resumeProfile` already computes. The rule has three cases and one refusal: an explicit
`category` query param wins; otherwise the first inferred category; otherwise no filter at
all, reported as such. It never picks a plausible role — a coverage number attributed to
the wrong role reads exactly like a correct one.

The resolved seniority is deliberately not folded in: a junior's coverage against
junior-only postings is a smaller number that says nothing about their skills, and skills
are what this page reads.

**Files:**
- Create: `internal/api/handler/cv_roast.go`
- Create: `internal/api/handler/cv_roast_test.go`

**Interfaces:**
- Consumes: `queryValues(c) url.Values`, `stripSkillParams(url.Values)`,
  `hasNonEmpty([]string) bool` — all existing in `resume_verdict.go`.
- Produces: `func roastRoleValues(c *fiber.Ctx, categories []string) (vals url.Values,
  role string)` — Task 3 calls it. `role` is `""` when nothing resolved.

- [ ] **Step 1: Write the failing test**

Create `internal/api/handler/cv_roast_test.go`:

```go
package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
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
	if _, err := app.Test(req); err != nil {
		t.Fatalf("probe request: %v", err)
	}
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handler/ -run TestRoastRoleValues -v`
Expected: FAIL — `roastRoleValues undefined`.

- [ ] **Step 3: Write the helper**

Create `internal/api/handler/cv_roast.go`:

```go
package handler

import (
	"net/url"

	"github.com/gofiber/fiber/v2"
)

// roastRoleValues builds the market filter for a public roast and names the role it
// selected. It mirrors roleValues (same skills strip, same category default) with one
// difference that matters: roleValues falls back to the caller's stored profile, and an
// anonymous caller has none, so the fallback is the CV's own inferred categories.
//
// Three outcomes, and the third is a refusal rather than a guess: an explicit `category`
// param wins; otherwise the first inferred category (classify returns them in precedence
// order, primary first); otherwise NO role filter and an empty role name. classify never
// guesses either — it returns nothing when its dictionary resolves nothing — and a
// coverage figure attributed to the wrong role reads exactly like a correct one, so the
// page is told the truth and says "the whole catalogue" instead.
//
// The seniority resolveProfile also derives is deliberately not folded in: filtering a
// junior's coverage to junior-only postings yields a smaller number that says nothing
// about their skills, and skills are the reading this page sells.
func roastRoleValues(c *fiber.Ctx, categories []string) (url.Values, string) {
	vals := queryValues(c)
	stripSkillParams(vals)
	if hasNonEmpty(vals["category"]) {
		return vals, vals["category"][0]
	}
	if len(categories) > 0 {
		vals["category"] = []string{categories[0]}
		return vals, categories[0]
	}
	delete(vals, "category")
	return vals, ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handler/ -run TestRoastRoleValues -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/api/handler/cv_roast.go internal/api/handler/cv_roast_test.go
go vet ./... && go test ./internal/api/handler/
git add internal/api/handler/cv_roast.go internal/api/handler/cv_roast_test.go
git commit -m "feat(handler): infer the roast's market role from the CV itself"
```

---

### Task 3: The handler — one upload, two readings, nothing stored

**Files:**
- Modify: `internal/api/handler/cv_roast.go`
- Modify: `internal/api/handler/cv_roast_test.go`

**Interfaces:**
- Consumes: `roastRoleValues` (Task 2), `coverageWithRole` (Task 1), and the existing
  `readResumeUpload(c) (resumeUpload, error)`, `resumeProfile(text) cvProfile`,
  `cvsection.Parse(text) (declared, body, all []string)`, `topRoleSkills(map[string]int64,
  int) []string`, `atsRoleTopN`, `atscheck.Score`, `search.FilterFromValues`,
  `dataResponseWithIgnored`.
- Produces: `func (h *resumeHandlers) RoastCV(c *fiber.Ctx) error` — Task 5 registers it,
  and the `roastResponse` shape Task 6 renders.

- [ ] **Step 1: Write the failing test**

Append to `internal/api/handler/cv_roast_test.go` (add `"encoding/json"`,
`"github.com/strelov1/freehire/internal/search/search"` to its imports):

```go
// roastApp mounts RoastCV on a bare app with the given facet counter, the way
// coverageApp does for MarketCoverage.
func roastApp(fc facetCounter) *fiber.App {
	h := &resumeHandlers{facets: fc}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/cv/roast", h.RoastCV)
	return app
}

func TestRoastCV_ScoresTextAndReadsTheMarket(t *testing.T) {
	fake := &recordingFacetCounter{res: search.FacetResult{
		Total:  500,
		Facets: map[string]map[string]int64{"skills": {"go": 300, "kubernetes": 250}},
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handler/ -run TestRoastCV -v`
Expected: FAIL — `h.RoastCV undefined`.

- [ ] **Step 3: Write the handler**

Append to `internal/api/handler/cv_roast.go` (add the imports it needs: `"strings"`,
`"github.com/strelov1/freehire/internal/candidate/atscheck"`,
`"github.com/strelov1/freehire/internal/candidate/cvsection"`,
`"github.com/strelov1/freehire/internal/job/verdict"`,
`"github.com/strelov1/freehire/internal/search/search"`):

```go
// roastResponse is the public roast's wire shape. Report is the deterministic ATS
// score — never model-refined on this path. Role names the category the market was
// measured against, empty when the dictionary resolved none; MarketScoped says which
// of those two happened, so the page never has to infer it from an empty string.
// Market is nil when the facet backend is unavailable (see MarketAvailable).
type roastResponse struct {
	Report          *atscheck.Report `json:"report"`
	Role            string           `json:"role"`
	MarketScoped    bool             `json:"market_scoped"`
	MarketAvailable bool             `json:"market_available"`
	Market          *verdict.Verdict `json:"market,omitempty"`
}

// RoastCV scores an uploaded CV for an ANONYMOUS caller: the deterministic ATS report
// plus a live market-coverage reading, with no account, no stored bytes and no model
// call. It accepts the same two body forms as ExtractResumeProfile (multipart PDF or
// {text} JSON) through the same reader.
//
// Nothing here writes. ExtractResumeProfile stores the résumé because its signed-in
// caller's later steps need the file; an anonymous visitor has no later step, so the
// bytes die with the request. The PII masking layer is likewise absent by construction
// rather than by omission — it exists to protect a CV from a MODEL, and no model runs.
//
// Withholding the model review is also the page's whole offer: the deterministic score
// names what is wrong, and signing in is what rewrites it.
//
// Public on purpose and rate-limited by IP at the route (see resume.go's register).
func (h *resumeHandlers) RoastCV(c *fiber.Ctx) error {
	up, err := readResumeUpload(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(up.Text) == "" {
		return fiber.NewError(fiber.StatusBadRequest, errResumeNoText)
	}

	profile := resumeProfile(up.Text)
	vals, role := roastRoleValues(c, profile.Categories)
	roleFilter := search.FilterFromValues(vals)

	// One role query feeds both readings, so the ATS keyword score and the coverage
	// figure can never disagree about what this role demands.
	roleFacet, ferr := h.roleFacet(c, roleFilter)

	report := atscheck.Score(up.Text, profile.Skills, topRoleSkills(roleFacet.Facets["skills"], atsRoleTopN))
	out := roastResponse{
		Report:       &report,
		Role:         role,
		MarketScoped: role != "",
	}

	// Search being down costs the market half, not the answer: the ATS score needs no
	// index at all, and a 503 would throw away the reading the visitor came for.
	// MarketCoverage answers 503 in the same situation, which is right for an API
	// client and wrong for a landing page.
	if ferr == nil {
		declared, body, all := cvsection.Parse(up.Text)
		v, err := h.coverageWithRole(c.Context(), roleFilter, roleFacet, profile.Skills, declared, body, all)
		if err == nil {
			out.MarketAvailable = true
			out.Market = &v
		}
	}
	return dataResponseWithIgnored(c, out, coverageIgnoredParams(c))
}

// roleFacet answers the role's skill distribution, or a zero result and an error when
// the facet backend is unconfigured or unreachable. Both are non-fatal here.
func (h *resumeHandlers) roleFacet(c *fiber.Ctx, roleFilter any) (search.FacetResult, error) {
	if h.facets == nil {
		return search.FacetResult{}, errSearchUnavailable
	}
	return h.facets.FacetCounts(c.Context(), search.FacetParams{
		Filter: roleFilter,
		Facets: []string{"skills"},
	})
}

// errSearchUnavailable stands for "no facet backend" so roleFacet has one error kind
// for both of its failures and the caller needs no nil check of its own.
var errSearchUnavailable = errors.New("search is not available")
```

Add `"errors"` to the import block.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handler/ -run TestRoastCV -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/api/handler/cv_roast.go internal/api/handler/cv_roast_test.go
go vet ./... && go test ./internal/api/handler/
git add internal/api/handler/cv_roast.go internal/api/handler/cv_roast_test.go
git commit -m "feat(handler): score an anonymous CV against ATS and the live market"
```

---

### Task 4: The degradations are the feature, so test them

Three situations the spec calls out. Each returns 200 with a real answer, and each is the
kind of thing that quietly turns into a 500 or a zero if nobody pins it.

**Files:**
- Modify: `internal/api/handler/cv_roast_test.go`

**Interfaces:**
- Consumes: `roastApp`, `doPostJSON`, `recordingFacetCounter`, `roastResponse`'s JSON
  field names — all from Tasks 1–3.
- Produces: nothing.

- [ ] **Step 1: Write the failing tests**

Append to `internal/api/handler/cv_roast_test.go` (add `"errors"` to its imports):

```go
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
		t.Error("overall is nil, want a real (low) score")
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
	mw.Close()

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
	mw.Close()

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

// TestRoastCV_TouchesNoStore reads the handler's own source and fails if it reaches for
// any of resumeHandlers' persisting collaborators. "Stores nothing" is a promise made to
// an anonymous visitor about their CV, and it is exactly the kind of promise a later
// well-meaning edit breaks — adding a "just cache the report" line looks harmless and is
// not. A behavioural test cannot see this: the fields are nil in every unit test, so a
// handler that used them would simply panic in prod and pass here.
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
```

Add `"bytes"`, `"mime/multipart"`, `"os"`, `"go/ast"`, `"go/parser"`, `"go/token"` and
`"slices"` to the test file's imports.

- [ ] **Step 2: Run tests to verify they fail or pass**

Run: `go test ./internal/api/handler/ -run TestRoastCV -v`
Expected: the four new tests reveal whether Task 3's degradations are real. Any failure is
a defect in `RoastCV`, not in the test — fix `cv_roast.go` until all pass. In particular,
`TestRoastCV_AnUnreadableCVIsAnAnswerNotAnError` fails if the empty-text guard is written
as a word-count threshold rather than a `TrimSpace`-empty check.

- [ ] **Step 3: Run the whole package**

Run: `go test ./internal/api/handler/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
gofmt -w internal/api/handler/cv_roast_test.go
git add internal/api/handler/cv_roast_test.go
git commit -m "test(handler): pin the roast's degradations to 200s with real answers"
```

---

### Task 5: Register the route, bound it by IP, and record it

The limiter is built inline from `mw.throttler`, the way `tracerLimiter` is at
`handler.go:741` — deliberately NOT one of `public_read_limit.go`'s constructors. That
file's own comment draws the boundary: it holds the *public read* budgets, and "every
other limiter here guards a write, an auth route or an LLM spend, and is keyed by its own
rules." This is a POST that runs a subprocess, so it is one of the others. Adding it to
`publicReadLimiterFuncs` would drag `resumeHandlers` into a guard about read budgets it
does not share.

**Files:**
- Modify: `internal/api/handler/cv_roast.go` (the limiter and its constant)
- Modify: `internal/api/handler/resume.go:103-106` (the register block)
- Modify: `internal/api/handler/AGENTS.md`
- Test: `internal/api/handler/cv_roast_test.go`

**Interfaces:**
- Consumes: `RoastCV` (Task 3), `middleware.throttler`, `ratelimit.Middleware`,
  `ratelimit.KeyByIP`.
- Produces: the mounted route `POST /api/v1/cv/roast`, which Task 6 calls.

- [ ] **Step 1: Write the failing test**

Append to `internal/api/handler/cv_roast_test.go`:

```go
// TestRoastCV_IsMountedPublicAndLimited drives the real register, not a bare app: the
// route being public is the point, and a limiter that is correct in isolation and
// unmounted in place is exactly the defect public_read_limit_test.go exists for.
func TestRoastCV_IsMountedPublicAndLimited(t *testing.T) {
	h := &resumeHandlers{facets: &recordingFacetCounter{res: search.FacetResult{Total: 10}}}
	app := fiber.New(fiber.Config{
		ErrorHandler: RenderError,
		ProxyHeader:  fiber.HeaderXForwardedFor,
	})
	refuse := func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusUnauthorized, "auth middleware ran")
	}
	h.register(app.Group("/api/v1"), middleware{
		cookie:    refuse,
		key:       refuse,
		throttler: newOneShotThrottler(),
	})

	post := func() int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost,
			"/api/v1/cv/roast", strings.NewReader(`{"text":"Backend Engineer\n\nSkills\nGo"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(fiber.HeaderXForwardedFor, "203.0.113.7")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if got := post(); got != fiber.StatusOK {
		t.Fatalf("first call = %d, want 200 — the route must need no session and no key", got)
	}
	if got := post(); got != fiber.StatusTooManyRequests {
		t.Errorf("second call = %d, want 429 — the limiter is not mounted", got)
	}
}
```

Add `"strings"` to the test file's imports. `newOneShotThrottler` already exists in
`public_read_limit_test.go`, same package.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handler/ -run TestRoastCV_IsMountedPublicAndLimited -v`
Expected: FAIL with 404 — the route is not registered.

- [ ] **Step 3: Add the limiter and mount the route**

In `internal/api/handler/cv_roast.go`, add:

```go
// cvRoastPerHour bounds the public roast per IP. The work is a pdftotext subprocess
// plus three facet queries — not free, and not a model call either, so this is sized to
// stop a scraper rather than to ration something scarce.
//
// Per HOUR rather than per minute, on purpose: someone fixing their CV genuinely
// re-uploads it several times in a row, and that is the behaviour the page wants. A
// per-minute ceiling would punish exactly the visitor who is getting value.
const cvRoastPerHour = 10

// cvRoastLimiter bounds the public roast by source address. There is no authenticated
// caller to key by — that is what public means — and the route mounts no auth gate, so
// KeyByIP is not a fallback here, it is the only thing there is.
func cvRoastLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByIP("cvroast"), cvRoastPerHour, time.Hour)
}
```

Add `"time"` and `"github.com/strelov1/freehire/internal/api/ratelimit"` to the imports.

In `internal/api/handler/resume.go`, inside `register`, after the `/market/coverage` line:

```go
	// The public CV roast: no session, no API key. It is the landing page for search
	// traffic that has never heard of us, so an account gate here would spend the visit
	// to gain nothing — the same reason /jobs/find is public and first. Stores nothing
	// and calls no model; the model review and tailoring are what the account is for.
	// IP-limited, since there is no caller to key by.
	api.Post("/cv/roast", cvRoastLimiter(mw.throttler), h.RoastCV)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handler/ -run TestRoastCV -v`
Expected: PASS (8 tests).

- [ ] **Step 5: Record the route in the block's AGENTS.md**

In `internal/api/handler/AGENTS.md`, add to the résumé/CV section:

```markdown
- `POST /cv/roast` is public — no cookie, no key — and is the only unauthenticated route
  that accepts a CV. It stores nothing (unlike `/me/resume/extract`, which stores because
  its signed-in caller's later steps need the file) and never calls the LLM analyzer, so
  `internal/candidate/pii` is not on its path: that layer protects a CV from a model, and
  no model runs. Its limiter is built inline from `mw.throttler` rather than in
  `public_read_limit.go` — that file holds the public READ budgets, and this is a POST
  that forks `pdftotext`.
```

- [ ] **Step 6: Full check and commit**

```bash
gofmt -w internal/api/handler/cv_roast.go internal/api/handler/resume.go internal/api/handler/cv_roast_test.go
go vet ./... && go test ./... && go vet -tags=integration ./...
git add internal/api/handler/ && git commit -m "feat(api): expose the public CV roast at POST /cv/roast"
```

---

### Task 6: The public page

**Files:**
- Create: `web/src/routes/roast/+page.svelte`
- Modify: `web/src/lib/api.ts` (the client call)
- Test: `web/src/routes/roast/page.test.ts`

**Interfaces:**
- Consumes: `POST /api/v1/cv/roast` and the `roastResponse` shape from Task 3 —
  `{report, role, market_scoped, market_available, market}`.
- Produces: the `/roast` route the articles will link to.

- [ ] **Step 1: Read the existing surfaces before writing anything**

Run:
```bash
sed -n '1,80p' web/src/routes/my/profile/cv-readiness/+page@my.svelte
grep -n "atsReport\|ats-report\|marketCoverage" web/src/lib/api.ts
```
The signed-in ATS page already renders `atscheck.Report`'s categories and line items. Reuse
its components rather than writing a second renderer — `LineItem`/`Status` is the shared
wire shape `cmd/gen-contracts` excludes cvmatch's `lineitem.go` for, precisely so one
component serves both. Note the component paths before continuing.

- [ ] **Step 2: Write the failing test**

Create `web/src/routes/roast/page.test.ts` asserting the three things the spec requires of
the page, against a mocked fetch:

```ts
import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

describe('/roast', () => {
  it('renders the drop zone with no session', () => {
    render(Page, { data: {} });
    expect(screen.getByTestId('roast-dropzone')).toBeInTheDocument();
  });

  it('names the role it measured against', async () => {
    render(Page, { data: {} });
    // Drive the component's result state directly through its exported setter or by
    // dispatching the upload; assert the role label renders and carries a change control.
    expect(await screen.findByTestId('roast-role')).toBeInTheDocument();
    expect(await screen.findByTestId('roast-role-change')).toBeInTheDocument();
  });

  it('says so when no role resolved', async () => {
    render(Page, { data: {} });
    expect(await screen.findByTestId('roast-role-unscoped')).toBeInTheDocument();
  });
});
```

Adjust the render/drive mechanics to match the harness the neighbouring `web/` tests use —
read one first (`ls web/src/routes/**/*.test.ts | head`) rather than inventing a pattern.

- [ ] **Step 3: Run test to verify it fails**

Run: `pnpm --dir web test -- roast`
Expected: FAIL — the route does not exist.

- [ ] **Step 4: Build the page**

`web/src/routes/roast/+page.svelte`, public (no `@my` layout), rendering in this order:

1. A drop zone (`data-testid="roast-dropzone"`) taking a PDF, posting `multipart/form-data`
   field `file` to `/api/v1/cv/roast`.
2. The overall score and the five categories, through the existing report components.
3. The role line: `data-testid="roast-role"` naming `role` with a `roast-role-change`
   control that re-posts with `?category=<picked>`, or `data-testid="roast-role-unscoped"`
   saying the reading covers the whole catalogue when `market_scoped` is false.
4. The market line when `market_available`: `market.covered` of `market.total`, and
   `market.gaps[0]` as the single highest-yield missing skill. Render nothing but a short
   note when `market_available` is false — never a zero, which reads as a measurement.
5. The CTA into sign-in for the model review and tailoring.

Constraints from this repo's own traps, all of which have bitten before:
- Any `class` prop goes through `cn`, never string interpolation.
- No `Object.hasOwn` / `toSorted` — the build target does not polyfill them and they kill
  the whole module in Safari.
- No `replaceState` in `onMount` — use `onRouterReady`.
- Do not touch `localStorage` at module scope.
- An `{#each}` key must be a stable value, never a composed signature.

- [ ] **Step 5: Run tests and lint**

Run:
```bash
pnpm --dir web test -- roast
pnpm --dir web lint
pnpm --dir web check
```
Expected: PASS on all three. `pnpm --dir web lint` is the one to run rather than relying on
the pre-commit hook — CI additionally runs oxlint.

- [ ] **Step 6: See it work**

Run `make up`, open `/roast` with no session, upload a real PDF CV, and confirm: a score
renders, the role is named, the market line names a gap skill. Then stop Meilisearch and
confirm the score still renders with the market note instead of an error.

- [ ] **Step 7: Commit**

```bash
git add web/src/routes/roast/ web/src/lib/api.ts
git commit -m "feat(web): add the public /roast page"
```

---

## Done means

- `go test ./...` and `go test -tags=integration ./internal/api/handler/` pass.
- `POST /api/v1/cv/roast` answers 200 with no cookie and no key, and 429 past ten calls
  from one IP in an hour.
- Nothing in the change writes to S3, Postgres or a cache, and `atscheck.Analyzer` is not
  constructed or called on this path.
- `/roast` renders a score, a named role with an override, and a market reading — and still
  renders the score with the market backend down.

## Deliberately not in this plan

- **The articles.** They are the reason this page exists and they are a content task; the
  page has to exist first.
- **`web/static/openapi.yaml`.** It does not document `/market/coverage` either; adding one
  public CV endpoint and not its sibling would make that file look complete when it is not.
- **Result caching.** A cache keyed on anonymous CV content is a store of CVs by another
  name, which this design refuses.
