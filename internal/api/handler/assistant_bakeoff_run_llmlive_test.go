//go:build integration && llmlive

// The model bake-off itself: one tailoring autopilot run per (candidate model, case),
// against a real gateway, scored by the product's own deterministic scores.
//
// It spends real money, so it is behind llmlive as well as integration and never runs in
// CI. It needs Docker (testcontainers), a Typst binary and pdftotext for the scores, the
// local profile fixture, and the gateway credentials:
//
//	BAKEOFF_MODELS=vendor/model-a,vendor/model-b \
//	LLM_BASE_URL=… LLM_API_KEY=… LLM_MODEL=… \
//	  go test -tags=integration,llmlive ./internal/api/handler/ \
//	    -run TestBakeoff -timeout 3h -v
//
// LLM_MODEL is the FIT model and stays pinned across every candidate — see the pin below.
// BAKEOFF_MODELS names the turn models being compared, and nothing else varies.
//
// The report lands in .cache/ (not in git) and carries every run's tailored CV, because
// whether a CV is any good is a judgement no number here makes. See
// openspec/changes/assistant-model-bakeoff/design.md.
package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvmatch"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/modroot"
)

// bakeoffModelsEnv names the turn models to compare, comma-separated, as the gateway's own
// catalogue spells them — the same ids the price table is keyed by.
const bakeoffModelsEnv = "BAKEOFF_MODELS"

// bakeoffRunTimeout bounds ONE autopilot run's HTTP read.
//
// An autopilot turn may take thirty rounds and the gateway behind this is bimodal: the same
// chain lands in 28s on one provider and 90s per stage on another. A deadline tight enough
// to be interesting would therefore report the gateway's mood as the model's failure, which
// is the one thing this measurement must not do. It is a runaway guard, not a bound.
const bakeoffRunTimeout = 25 * time.Minute

// countingModel counts what a turn actually cost.
//
// The stream cannot answer this: emitUsage fires on the runner's two terminal paths only, so
// a thirty-round run reports the LAST call's tokens and says nothing about the other
// twenty-nine (see bakeoffTally). The bake-off already substitutes the turn model, so it
// counts at that seam — a measurement problem solved where it costs nothing rather than by
// changing what every assistant turn in production streams.
//
// No lock: the runner drives one model call at a time within a turn, and a bake-off run is
// one turn.
type countingModel struct {
	inner assistant.Model
	tally *bakeoffTally
}

func (m *countingModel) Chat(ctx context.Context, msgs []llms.MessageContent, tools []llms.Tool, stream llm.ChatStream) (*llms.ContentChoice, error) {
	started := time.Now()
	choice, err := m.inner.Chat(ctx, msgs, tools, stream)
	// A failed call is a round that happened and was billed for whatever it read before it
	// died. Counting it with no usage records it as unmeasured — which is what it is — and
	// keeps the round count honest, so a model that fails on round nine is not reported as
	// having taken eight.
	var usage *llm.Usage
	if choice != nil {
		usage = llm.UsageFrom(choice)
	}
	m.tally.observeCall(usage, time.Since(started))
	return choice, err
}

// bakeoffGateway builds one turn client against the configured gateway, for the named model.
// *llm.Client is what assistant.Model wants, so this is the value the runner drives.
func bakeoffGateway(t *testing.T, model string) (*llm.Client, func()) {
	t.Helper()
	s := bakeoffSettings(t, model)
	c, flush, err := llm.NewClient(s, "bakeoff")
	if err != nil {
		t.Fatalf("gateway client for %q: %v", model, err)
	}
	return c, flush
}

// bakeoffSettings reads the gateway credentials, skipping when the run has none rather than
// failing: a machine without them is not a broken bake-off, it is one nobody asked for.
func bakeoffSettings(t *testing.T, model string) llm.Settings {
	t.Helper()
	s := llm.Settings{
		BaseURL: os.Getenv("LLM_BASE_URL"),
		APIKey:  os.Getenv("LLM_API_KEY"),
		Model:   model,
	}
	if !s.Enabled() {
		t.Skip("LLM_BASE_URL/LLM_API_KEY/LLM_MODEL not set")
	}
	return s
}

// bakeoffCandidates reads the models to compare. Absent, the bake-off skips rather than
// inventing a list: which models are worth a paid run is a decision, not a default.
func bakeoffCandidates(t *testing.T) []string {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(bakeoffModelsEnv))
	if raw == "" {
		t.Skipf("%s not set; name the turn models to compare, comma-separated", bakeoffModelsEnv)
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range strings.Split(raw, ",") {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		// A model listed twice would be two rows the report cannot tell apart, and the
		// second would be read as a second opinion rather than a duplicate.
		if seen[m] {
			t.Fatalf("%s names %q twice", bakeoffModelsEnv, m)
		}
		seen[m] = true
		out = append(out, m)
	}
	if len(out) == 0 {
		t.Fatalf("%s is set but names no model", bakeoffModelsEnv)
	}
	return out
}

// bakeoffToolchain resolves the renderer and text extractor the scores read, failing rather
// than skipping: a bake-off that spends money and reports no score is worse than one that
// refuses to start.
func bakeoffToolchain(t *testing.T) (cv.Renderer, func([]byte) (string, error)) {
	t.Helper()
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Fatal("typst is not installed; the scores read the RENDERED CV and cannot be computed without it")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Fatal("pdftotext is not installed; the scores read the rendered CV's text layer")
	}
	return cv.NewTypstRenderer(bin), resume.ExtractPDFText
}

// runBakeoffAutopilot drives one autopilot run over a real socket and feeds every streamed
// frame to the tally.
//
// Not app.Test: that helper's 10-second ceiling is shorter than a single live round, so
// every run would be reported as a failure of the model rather than of the test client.
func runBakeoffAutopilot(t *testing.T, addr, sessionID, cookie string, tally *bakeoffTally) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), bakeoffRunTimeout)
	defer cancel()

	url := "http://" + addr + "/api/v1/assistant/sessions/" + sessionID + "/autopilot"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})

	// No client timeout: the context already bounds the whole read, and a client timeout
	// would additionally cap the idle gap between frames — which on this gateway is a
	// normal long round, not a hang.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("open the run: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != fiber.StatusOK {
		return fmt.Errorf("autopilot returned %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	// A tool result carrying a vacancy description is far past bufio's 64 KB default, and a
	// truncated frame reads as a stream that ended.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		payload, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok {
			continue
		}
		var e assistant.Event
		if err := json.Unmarshal([]byte(payload), &e); err != nil {
			// A frame we cannot read is a hole in the measurement, not a failed run: the
			// heartbeat and the open comment are not events at all, and they arrive on
			// lines this prefix never matches.
			continue
		}
		tally.observeEvent(e)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read the run's stream: %w", err)
	}
	return nil
}

// scoreBakeoffRun computes the two deterministic scores over the tailored CV, and returns
// the rendered text the report carries.
//
// Both scores read the RENDERED text layer rather than the stored document, which is the
// whole point: a bullet the active template never prints contributes nothing to either.
func scoreBakeoffRun(t *testing.T, h *cvHandlers, userID int64, cvID uuid.UUID, base atscheck.Report, c bakeoffCase, jobSkills []string) (cvmatch.Score, atscheck.Delta, string, error) {
	t.Helper()
	ctx := context.Background()

	rec, err := h.cvStore.Get(ctx, cvID, userID)
	if err != nil {
		return cvmatch.Score{}, atscheck.Delta{}, "", fmt.Errorf("read the tailored cv: %w", err)
	}
	tmpl, err := cv.ResolveTemplate(rec.TemplateID)
	if err != nil {
		return cvmatch.Score{}, atscheck.Delta{}, "", fmt.Errorf("resolve template: %w", err)
	}
	text, err := h.renderedCVText(ctx, rec.Document, tmpl)
	if err != nil {
		return cvmatch.Score{}, atscheck.Delta{}, "", fmt.Errorf("render the tailored cv: %w", err)
	}

	skills := skilltag.Parse(text, skilltag.WithResumeAcronyms())
	match := cvmatch.Compute(cvmatch.Input{
		CVText: text, CVSkills: skills,
		JobTitle: c.Vacancy.Title, JobSkills: jobSkills,
	})
	delta := atscheck.Compare(base, atscheck.Score(text, skills, jobSkills))
	return match, delta, text, nil
}

// bakeoffReport is what one bake-off writes out: the rows, and the two facts without which a
// row cannot be read a week later — when its prices were captured, and whose CV it ran.
type bakeoffReport struct {
	Ran             string       `json:"ran"`
	PricesCaptured  string       `json:"prices_captured"`
	PricesSource    string       `json:"prices_source"`
	ProfileCaptured string       `json:"profile_captured"`
	CasesCaptured   string       `json:"cases_captured"`
	FitModel        string       `json:"fit_model"`
	Rows            []bakeoffRow `json:"rows"`
}

// writeBakeoffReport puts the report under .cache/, which is not in git — it carries the
// tailored CVs, and those are the candidate's own document.
func writeBakeoffReport(t *testing.T, rep bakeoffReport) string {
	t.Helper()
	root, err := modroot.Find()
	if err != nil {
		t.Fatalf("locate the module root: %v", err)
	}
	dir := filepath.Join(root, ".cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("make %s: %v", dir, err)
	}
	path := filepath.Join(dir, "bakeoff-"+time.Now().UTC().Format("20060102-150405")+".json")
	blob, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatalf("encode the report: %v", err)
	}
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// printBakeoffTable prints the ranked rows to the test log, best first.
func printBakeoffTable(t *testing.T, rows []bakeoffRow) {
	t.Helper()
	t.Log("model | case | match | ats | rounds | in/out/cached | cache | cost | outcome")
	for _, r := range rows {
		match, ats := "—", "—"
		if r.Match != nil {
			match = fmt.Sprintf("%d", r.Match.Overall)
		}
		if r.ATS != nil {
			ats = fmt.Sprintf("%+d", r.ATS.Change)
		}
		cost := "unknown"
		if r.Cost.Known {
			cost = fmt.Sprintf("$%.4f", r.Cost.USD)
			if r.Cost.Floor {
				cost = "≥" + cost
			}
		}
		outcome := r.Tally.Stop
		if r.Failure != "" {
			outcome = "FAILED: " + r.Failure
		}
		verdict := r.Tally.cacheVerdict()
		cache := verdict.State
		if verdict.State == cacheObserved {
			cache = fmt.Sprintf("%s %.0f%%", verdict.State, verdict.Share*100)
		}
		t.Logf("%s | %s | %s | %s | %d | %d/%d/%d | %s | %s | %s",
			r.Model, r.Case, match, ats, r.Tally.Rounds,
			r.Tally.Input, r.Tally.Output, r.Tally.CachedInput, cache, cost, outcome)
	}
}

// TestBakeoffRunsEveryCandidateOverEveryCase is the bake-off.
//
// One fresh database per MODEL, not per run: seeding once and running every model against
// the same database would make the first model's edits the second's starting point. Per-run
// isolation would be stricter and buys nothing — cases within one model's pass bind their
// own CV copy and their own vacancy.
//
// A run that fails, is cancelled or hits the step cap becomes a row carrying that outcome
// and the pass continues. Only failing to read the case set ends the bake-off: every model
// running over nothing would report a clean sweep of zero rows, which reads exactly like a
// bake-off that found no difference between them.
func TestBakeoffRunsEveryCandidateOverEveryCase(t *testing.T) {
	candidates := bakeoffCandidates(t)
	renderer, extract := bakeoffToolchain(t)

	set, err := loadBakeoffCases(bakeoffCaseFixture)
	if err != nil {
		t.Fatalf("loadBakeoffCases: %v", err)
	}
	profile := loadBakeoffProfileOrSkip(t)

	priceBlob, err := os.ReadFile(filepath.FromSlash(bakeoffPriceFixture))
	if err != nil {
		t.Fatalf("read the price table: %v", err)
	}
	prices, err := parseBakeoffPrices(priceBlob)
	if err != nil {
		t.Fatalf("parseBakeoffPrices: %v", err)
	}

	// The fit model is built ONCE, outside the candidate loop, and pinned.
	// Built through llm.NewClient, never through the harness's plain wrapper: the wrapper
	// installs no HTTP transport, and `reasoning_effort` is written by one — so Stage 1's
	// llm.ReasoningNone is silently dropped and the stage blows its deadline on every
	// analysis. See withFitClient.
	//
	// WithTimeout matches what production gives the chain, so the analysis the bake-off runs
	// against is the one the product runs.
	fitID := os.Getenv("LLM_MODEL")
	fitBase, flushFit := bakeoffGateway(t, fitID)
	defer flushFit()
	fitClient := fitBase.WithTimeout(matchAnalysisLLMTimeout)

	var rows []bakeoffRow
	for _, model := range candidates {
		// The assertion task 5.2 asks for, made where it can actually fail: if a later edit
		// rebinds the fit chain to the candidate, this is what stops the run rather than
		// letting it publish two models measured as one.
		if got := fitClient.ModelID(); got != fitID {
			t.Fatalf("the fit model moved to %q while measuring %q; only the turn model may vary", got, model)
		}
		rows = append(rows, runBakeoffPass(t, model, set, profile, prices, fitClient, renderer, extract)...)
	}

	rankBakeoffRows(rows)
	printBakeoffTable(t, rows)
	path := writeBakeoffReport(t, bakeoffReport{
		Ran:             time.Now().UTC().Format(time.RFC3339),
		PricesCaptured:  prices.Captured,
		PricesSource:    prices.Source,
		ProfileCaptured: profile.Captured,
		CasesCaptured:   set.Captured,
		FitModel:        fitID,
		Rows:            rows,
	})
	t.Logf("report written to %s", path)

	// A bake-off whose every run failed is a report of a broken gateway wearing the shape of
	// a model comparison, and nothing downstream could tell them apart.
	var completed int
	for _, r := range rows {
		if r.Failure == "" {
			completed++
		}
	}
	if completed == 0 {
		t.Fatalf("every one of %d runs failed; the report is a measurement of nothing", len(rows))
	}
}

// runBakeoffPass runs one candidate model over every case, on a database of its own.
func runBakeoffPass(
	t *testing.T,
	model string,
	set bakeoffCaseSet,
	profile bakeoffProfile,
	prices bakeoffPriceTable,
	fitClient *llm.Client,
	renderer cv.Renderer,
	extract func([]byte) (string, error),
) []bakeoffRow {
	t.Helper()

	turnClient, flush := bakeoffGateway(t, model)
	defer flush()
	// The deadline production gives a turn, not llm.DefaultTimeout.
	//
	// They differ by a factor of two — 180s against 90s — and the gap is not academic: the
	// first full bake-off (2026-09-08) had every flagship run die on `llm: chat: context
	// deadline exceeded` after two rounds, so all three rows reported the BASE CV, unedited,
	// and scored identically to the other candidate's. A bake-off run under a deadline the
	// product does not use measures the test rig.
	turn := turnClient.WithTimeout(assistantLLMTimeout)

	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	userID, cookie := assistantUser(t, pool, iss, "bakeoff@example.test", true)
	if banked := seedBakeoffProfile(t, pool, userID, profile); banked == 0 {
		t.Fatal("the profile banked no achievement; every edit would bounce off the evidence gate")
	}

	rows := make([]bakeoffRow, 0, len(set.Cases))
	for _, c := range set.Cases {
		// The tally is per RUN, and the model wrapper closes over it, so the harness is
		// rebuilt per case rather than reused.
		tally := &bakeoffTally{}
		h, app := newAutopilotHarness(t, pool, iss, &countingModel{inner: turn, tally: tally}, nil,
			withFitClient(fitClient), withRenderedCVScoring(renderer, extract), withTrackingTools(pool, db.New(pool)))

		sess, cvID, _ := seedBakeoffCase(t, pool, h, userID, c, string(profile.CV))

		// The base report is taken BEFORE the run, off the same document the run starts
		// from, so the ATS delta describes what this run changed and not what the profile
		// already was.
		base, err := bakeoffBaseReport(t, h.cv, userID, cvID, c)
		if err != nil {
			rows = append(rows, bakeoffRow{Model: model, Case: c.ID, Vacancy: c.Vacancy.Title,
				Tally: *tally, Cost: prices.Rates.cost(model, *tally), Failure: err.Error()})
			continue
		}

		addr := serveOnSocket(t, app)
		runErr := runBakeoffAutopilot(t, addr, sess.ID.String(), cookie, tally)

		row := bakeoffRow{
			Model: model, Case: c.ID, Vacancy: c.Vacancy.Title,
			Tally: *tally, Cost: prices.Rates.cost(model, *tally),
		}
		if runErr != nil {
			row.Failure = runErr.Error()
			rows = append(rows, row)
			continue
		}

		match, delta, text, err := scoreBakeoffRun(t, h.cv, userID, cvID, base, c, bakeoffJobSkills(c))
		if err != nil {
			row.Failure = err.Error()
			rows = append(rows, row)
			continue
		}
		row.Match, row.ATS, row.TailoredCV = &match, &delta, text
		rows = append(rows, row)
	}
	return rows
}

// bakeoffBaseReport scores the document the run STARTS from, so the ATS delta measures the
// run rather than the profile.
func bakeoffBaseReport(t *testing.T, h *cvHandlers, userID int64, cvID uuid.UUID, c bakeoffCase) (atscheck.Report, error) {
	t.Helper()
	ctx := context.Background()
	rec, err := h.cvStore.Get(ctx, cvID, userID)
	if err != nil {
		return atscheck.Report{}, fmt.Errorf("read the pre-run cv: %w", err)
	}
	tmpl, err := cv.ResolveTemplate(rec.TemplateID)
	if err != nil {
		return atscheck.Report{}, fmt.Errorf("resolve template: %w", err)
	}
	text, err := h.renderedCVText(ctx, rec.Document, tmpl)
	if err != nil {
		return atscheck.Report{}, fmt.Errorf("render the pre-run cv: %w", err)
	}
	return atscheck.Score(text, skilltag.Parse(text, skilltag.WithResumeAcronyms()), bakeoffJobSkills(c)), nil
}

// bakeoffJobSkills reads the vacancy's own skills out of its posting with the same
// dictionary the catalogue tags jobs with, so both scores are grounded in the posting rather
// than in a list written for this test.
func bakeoffJobSkills(c bakeoffCase) []string {
	return skilltag.Parse(c.Vacancy.Title + "\n" + c.Vacancy.Description)
}
