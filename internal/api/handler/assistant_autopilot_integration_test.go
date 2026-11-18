//go:build integration

// Integration tests for the autopilot run endpoint (see the tailor-autopilot change): it
// runs only on a tailoring session bound to a CV, it takes the pre-run snapshot itself, and
// the brief and the turn's ceiling are the server's rather than the caller's.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// autopilotConfig is what a caller may say about a harness beyond the two models.
//
// Read while the handlers are ASSEMBLED rather than applied to them afterwards: the fit
// analyzer is built into a surface that closes over it, so replacing it after the fact
// would leave the closure holding the old one.
type autopilotConfig struct {
	// fitClient overrides the client the fit chain runs on. Nil wraps fitM the plain way.
	fitClient *llm.Client
	// renderer and extract are the CV toolchain the deterministic scores read.
	renderer cv.Renderer
	extract  func([]byte) (string, error)
	// tracking is the job-tracking surface the tool registry offers to every preset.
	tracking *trackingHandlers
}

// autopilotOption adjusts a harness past the two models every caller names.
//
// Variadic rather than a widened signature: the five arguments below are what an autopilot
// test is ABOUT, and the one caller wanting more is the bake-off. Adding a sixth parameter
// would edit six call sites to say nil for something none of them has an opinion on.
type autopilotOption func(*autopilotConfig)

// withFitClient runs the fit chain on a client the caller built, instead of on the plain
// wrapper around fitM.
//
// It exists because llm.NewWithModel — which is what the wrapper is — produces a client with
// NO HTTP transport, and `reasoning_effort` is written BY a transport (llm.Client.transport
// installs reasoningInjector; only llm.New does). So on a wrapped model every
// llm.WithReasoning is silently dropped.
//
// Against a fake that costs nothing. Against a real gateway it is the difference between a
// working chain and none: matchanalysis asks Stage 1 for llm.ReasoningNone (#2640), a
// wrapped client never sends it, GLM deliberates for ~1500 reasoning tokens, and the stage
// blows its 90s deadline — twice, since it retries — on every single analysis. Measured
// 2026-09-08: 6 of 6 attempts, while production ran 21 analyses in the same day with no
// Stage 1 failure at all.
func withFitClient(c *llm.Client) autopilotOption {
	return func(cfg *autopilotConfig) { cfg.fitClient = c }
}

// withTrackingTools wires the tracking surface the tool registry offers to EVERY preset,
// the tailoring autopilot included (see assistantHandlers.registry).
//
// It matters to a live run and to nothing else: a scripted stand-in never calls a tool it
// was not scripted to call, while a real model reaches for save_job and my_jobs mid-run and
// gets "job tracking is not available". That refusal costs a round, and rounds are the
// bake-off's primary measurement — the transcript is replayed into each one.
//
// The search surface has no equivalent option: it needs a Meilisearch, which this harness
// does not stand. A bake-off therefore measures models whose search_jobs refuses. Every
// candidate faces the same refusal, so the rows stay comparable to each other; they are not
// comparable to production, and a report read as though they were would overstate how many
// rounds a turn takes.
func withTrackingTools(pool *pgxpool.Pool, queries *db.Queries) autopilotOption {
	return func(cfg *autopilotConfig) {
		cfg.tracking = &trackingHandlers{
			tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool)),
		}
	}
}

// withRenderedCVScoring wires the CV toolchain the deterministic scores read.
//
// cvmatch and atscheck both score the text layer of the RENDERED PDF rather than the stored
// document (renderedCVText says why), and a handler holding neither half does not fail that
// read — it degrades to an unavailable score. That is right for the workspace, whose panel
// must keep loading without a toolchain, and wrong for the bake-off, which would rank a
// column of absences and report it as a tie.
//
// Both halves together or neither: renderedCVText checks for both, and a harness carrying
// the renderer alone would reach the extractor with nothing behind it.
func withRenderedCVScoring(r cv.Renderer, extract func([]byte) (string, error)) autopilotOption {
	return func(cfg *autopilotConfig) {
		cfg.renderer = r
		cfg.extract = extract
	}
}

// newAutopilotHarness wires the handlers, the app and the routes an autopilot test needs.
// Every test here wants the same assembly — the CV tools, the editor, the experience bank and
// a match surface — and differs only in the two models: the one that answers the turn and the
// one that answers the fit chain.
func newAutopilotHarness(t *testing.T, pool *pgxpool.Pool, iss *auth.Issuer, turnM assistant.Model, fitM llms.Model, opts ...autopilotOption) (*assistantHandlers, *fiber.App) {
	t.Helper()
	var cfg autopilotConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	fitClient := cfg.fitClient
	if fitClient == nil {
		fitClient = llm.NewWithModel(fitM)
	}

	queries := db.New(pool)
	bank := experience.NewStore(experience.NewQueriesRepository(queries))
	h := &assistantHandlers{
		store: assistant.NewStore(queries), queries: queries,
		maxPrompt:  defaultAssistantMaxPrompt,
		stages:     queries,
		experience: bank,
		// The run plan reads the cached analysis through the service, and the tools read the
		// vacancy through jobs — both held by the assistant itself now.
		fit:      fitanalysis.New(queries, nil, matchanalysis.NewAnalyzer(nil)),
		jobs:     queries,
		tracking: cfg.tracking,
		cv: &cvHandlers{
			cvStore:        cv.NewStore(cv.NewQueriesRepository(queries)),
			editor:         cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: bank}),
			queries:        queries,
			jobReader:      queries,
			cvRenderer:     cfg.renderer,
			extractPDFText: cfg.extract,
			fit:            fitanalysis.New(queries, nil, matchanalysis.NewAnalyzer(nil)),
			match: fitAPI(pool, queries, iss, resume.New(nil, resume.NewQueriesRepository(queries)),
				matchanalysis.NewAnalyzer(fitClient)),
		},
	}
	h.runner = assistant.NewRunner(turnM, h.store, assistant.RunnerConfig{MaxSteps: 3})

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	mw := middleware{
		paidAI: markPaidAI,
		cookie: auth.RequireAuth(iss, testVersions),
		key:    auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}),
	}
	h.register(api, mw, newContract(contractHandlers{assistantHandlers: h}))
	h.cv.register(api, mw, cvContract(h.cv))
	return h, app
}

// walkedTheRequirements is the turn model an autopilot test uses when the run itself is not
// what it is checking: one answer, no tool calls.
func walkedTheRequirements() *turnModel {
	return &turnModel{replies: []*llms.ContentChoice{{Content: "Walked the requirements."}}}
}

// fullFitChain is a fit model that answers all three stages, cycling so one model can serve
// more than one analysis.
func fullFitChain() *fitModel {
	return &fitModel{resp: []string{fitStage1, fitStage2, fitStage3}}
}

// seedTailoringSession creates a CV bound to a vacancy plus the tailoring session that
// addresses it, and returns the session id and the CV id.
func seedTailoringSession(t *testing.T, pool *pgxpool.Pool, h *assistantHandlers, userID int64) (string, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('greenhouse', 'autopilot-1', 'https://example.test/j/1', 'Go Developer', 'go-developer-autopilot')
		 RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	var cvID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO cvs (user_id, title, template_id, data, job_id)
		 VALUES ($1, 'Tailored', 'classic-ats', '{"summary":"before the run"}'::jsonb, $2)
		 RETURNING id`, userID, jobID).Scan(&cvID); err != nil {
		t.Fatalf("seed cv: %v", err)
	}
	sess, err := h.store.CreateSession(ctx, userID, assistant.PresetTailor, &cvID, &jobID)
	if err != nil {
		t.Fatalf("create tailoring session: %v", err)
	}
	return sess.ID.String(), cvID
}

func TestAutopilotRunsOnATailoringSessionAndSnapshotsFirst(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "Walked the requirements."}}}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "autopilot@example.test", true)

	sessionID, _ := seedTailoringSession(t, pool, h, userID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	stream := string(body)
	for _, want := range []string{"event: user_prompt", "event: result"} {
		if !strings.Contains(stream, want) {
			t.Errorf("stream is missing %q:\n%s", want, stream)
		}
	}

	// A run no longer snapshots the document: every edit it makes carries the batch it
	// belongs to, and undoing the run is undoing those. Nothing is taken up front, so
	// two runs started at once can no longer snapshot over each other.

	// The brief is the server's too: the recorded prompt is one we wrote, and the caller
	// sent no text at all.
	msgs, err := h.store.Transcript(context.Background(), mustUUID(t, sessionID))
	if err != nil {
		t.Fatalf("transcript: %v", err)
	}
	if len(msgs) == 0 || msgs[0].Role != "user" || len(msgs[0].Content) == 0 {
		t.Fatalf("transcript = %+v, want it to open with the server's brief", msgs)
	}
}

func TestAutopilotIsRefusedOnANonTailoringSession(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAssistantApp(pool, iss, &turnModel{})
	_, cookie := assistantUser(t, pool, iss, "autopilot-chat@example.test", true)

	// A plain chat session: no CV, no vacancy, no tailoring tools.
	id := createSession(t, app, cookie)
	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+id+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusConflict {
		t.Errorf("autopilot on a chat session: status %d, want 409", resp.StatusCode)
	}
}

func TestAutopilotOnAForeignSessionIsNotFound(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, &turnModel{})
	owner, _ := assistantUser(t, pool, iss, "autopilot-owner@example.test", true)
	_, strangerCookie := assistantUser(t, pool, iss, "autopilot-stranger@example.test", true)

	sessionID, _ := seedTailoringSession(t, pool, h, owner)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", strangerCookie, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("foreign session: status %d, want 404 — ownership never leaks as 403", resp.StatusCode)
	}
}

// TestAutopilotComputesAnalysisWhenMissing: the tailoring bootstrap no longer requires a
// pre-existing fit analysis (tailor-coldstart-autopilot). cv_context — the run's first tool call
// — still reads the analysis from the cache, so an autopilot run started on a vacancy with none
// cached must compute and cache one itself, inline, before the turn proceeds.
func TestAutopilotComputesAnalysisWhenMissing(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	fitM := fullFitChain()
	h, app := newAutopilotHarness(t, pool, iss, walkedTheRequirements(), fitM)

	userID, cookie := assistantUser(t, pool, iss, "autopilot-analysis@example.test", true)
	// The fit chain refuses to analyze a candidate with an empty bank (there is nothing to
	// reason over) — without this, ensureCachedAnalysis would correctly compute nothing, and
	// the test would pass for the wrong reason.
	seedBankedCareer(t, queries, userID)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)

	var jobID int64
	if err := pool.QueryRow(context.Background(), `SELECT job_id FROM cvs WHERE id = $1`, cvID).Scan(&jobID); err != nil {
		t.Fatalf("read job id: %v", err)
	}
	if _, err := queries.GetUserJobAnalysis(context.Background(),
		db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("test setup: analysis already cached (err = %v), want none", err)
	}

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if _, err := queries.GetUserJobAnalysis(context.Background(),
		db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID}); err != nil {
		t.Errorf("analysis not cached after the run: %v", err)
	}
	if fitM.n != 6 {
		t.Errorf("fit model called %d times, want 6 (one full chain run before the run, one after) — the post-run refresh must fire even when the pre-run step just computed one", fitM.n)
	}
}

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return id
}

// The revert is the other half of the run: it restores the document the run started from
// and clears the log that described the run, since that log now names edits that are gone.
// The CV read carries the run's log, so the workspace panel renders from the CV it already
// re-reads after every turn.
func TestCVReadCarriesTheRunReport(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, h := newAssistantApp(pool, iss, &turnModel{})
	userID, cookie := assistantUser(t, pool, iss, "autopilot-read@example.test", true)
	_, cvID := seedTailoringSession(t, pool, h, userID)

	ctx := context.Background()
	if err := h.cv.cvStore.SetAutopilotReport(ctx, cvID, userID, []cv.AutopilotEntry{
		{Requirement: "Kafka in production", Status: cv.AutopilotClosedBank, Note: "reframed"},
		{Requirement: "Team leadership", Status: cv.AutopilotOpen},
	}); err != nil {
		t.Fatalf("report: %v", err)
	}

	resp := assistantRequest(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+cvID.String(), cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("get cv: status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	// The revertable flag is gone: whether a run can be undone is a property of the history
	// feed, whose entries carry the batch they belong to.
	for _, want := range []string{`"autopilot_report"`, "Kafka in production", `"closed_bank"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("CV read is missing %q:\n%s", want, body)
		}
	}
}

// seedFitAnalysis caches a fit analysis for (user, job) so the run has a requirement list to
// lay down. The vacancy is the one seedTailoringSession created for this CV.
func seedFitAnalysis(t *testing.T, pool *pgxpool.Pool, userID int64, cvID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	var jobID int64
	if err := pool.QueryRow(ctx, `SELECT job_id FROM cvs WHERE id = $1`, cvID).Scan(&jobID); err != nil {
		t.Fatalf("read job id: %v", err)
	}
	analysis := `{"requirement_match":[
		{"text":"Kafka in production","priority":"required","status":"missing-have"},
		{"text":"Team leadership","priority":"preferred","status":"missing-gap"}
	]}`
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_job_analysis (user_id, job_id, analysis, model)
		 VALUES ($1, $2, $3::jsonb, 'test-model')`, userID, jobID, analysis); err != nil {
		t.Fatalf("seed analysis: %v", err)
	}
}

// brokenFitModel fails every call, the way a stage that blows its deadline does.
type brokenFitModel struct{ n int }

func (m *brokenFitModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	m.n++
	return nil, errors.New("stage timed out")
}
func (*brokenFitModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

// countingTurnModel answers like walkedTheRequirements and records whether it was called at
// all — a run that must not start is a run whose turn model never sees a request.
type countingTurnModel struct {
	turnModel
	calls int
}

func (m *countingTurnModel) Chat(ctx context.Context, msgs []llms.MessageContent, tools []llms.Tool, s llm.ChatStream) (*llms.ContentChoice, error) {
	m.calls++
	return m.turnModel.Chat(ctx, msgs, tools, s)
}

// An autopilot run whose pre-run fit analysis produced nothing must REFUSE, not proceed.
//
// The run's whole brief is to walk the vacancy's requirements, and that list comes from the
// analysis. Without one the run has no plan: cv_context answers "no analysis has been run for
// this job", the agent improvises, and it still spends a turn's worth of tokens editing the
// candidate's CV against nothing. Measured on prod 2026-08-25..09-08: 57 of 159 runs, 36%,
// went out this way, and nothing anywhere said so — `ensure` discarded Ensure's outcome and
// the run proceeded regardless.
//
// The refusal must reach the client as a terminal frame. The stream is already open by the
// time this runs (opening it first is what stopped a proxy cutting cold-start runs), and a
// stream that ends with no `result` leaves the session believing a turn is still in flight.
func TestAutopilotRefusesToRunWithoutItsRequirementList(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	turnM := &countingTurnModel{turnModel: turnModel{replies: []*llms.ContentChoice{{Content: "Walked the requirements."}}}}
	fitM := &brokenFitModel{}
	h, app := newAutopilotHarness(t, pool, iss, turnM, fitM)

	userID, cookie := assistantUser(t, pool, iss, "autopilot-no-plan@example.test", true)
	seedBankedCareer(t, queries, userID)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d, want 200 — the stream is already open", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	_ = resp.Body.Close()
	stream := string(body)

	if fitM.n == 0 {
		t.Fatal("the fit chain was never attempted; this test is not exercising the fill")
	}
	if turnM.calls != 0 {
		t.Errorf("the turn model was called %d times; a run with no requirement list must not start", turnM.calls)
	}
	// A terminal frame, or the session stays "turn in flight" and queues every later message
	// behind a turn that is over.
	if !strings.Contains(stream, "event: result") {
		t.Errorf("stream carries no terminal result:\n%s", stream)
	}
	// And a reason the candidate can act on, rather than a run that silently did nothing.
	if !strings.Contains(stream, "fit analysis") {
		t.Errorf("stream does not say why the run did not start:\n%s", stream)
	}

	// The CV is untouched: refusing costs the candidate a run, editing their document
	// against nothing costs them the document.
	rec, err := h.cv.cvStore.Get(context.Background(), cvID, userID)
	if err != nil {
		t.Fatalf("get cv: %v", err)
	}
	if rec.Document.Summary != "before the run" {
		t.Errorf("summary = %q, want the document untouched", rec.Document.Summary)
	}
}
