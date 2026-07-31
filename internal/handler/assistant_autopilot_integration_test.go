//go:build integration

// Integration tests for the autopilot run endpoint (see the tailor-autopilot change): it
// runs only on a tailoring session bound to a CV, it takes the pre-run snapshot itself, and
// the brief and the turn's ceiling are the server's rather than the caller's.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/llm"
)

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
