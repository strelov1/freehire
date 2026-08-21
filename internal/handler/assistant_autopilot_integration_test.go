//go:build integration

// Integration tests for the autopilot run endpoint (see the tailor-autopilot change): it
// runs only on a tailoring session bound to a CV, it takes the pre-run snapshot itself, and
// the brief and the turn's ceiling are the server's rather than the caller's.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/resume"
)

// newAutopilotHarness wires the handlers, the app and the routes an autopilot test needs.
// Every test here wants the same assembly — the CV tools, the editor, the experience bank and
// a match surface — and differs only in the two models: the one that answers the turn and the
// one that answers the fit chain.
func newAutopilotHarness(t *testing.T, pool *pgxpool.Pool, iss *auth.Issuer, turnM assistant.Model, fitM llms.Model) (*assistantHandlers, *fiber.App) {
	t.Helper()
	queries := db.New(pool)
	bank := experience.NewStore(experience.NewQueriesRepository(queries))
	h := &assistantHandlers{
		store: assistant.NewStore(queries), queries: queries,
		maxPrompt:  defaultAssistantMaxPrompt,
		stages:     queries,
		experience: bank,
		cv: &cvHandlers{
			cvStore:            cv.NewStore(cv.NewQueriesRepository(queries)),
			editor:             cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: bank}),
			queries:            queries,
			jobReader:          queries,
			matchAnalysisCache: queries,
			match: fitAPI(pool, queries, iss, resume.New(nil, resume.NewQueriesRepository(queries)),
				matchanalysis.NewAnalyzer(llm.NewWithModel(fitM))),
		},
	}
	h.runner = assistant.NewRunner(turnM, h.store, assistant.RunnerConfig{MaxSteps: 3})

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	mw := middleware{
		cookie: auth.RequireAuth(iss, testVersions),
		key:    auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}),
	}
	h.register(api, mw)
	h.cv.register(api, mw)
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

// gatedFitModel is a fitModel that holds its FIRST call until released, so a test can inspect
// the world while the cold-start chain is provably mid-flight.
type gatedFitModel struct {
	fitModel
	started     chan struct{}
	release     chan struct{}
	holdOnce    sync.Once
	releaseOnce sync.Once
}

func newGatedFitModel(t *testing.T) *gatedFitModel {
	m := &gatedFitModel{
		fitModel: fitModel{resp: []string{fitStage1, fitStage2, fitStage3}},
		started:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	t.Cleanup(m.letGo)
	return m
}

// letGo releases the model however many times it is called, so a test may release it
// explicitly and still be cleaned up after.
func (m *gatedFitModel) letGo() { m.releaseOnce.Do(func() { close(m.release) }) }

func (m *gatedFitModel) GenerateContent(ctx context.Context, msgs []llms.MessageContent, opts ...llms.CallOption) (*llms.ContentResponse, error) {
	m.holdOnce.Do(func() {
		close(m.started)
		<-m.release
	})
	return m.fitModel.GenerateContent(ctx, msgs, opts...)
}

// TestAutopilotAnswersBeforeTheColdStartAnalysis: the response must open BEFORE the cold-start
// fit analysis runs, not after it.
//
// The pre-run used to happen in the handler, ahead of streamSSE, so on a vacancy with nothing
// cached the response stayed silent for as long as the three-stage chain took — measured at
// 2m6s on prod, 2026-08-21, against nginx's 60s default. The proxy cut five runs that day and
// the browser reported "could not start the run" for runs that were running fine and went on
// to finish. Nothing downstream can tell those two apart, so the fix has to be here: open the
// stream first, wait inside it, where the heartbeat is already ticking.
func TestAutopilotAnswersBeforeTheColdStartAnalysis(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	fitM := newGatedFitModel(t)
	h, app := newAutopilotHarness(t, pool, iss, walkedTheRequirements(), fitM)

	userID, cookie := assistantUser(t, pool, iss, "autopilot-ttfb@example.test", true)
	seedBankedCareer(t, queries, userID)
	sessionID, _ := seedTailoringSession(t, pool, h, userID)

	addr := serveOnSocket(t, app)
	conn := dialAutopilotTurn(t, addr, sessionID, cookie)
	defer func() { _ = conn.Close() }()

	select {
	case <-fitM.started:
	case <-time.After(15 * time.Second):
		t.Fatal("the cold-start analysis never started")
	}

	// The chain is held mid-flight. The response line must already be on the wire: what a
	// proxy counts is silence from the upstream, and this is the silence that cost the runs.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("nothing answered while the cold-start analysis is still running (%v) — the "+
			"stream opens only after the chain, which is exactly what a proxy times out", err)
	}
	if !strings.HasPrefix(status, "HTTP/1.1 200") {
		t.Fatalf("response line %q, want HTTP/1.1 200", strings.TrimSpace(status))
	}
	fitM.letGo()
}

// TestAutopilotRefreshesAnalysisAfterEveryRun: the fit analysis is no longer a frozen
// snapshot of the base profile (see docs/superpowers/specs/2026-08-09-fit-analysis-post-autopilot-verify-design.md).
// An autopilot run must recompute and overwrite the cached (user, job) row once it ends,
// even when a (now-stale) analysis was already cached and the run itself made zero edits.
func TestAutopilotRefreshesAnalysisAfterEveryRun(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	fitM := fullFitChain()
	h, app := newAutopilotHarness(t, pool, iss, walkedTheRequirements(), fitM)

	userID, cookie := assistantUser(t, pool, iss, "autopilot-refresh@example.test", true)
	seedBankedCareer(t, queries, userID)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)

	var jobID int64
	if err := pool.QueryRow(context.Background(), `SELECT job_id FROM cvs WHERE id = $1`, cvID).Scan(&jobID); err != nil {
		t.Fatalf("read job id: %v", err)
	}
	// A stale analysis under a model id the fake never produces, so a later match on the
	// LIVE model id (empty string — llm.NewWithModel sets none) can only mean the row was
	// actually overwritten, not left alone.
	if err := queries.UpsertUserJobAnalysis(context.Background(), db.UpsertUserJobAnalysisParams{
		UserID: userID, JobID: jobID, Analysis: []byte(`{"verdict":"Good Fit","overall_score":70}`), Model: "stale-model",
	}); err != nil {
		t.Fatalf("seed cached analysis: %v", err)
	}

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if fitM.n != 3 {
		t.Errorf("fit model called %d times, want exactly 3 (one full chain run) — a cached analysis must still be refreshed once, not skipped and not recomputed twice", fitM.n)
	}
	row, err := queries.GetUserJobAnalysis(context.Background(), db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
	if err != nil {
		t.Fatalf("read cached analysis: %v", err)
	}
	if row.Model == "stale-model" {
		t.Error("cached analysis still carries the pre-run model stamp — the row was not overwritten")
	}
}

// dialAutopilotTurn opens an autopilot run on a raw socket, the same way dialTurn opens an
// ordinary message — the autopilot endpoint reads no body, so none is sent.
func dialAutopilotTurn(t *testing.T, addr, sessionID, cookie string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	request := fmt.Sprintf("POST /api/v1/assistant/sessions/%s/autopilot HTTP/1.1\r\n"+
		"Host: 127.0.0.1\r\nCookie: %s=%s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n",
		sessionID, auth.CookieName, cookie)
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	return conn
}

// dialAutopilotTurnInBackground opens an autopilot run and keeps reading it, exactly like
// startTurnInBackground does for an ordinary message. The returned channel closes when the
// stream ends.
func dialAutopilotTurnInBackground(t *testing.T, addr, sessionID, cookie string) <-chan struct{} {
	t.Helper()
	conn := dialAutopilotTurn(t, addr, sessionID, cookie)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	}()
	return done
}

// TestAutopilotRefreshSurvivesACancelledRun: the guaranteed post-run refresh (see
// TestAutopilotRefreshesAnalysisAfterEveryRun) must not be defeated by CancelAssistantTurn —
// cancelling shares the run's own ctx, and the refresh runs on context.WithoutCancel(ctx)
// specifically so a candidate who stops a run mid-flight still gets a current fit analysis,
// not the code silently skipping it because the ctx it inherited was already dead.
func TestAutopilotRefreshSurvivesACancelledRun(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	turnM := newDisconnectModel(t)
	fitM := fullFitChain()
	h, app := newAutopilotHarness(t, pool, iss, turnM, fitM)

	userID, cookie := assistantUser(t, pool, iss, "autopilot-cancel-refresh@example.test", true)
	seedBankedCareer(t, queries, userID)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)

	var jobID int64
	if err := pool.QueryRow(context.Background(), `SELECT job_id FROM cvs WHERE id = $1`, cvID).Scan(&jobID); err != nil {
		t.Fatalf("read job id: %v", err)
	}
	// Seeded so the pre-run ensure step is a no-op (cache hit): every fitM call below can
	// only come from the post-run refresh, isolating exactly the thing this test checks.
	if err := queries.UpsertUserJobAnalysis(context.Background(), db.UpsertUserJobAnalysisParams{
		UserID: userID, JobID: jobID, Analysis: []byte(`{"verdict":"Good Fit","overall_score":70}`), Model: "stale-model",
	}); err != nil {
		t.Fatalf("seed cached analysis: %v", err)
	}

	addr := serveOnSocket(t, app)
	streamed := dialAutopilotTurnInBackground(t, addr, sessionID, cookie)

	select {
	case <-turnM.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never started")
	}

	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+sessionID+"/cancel", cookie, nil)
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("cancel: status %d, want 204", resp.StatusCode)
	}
	turnM.letGo()
	<-streamed

	// The refresh runs after runner.Run returns, inside the same detached goroutine as the
	// turn — poll briefly rather than assuming it has already landed the instant the stream
	// closed.
	deadline := time.Now().Add(5 * time.Second)
	for fitM.n != 3 {
		if time.Now().After(deadline) {
			t.Fatalf("fit model called %d times after 5s, want 3 — the post-run refresh must survive a cancelled run", fitM.n)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := queries.GetUserJobAnalysis(context.Background(),
		db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID}); err != nil {
		t.Errorf("analysis not cached after a cancelled run: %v", err)
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

// scriptedTurnModel replays a fixed sequence of replies AND records the tool results it was
// fed, so a test can assert on what the tools told the model — including a refusal.
type scriptedTurnModel struct {
	replies []*llms.ContentChoice
	seen    []string
}

func (m *scriptedTurnModel) Chat(_ context.Context, msgs []llms.MessageContent, _ []llms.Tool, s llm.ChatStream) (*llms.ContentChoice, error) {
	for _, msg := range msgs {
		if msg.Role != llms.ChatMessageTypeTool {
			continue
		}
		for _, part := range msg.Parts {
			if r, ok := part.(llms.ToolCallResponse); ok {
				m.seen = append(m.seen, r.Content)
			}
		}
	}
	if len(m.replies) == 0 {
		return &llms.ContentChoice{Content: "done"}, nil
	}
	reply := m.replies[0]
	m.replies = m.replies[1:]
	if s.OnText != nil && reply.Content != "" {
		s.OnText(reply.Content)
	}
	return reply, nil
}

// The shape of a real run, driven end to end: the agent searches the bank, tries to write a
// bullet without citing evidence and is refused, writes it citing the evidence, records the
// report, and closes with a summary. The provenance wall must hold inside an unattended run
// exactly as it does in conversation — that is the one rule the whole capability rests on.
func TestAnAutopilotRunSearchesEditsAndReports(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	ctx := context.Background()

	queries := db.New(pool)
	bank := experience.NewStore(experience.NewQueriesRepository(queries))

	model := &scriptedTurnModel{}
	app, h := newAssistantApp(pool, iss, model)
	h.experience = bank
	userID, cookie := assistantUser(t, pool, iss, "autopilot-run@example.test", true)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)

	// The candidate's own words are in the bank, so a bullet may cite them.
	atom, err := bank.AddAtom(ctx, userID, experience.Atom{
		Claim:      "Ran the payments Kafka cluster through a 10x traffic year.",
		Skills:     []string{"kafka"},
		Provenance: experience.ProvenanceStatedInChat,
	})
	if err != nil {
		t.Fatalf("seed experience atom: %v", err)
	}

	model.replies = []*llms.ContentChoice{
		callReplyChoice("experience_search", `{"query":"Kafka"}`),
		// A bullet with no evidence_id: the claim wall must refuse it mid-run, exactly as it
		// does in conversation. (set_summary would NOT be refused — a summary asserts nothing
		// the bank has to back — so the attempt has to be a bullet to test the rule.)
		callReplyChoice("cv_edit", `{"ops":[{"kind":"insert","path":"experience[0].bullets[0]","value":"Led a team of twelve."}]}`),
		callReplyChoice("cv_edit", `{"ops":[{"kind":"insert","path":"experience[0].bullets[0]","value":"Ran the payments Kafka cluster through a 10x traffic year.","evidence_id":"`+atom.ID.String()+`"}]}`),
		callReplyChoice("tailor_report", `{"items":[
			{"requirement":"Kafka in production","status":"closed_bank","note":"Cited the payments cluster."},
			{"requirement":"Team leadership","status":"open","note":"Nothing in the bank."}
		]}`),
		{Content: "Closed one of two. Have you led a team?"},
	}

	// The CV needs an experience entry for add_bullet to address.
	if _, err := pool.Exec(ctx,
		`UPDATE cvs SET data = '{"summary":"before the run","experience":[{"company":"Acme","title":"Engineer","bullets":[]}]}'::jsonb
		 WHERE id = $1`, cvID); err != nil {
		t.Fatalf("seed cv document: %v", err)
	}

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read stream: %v", err)
	}

	// The unevidenced edit came back as a refusal naming what to do next — inside the run.
	var refused bool
	for _, result := range model.seen {
		if strings.Contains(result, "evidence_id") && strings.Contains(result, "experience_search") {
			refused = true
		}
	}
	if !refused {
		t.Errorf("an unevidenced bullet was not refused during the run; tool results were %q", model.seen)
	}

	rec, err := h.cv.cvStore.Get(ctx, cvID, userID)
	if err != nil {
		t.Fatalf("get cv: %v", err)
	}
	if len(rec.Document.Experience) == 0 || len(rec.Document.Experience[0].Bullets) != 1 {
		t.Fatalf("document = %+v, want exactly the evidenced bullet — the refused one must not be there", rec.Document)
	}
	if !strings.Contains(rec.Document.Experience[0].Bullets[0], "Kafka") {
		t.Errorf("bullet = %q, want the evidenced one", rec.Document.Experience[0].Bullets[0])
	}
	if len(rec.AutopilotReport) != 2 || rec.AutopilotReport[0].Status != cv.AutopilotClosedBank {
		t.Errorf("report = %+v, want both requirements recorded", rec.AutopilotReport)
	}
	// Whether the run can be undone is no longer a flag on the CV: its edits carry the batch
	// they belong to, and undoing the run is undoing them.
}

// A run that never reaches its own report still has to leave one. The runner's last call on
// hitting the step cap offers NO tools, so a capped run cannot call tailor_report at all —
// and a CV edited by a run with no report on it would show the workspace no way to undo it.
// The server therefore lays down the requirement list as `not_reached` when the run starts;
// whatever the agent reports later replaces it.
func TestARunThatNeverReportsStillLeavesOne(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	// The model answers in prose and calls nothing: the run "ends" having reported nothing.
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "I had a look."}}}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "autopilot-silent@example.test", true)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)
	seedFitAnalysis(t, pool, userID, cvID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read stream: %v", err)
	}

	rec, err := h.cv.cvStore.Get(context.Background(), cvID, userID)
	if err != nil {
		t.Fatalf("get cv: %v", err)
	}
	if len(rec.AutopilotReport) == 0 {
		t.Fatal("a run that reported nothing left no report; the panel would offer no way to undo it")
	}
	for _, entry := range rec.AutopilotReport {
		if entry.Status != cv.AutopilotNotReached {
			t.Errorf("entry %q = %q, want every requirement recorded as not reached", entry.Requirement, entry.Status)
		}
	}
}

// seedTailoringSessionWithSurfaces creates a vacancy whose description prefers the long
// form of infrastructure-as-code, and a tailored CV whose skills chip still says IaC.
func seedTailoringSessionWithSurfaces(t *testing.T, pool *pgxpool.Pool, h *assistantHandlers, userID int64) (string, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug, description)
		 VALUES ('greenhouse', $1, 'https://example.test/j/'||$1, 'Platform Engineer', $1,
		         'We practice infrastructure as code and Terraform.')
		 RETURNING id`, "autopilot-align-"+uuid.NewString()[:8]).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	doc := `{"summary":"before the run","skills":[{"items":["IaC","Terraform"]}],` +
		`"experience":[{"company":"Acme","role":"Engineer","bullets":["Shipped platform tools"]}]}`
	var cvID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO cvs (user_id, title, template_id, data, job_id)
		 VALUES ($1, 'Tailored', 'classic-ats', $2::jsonb, $3)
		 RETURNING id`, userID, doc, jobID).Scan(&cvID); err != nil {
		t.Fatalf("seed cv: %v", err)
	}
	sess, err := h.store.CreateSession(ctx, userID, assistant.PresetTailor, &cvID, &jobID)
	if err != nil {
		t.Fatalf("create tailoring session: %v", err)
	}
	return sess.ID.String(), cvID
}

func TestAutopilotAlignsSkillSurfacesBeforeTurn(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "Walked the requirements."}}}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "autopilot-align@example.test", true)

	sessionID, cvID := seedTailoringSessionWithSurfaces(t, pool, h, userID)
	seedFitAnalysis(t, pool, userID, cvID)

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	io.ReadAll(resp.Body)

	rec, err := h.cv.cvStore.Get(context.Background(), cvID, userID)
	if err != nil {
		t.Fatalf("get cv: %v", err)
	}
	if len(rec.Document.Skills) == 0 || rec.Document.Skills[0].Items[0] != "infrastructure as code" {
		t.Fatalf("skills = %+v, want infrastructure as code before the turn", rec.Document.Skills)
	}

	var feed struct {
		Data []cvedit.RevisionView `json:"data"`
	}
	decodeJSON(t, assistantRequest(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+cvID.String()+"/revisions", cookie, nil), &feed)
	var alignRevs int
	for _, rev := range feed.Data {
		if rev.Actor == string(cvedit.ActorSystem) && rev.Origin == string(cvedit.OriginImport) && rev.BatchID == "" {
			alignRevs++
		}
	}
	if alignRevs != 1 {
		t.Fatalf("system align revisions = %d, want 1; feed=%+v", alignRevs, feed.Data)
	}

	// Already aligned: a second start is a no-op on the document and adds no revision.
	resp2 := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp2.StatusCode != fiber.StatusOK {
		t.Fatalf("second autopilot: status %d", resp2.StatusCode)
	}
	io.ReadAll(resp2.Body)
	decodeJSON(t, assistantRequest(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+cvID.String()+"/revisions", cookie, nil), &feed)
	alignRevs = 0
	for _, rev := range feed.Data {
		if rev.Actor == string(cvedit.ActorSystem) && rev.Origin == string(cvedit.OriginImport) && rev.BatchID == "" {
			alignRevs++
		}
	}
	if alignRevs != 1 {
		t.Fatalf("after second start system align revisions = %d, want still 1", alignRevs)
	}
}

func TestAutopilotUndoLeavesSurfaceAlign(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	ctx := context.Background()

	model := &scriptedTurnModel{}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "autopilot-undo-align@example.test", true)
	sessionID, cvID := seedTailoringSessionWithSurfaces(t, pool, h, userID)
	seedFitAnalysis(t, pool, userID, cvID)

	model.replies = []*llms.ContentChoice{
		callReplyChoice("cv_edit", `{"ops":[{"kind":"remove","path":"experience[0].bullets[0]"}]}`),
		{Content: "Removed a bullet."},
	}

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	io.ReadAll(resp.Body)

	rec, err := h.cv.cvStore.Get(ctx, cvID, userID)
	if err != nil {
		t.Fatalf("get cv: %v", err)
	}
	if len(rec.Document.Skills) == 0 || rec.Document.Skills[0].Items[0] != "infrastructure as code" {
		t.Fatalf("skills after run = %+v, want aligned form", rec.Document.Skills)
	}
	if len(rec.Document.Experience) == 0 || len(rec.Document.Experience[0].Bullets) != 0 {
		t.Fatalf("experience = %+v, want the run's remove to have cleared the bullet", rec.Document.Experience)
	}

	var feed struct {
		Data []cvedit.RevisionView `json:"data"`
	}
	decodeJSON(t, assistantRequest(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+cvID.String()+"/revisions", cookie, nil), &feed)
	var batchID string
	for _, rev := range feed.Data {
		if rev.Origin == string(cvedit.OriginTailorAgent) && rev.BatchID != "" {
			batchID = rev.BatchID
			break
		}
	}
	if batchID == "" {
		t.Fatalf("no agent batch in feed: %+v", feed.Data)
	}

	undo := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/me/cvs/"+cvID.String()+"/revisions/batch/"+batchID+"/undo", cookie, nil)
	if undo.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(undo.Body)
		t.Fatalf("undo batch = %d: %s", undo.StatusCode, body)
	}
	undo.Body.Close()

	rec, err = h.cv.cvStore.Get(ctx, cvID, userID)
	if err != nil {
		t.Fatalf("get cv after undo: %v", err)
	}
	if len(rec.Document.Skills) == 0 || rec.Document.Skills[0].Items[0] != "infrastructure as code" {
		t.Fatalf("skills after undo = %+v, want JD wording left by the align revision", rec.Document.Skills)
	}
	if len(rec.Document.Experience) == 0 || len(rec.Document.Experience[0].Bullets) != 1 {
		t.Fatalf("experience after undo = %+v, want the run's remove reverted", rec.Document.Experience)
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
