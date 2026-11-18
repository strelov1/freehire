//go:build integration

package handler

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/tmc/langchaingo/llms"
)

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
	},
		experience.AuthorQuoted,
	)
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
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
