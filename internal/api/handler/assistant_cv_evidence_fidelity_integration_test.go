//go:build integration

// Integration tests for check_evidence_fidelity inside a real tailoring turn (see the
// tailor-evidence-fidelity-check change): the tool hands the agent back the real text of an
// atom it already cited, and a scripted turn can act on that to revise its own overstated
// wording — the class of problem the evidence-citation gate alone does not catch, since a
// citation can be real and still say more than the atom does.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// The shape of the fix this whole change exists to enable: the agent writes a bullet that
// overstates a real, cited atom, calls check_evidence_fidelity, and revises the SAME bullet
// once it has the atom's own words back in front of it.
func TestAutopilotRevisesAnOverstatedBulletAfterFidelityCheck(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	ctx := context.Background()

	model := &scriptedTurnModel{}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "fidelity-revise@example.test", true)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)

	// The candidate mentioned AWS on a skills list — real evidence, but thin. A bullet
	// claiming to have "deployed and operated" services on it says more than this atom does.
	atom, err := h.experience.AddAtom(ctx, userID, experience.Atom{
		Claim:      "Listed AWS as a familiar technology",
		Provenance: experience.ProvenanceStatedInChat,
	},
		experience.AuthorQuoted,
	)
	if err != nil {
		t.Fatalf("seed experience atom: %v", err)
	}

	model.replies = []*llms.ContentChoice{
		callReplyChoice("cv_edit", `{"ops":[{"kind":"insert","path":"experience[0].bullets[0]",`+
			`"value":"Deployed and operated production services on AWS at scale.","evidence_id":"`+atom.ID.String()+`"}]}`),
		callReplyChoice("check_evidence_fidelity", `{"evidence_id":"`+atom.ID.String()+`"}`),
		callReplyChoice("cv_edit", `{"ops":[{"kind":"set","path":"experience[0].bullets[0]",`+
			`"value":"Familiar with AWS.","evidence_id":"`+atom.ID.String()+`"}]}`),
		{Content: "Toned that bullet down to what you actually told me."},
	}

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

	// The tool actually ran and handed back the atom's own words — not a stub result the
	// model could have produced on its own.
	var sawAtomText bool
	for _, result := range model.seen {
		if strings.Contains(result, atom.Claim) {
			sawAtomText = true
		}
	}
	if !sawAtomText {
		t.Errorf("check_evidence_fidelity's result never carried the atom's own claim; tool results were %q", model.seen)
	}

	rec, err := h.cv.cvStore.Get(ctx, cvID, userID)
	if err != nil {
		t.Fatalf("get cv: %v", err)
	}
	if len(rec.Document.Experience) == 0 || len(rec.Document.Experience[0].Bullets) != 1 {
		t.Fatalf("document = %+v, want exactly one bullet — the revision replaces, it does not add", rec.Document)
	}
	bullet := rec.Document.Experience[0].Bullets[0]
	if strings.Contains(bullet, "Deployed and operated") {
		t.Errorf("bullet = %q, the overstated wording is still on the page after a revision was made", bullet)
	}
	if bullet != "Familiar with AWS." {
		t.Errorf("bullet = %q, want the revised, evidence-faithful wording", bullet)
	}

	// Both cv_edit calls actually landed as revisions — the fidelity check itself writes
	// nothing, but the edit it prompted must be as durable and undoable as any other.
	var feed struct {
		Data []cvedit.RevisionView `json:"data"`
	}
	decodeJSON(t, assistantRequest(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+cvID.String()+"/revisions", cookie, nil), &feed)
	if len(feed.Data) != 2 {
		t.Fatalf("revision feed holds %d entries, want 2 (the overstated insert, then its revision): %+v", len(feed.Data), feed.Data)
	}
	batch := feed.Data[0].BatchID
	for _, rev := range feed.Data {
		if rev.Origin != string(cvedit.OriginTailorAgent) {
			t.Errorf("revision %+v has origin %q, want %q", rev, rev.Origin, cvedit.OriginTailorAgent)
		}
		if rev.BatchID == "" || rev.BatchID != batch {
			t.Errorf("revision %+v does not share the run's batch id %q — both edits happened in the same turn", rev, batch)
		}
	}
}

// A bad evidence_id must fail the same way it fails everywhere else this tool surface cites
// one — named, so the model can correct itself — and must leave the CV exactly as it found it,
// since the tool reads and never writes.
func TestFidelityCheckRefusesAnUnresolvedEvidenceId(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	ctx := context.Background()

	model := &scriptedTurnModel{}
	app, h := newAssistantApp(pool, iss, model)
	userID, cookie := assistantUser(t, pool, iss, "fidelity-unresolved@example.test", true)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)

	missing := uuid.New()
	model.replies = []*llms.ContentChoice{
		callReplyChoice("check_evidence_fidelity", `{"evidence_id":"`+missing.String()+`"}`),
		{Content: "Could not find that citation, let me look again."},
	}

	const seeded = `{"summary":"before the run","experience":[{"company":"Acme","title":"Engineer","bullets":[]}]}`
	if _, err := pool.Exec(ctx, `UPDATE cvs SET data = $2::jsonb WHERE id = $1`, cvID, seeded); err != nil {
		t.Fatalf("seed cv document: %v", err)
	}
	var before []byte
	if err := pool.QueryRow(ctx, `SELECT data FROM cvs WHERE id = $1`, cvID).Scan(&before); err != nil {
		t.Fatalf("read seeded document: %v", err)
	}

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read stream: %v", err)
	}

	var refused bool
	for _, result := range model.seen {
		if strings.Contains(result, "no banked achievement") && strings.Contains(result, missing.String()) {
			refused = true
		}
	}
	if !refused {
		t.Errorf("an unresolved evidence_id was not named in the refusal; tool results were %q", model.seen)
	}

	// Byte-for-byte against what was seeded — not just the bullets — so a stray write to
	// any other field would fail this too, not only an unscripted cv_edit that never happened.
	var after []byte
	if err := pool.QueryRow(ctx, `SELECT data FROM cvs WHERE id = $1`, cvID).Scan(&after); err != nil {
		t.Fatalf("read document after the run: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("document changed from %s to %s — the fidelity check must never write", before, after)
	}

	// No revision either: a read-only tool call must not create an entry in the CV's history.
	var feed struct {
		Data []cvedit.RevisionView `json:"data"`
	}
	decodeJSON(t, assistantRequest(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+cvID.String()+"/revisions", cookie, nil), &feed)
	if len(feed.Data) != 0 {
		t.Errorf("revision feed holds %d entries, want 0 — a refused lookup must leave no trace: %+v", len(feed.Data), feed.Data)
	}
}
