//go:build integration

// Integration tests for the base-CV surface: the bases list, duplicate, choosing a base when
// tailoring, and the rule that an upload never overwrites a base the candidate edited by hand.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// buildBasesApp registers the literal /bases route BEFORE /:id, the order Register uses —
// reversed, Fiber reads "bases" as a CV uuid.
func buildBasesApp(h *cvHandlers, iss *auth.Issuer) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	saved := auth.RequireAuth(iss, testVersions)
	app.Get("/api/v1/me/cvs/bases", saved, cvContract(h))
	app.Post("/api/v1/me/cvs/tailor", saved, markPaidAI(plan.FeatureTailor), cvContract(h))
	app.Post("/api/v1/me/cvs/:id/duplicate", saved, cvContract(h))
	// No paidAI middleware, as in Register: opening a base conversation costs nothing.
	app.Post("/api/v1/me/cvs/:id/base-session", saved, cvContract(h))
	app.Post("/api/v1/me/cvs/:id/tailor-session", saved, markPaidAI(plan.FeatureTailor), cvContract(h))
	return app
}

type basesFixture struct {
	h     *cvHandlers
	iss   *auth.Issuer
	pool  *pgxpool.Pool
	app   *fiber.App
	user  int64
	tok   string
	first cv.Meta
	other cv.Meta
}

// newBasesFixture gives a signed-in subscriber two bases ("First" then "Second") and two
// vacancies, bases-job-1 and bases-job-2.
func newBasesFixture(t *testing.T) *basesFixture {
	t.Helper()
	h, iss, pool := newTailorAPI(t)
	// The shared fixture allows one new tailoring session a day; these tests tailor several
	// vacancies in a run.
	h.plans = plan.NewStore(h.queries, pool, plan.DefaultConfig().Enforcing().WithProCeiling(plan.FeatureTailor, 10))
	user := seedSubscriber(t, pool, "bases-"+uuid.NewString()[:6]+"@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)
	ctx := context.Background()
	first, err := h.cvStore.Create(ctx, user, "First", cv.DefaultTemplateID, cv.Document{Summary: "one"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := h.cvStore.Create(ctx, user, "Second", cv.DefaultTemplateID, cv.Document{Summary: "two"})
	if err != nil {
		t.Fatal(err)
	}
	seedJobSlug(t, pool, "bases-job-1")
	seedJobSlug(t, pool, "bases-job-2")
	return &basesFixture{h: h, iss: iss, pool: pool, app: buildBasesApp(h, iss), user: user, tok: tok, first: first, other: other}
}

func (f *basesFixture) tailor(t *testing.T, slug string, req tailorCVRequest) (int, tailorCVResponse) {
	t.Helper()
	req.JobSlug = slug
	resp := doCV(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/tailor", f.tok, req)
	defer resp.Body.Close()
	var out struct {
		Data tailorCVResponse `json:"data"`
	}
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out.Data
}

func TestListBaseCVs_FlagsTheDefaultAndSkipsTailoredCopies(t *testing.T) {
	f := newBasesFixture(t)
	if code, _ := f.tailor(t, "bases-job-1", tailorCVRequest{BaseCVID: f.other.ID.String()}); code != fiber.StatusCreated {
		t.Fatalf("tailor = %d", code)
	}
	resp := doCV(t, f.app, fiber.MethodGet, "/api/v1/me/cvs/bases", f.tok, nil)
	defer resp.Body.Close()
	var out struct {
		Data []cvBaseResponse `json:"data"`
	}
	decodeJSON(t, resp, &out)
	if len(out.Data) != 2 {
		t.Fatalf("bases = %+v, want the two bases and no tailored copy", out.Data)
	}
	if out.Data[0].ID != f.first.ID.String() || out.Data[0].IsDefault || out.Data[1].ID != f.other.ID.String() || !out.Data[1].IsDefault {
		t.Fatalf("bases = %+v, want [first, second(default: last used)]", out.Data)
	}
}

func TestListBaseCVs_EmptyForAUserWithNoBase(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	user := seedSubscriber(t, pool, "nobases@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)
	resp := doCV(t, buildBasesApp(h, iss), fiber.MethodGet, "/api/v1/me/cvs/bases", tok, nil)
	defer resp.Body.Close()
	var out struct {
		Data []cvBaseResponse `json:"data"`
	}
	decodeJSON(t, resp, &out)
	if out.Data == nil || len(out.Data) != 0 {
		t.Fatalf("data = %#v, want an empty JSON array", out.Data)
	}
}

func TestTailorCV_BaseChoice(t *testing.T) {
	f := newBasesFixture(t)

	code, got := f.tailor(t, "bases-job-1", tailorCVRequest{BaseCVID: f.other.ID.String()})
	if code != fiber.StatusCreated || got.BaseCVID != f.other.ID.String() {
		t.Fatalf("tailor from second: code=%d base=%q, want 201 and the chosen base", code, got.BaseCVID)
	}
	copyRec, _ := f.h.cvStore.Get(context.Background(), uuid.MustParse(got.TailorCVID), f.user)
	if copyRec.BaseCVID != f.other.ID || copyRec.Document.Summary != "two" {
		t.Fatalf("copy = base %s summary %q, want base %s and its document", copyRec.BaseCVID, copyRec.Document.Summary, f.other.ID)
	}

	// A repeat tailor of the same vacancy reopens the same copy whatever base is named.
	code, again := f.tailor(t, "bases-job-1", tailorCVRequest{BaseCVID: f.first.ID.String()})
	if code != fiber.StatusCreated || again.TailorCVID != got.TailorCVID || again.BaseCVID != f.other.ID.String() {
		t.Fatalf("repeat = code %d copy %s base %s, want the same copy and its own base", code, again.TailorCVID, again.BaseCVID)
	}

	// No base_cv_id: the default base (now the one last used).
	if code, def := f.tailor(t, "bases-job-2", tailorCVRequest{}); code != fiber.StatusCreated || def.BaseCVID != f.other.ID.String() {
		t.Fatalf("default tailor = code %d base %s, want the last-used base", code, def.BaseCVID)
	}

	// Bad references.
	if code, _ := f.tailor(t, "bases-job-1", tailorCVRequest{BaseCVID: got.TailorCVID}); code != fiber.StatusConflict {
		t.Errorf("tailored copy as base = %d, want 409", code)
	}
	if code, _ := f.tailor(t, "bases-job-1", tailorCVRequest{BaseCVID: uuid.NewString()}); code != fiber.StatusNotFound {
		t.Errorf("unknown base = %d, want 404", code)
	}
	if code, _ := f.tailor(t, "bases-job-1", tailorCVRequest{BaseCVID: "not-a-uuid"}); code != fiber.StatusBadRequest {
		t.Errorf("malformed base = %d, want 400", code)
	}
}

func TestTailorCV_AnotherUsersBaseIs404(t *testing.T) {
	f := newBasesFixture(t)
	stranger := seedSubscriber(t, f.pool, "stranger@example.test", true)
	theirs, err := f.h.cvStore.Create(context.Background(), stranger, "Theirs", cv.DefaultTemplateID, cv.Document{})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := f.tailor(t, "bases-job-1", tailorCVRequest{BaseCVID: theirs.ID.String()}); code != fiber.StatusNotFound {
		t.Fatalf("foreign base = %d, want 404", code)
	}
}

func TestStartTailorSession_ReportsTheCopyOwnBase(t *testing.T) {
	f := newBasesFixture(t)
	_, got := f.tailor(t, "bases-job-1", tailorCVRequest{BaseCVID: f.other.ID.String()})
	// Make the OTHER base the default again by tailoring a second vacancy from the first.
	if code, _ := f.tailor(t, "bases-job-2", tailorCVRequest{BaseCVID: f.first.ID.String()}); code != fiber.StatusCreated {
		t.Fatal("second tailor failed")
	}
	resp := doCV(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/"+got.TailorCVID+"/tailor-session", f.tok, nil)
	defer resp.Body.Close()
	var out struct {
		Data tailorSessionResponse `json:"data"`
	}
	decodeJSON(t, resp, &out)
	if out.Data.BaseCVID != f.other.ID.String() {
		t.Fatalf("base = %s, want the copy's own base %s (not today's default)", out.Data.BaseCVID, f.other.ID)
	}
}

func TestDuplicateCV(t *testing.T) {
	f := newBasesFixture(t)
	resp := doCV(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/"+f.first.ID.String()+"/duplicate", f.tok, nil)
	if resp.StatusCode != fiber.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("duplicate = %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Data cvResponse `json:"data"`
	}
	decodeJSON(t, resp, &out)
	if out.Data.Title != "First (copy)" || out.Data.Document.Summary != "one" || out.Data.ID == f.first.ID.String() {
		t.Fatalf("copy = %+v", out.Data)
	}
	if def, _, _ := f.h.cvStore.BaseCV(context.Background(), f.user); def.ID != f.first.ID {
		t.Fatalf("duplicating moved the default to %s", def.ID)
	}

	// A tailored copy cannot be duplicated; a foreign or unknown id is a 404.
	_, tailored := f.tailor(t, "bases-job-1", tailorCVRequest{})
	if code := statusOf(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.TailorCVID+"/duplicate", f.tok); code != fiber.StatusConflict {
		t.Errorf("duplicate tailored = %d, want 409", code)
	}
	if code := statusOf(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/"+uuid.NewString()+"/duplicate", f.tok); code != fiber.StatusNotFound {
		t.Errorf("duplicate unknown = %d, want 404", code)
	}
	stranger := seedSubscriber(t, f.pool, "stranger2@example.test", true)
	sTok, _ := f.iss.Issue(stranger, testTokenVersion)
	if code := statusOf(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/"+f.first.ID.String()+"/duplicate", sTok); code != fiber.StatusNotFound {
		t.Errorf("duplicate foreign = %d, want 404", code)
	}
}

// An upload newer than the base refreshes an UNEDITED base on the next tailor, and leaves one
// the candidate edited by hand alone.
func TestTailorCV_StaleRefreshSkipsAHandEditedBase(t *testing.T) {
	f := newBasesFixture(t)
	ctx := context.Background()
	oldAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	newBlob, _ := json.Marshal(resumeextract.Structured{FullName: "Ada Lovelace", Summary: "after upload", Skills: []string{"Go"}})
	if _, err := f.pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = now(),
		 resume_structured = $2, resume_structured_uploaded_at = now(), resume_structured_model = 'test'
		 WHERE id = $1`, f.user, newBlob); err != nil {
		t.Fatal(err)
	}
	// Both bases predate the upload; only "Second" carries a hand edit.
	if _, err := f.pool.Exec(ctx, `UPDATE cvs SET updated_at = $1 WHERE user_id = $2`, oldAt, f.user); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE cvs SET hand_edited = true WHERE id = $1`, f.other.ID); err != nil {
		t.Fatal(err)
	}

	if code, _ := f.tailor(t, "bases-job-1", tailorCVRequest{BaseCVID: f.other.ID.String()}); code != fiber.StatusCreated {
		t.Fatalf("tailor hand-edited = %d", code)
	}
	if rec, _ := f.h.cvStore.Get(ctx, f.other.ID, f.user); rec.Document.Summary != "two" {
		t.Fatalf("hand-edited base summary = %q, want it left alone", rec.Document.Summary)
	}

	if code, _ := f.tailor(t, "bases-job-2", tailorCVRequest{BaseCVID: f.first.ID.String()}); code != fiber.StatusCreated {
		t.Fatalf("tailor unedited = %d", code)
	}
	if rec, _ := f.h.cvStore.Get(ctx, f.first.ID, f.user); rec.Document.Summary != "after upload" {
		t.Fatalf("unedited base summary = %q, want refreshed from the upload", rec.Document.Summary)
	}
}

// statusOf issues a request and returns only its status, closing the body.
func statusOf(t *testing.T, app *fiber.App, method, path, token string) int {
	t.Helper()
	resp := doCV(t, app, method, path, token, nil)
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestStartBaseSession(t *testing.T) {
	f := newBasesFixture(t)
	ctx := context.Background()

	start := func(id string, tok string) (int, baseSessionResponse) {
		resp := doCV(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/"+id+"/base-session", tok, nil)
		defer resp.Body.Close()
		var out struct {
			Data baseSessionResponse `json:"data"`
		}
		b, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(b, &out)
		return resp.StatusCode, out.Data
	}

	code, first := start(f.first.ID.String(), f.tok)
	if code != fiber.StatusOK || first.SessionID == "" || first.BaseCVID != f.first.ID.String() {
		t.Fatalf("start = %d %+v", code, first)
	}
	sess, err := f.h.assistantSessions.Session(ctx, uuid.MustParse(first.SessionID), f.user)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Preset != assistant.PresetBase || sess.CVID == nil || *sess.CVID != f.first.ID || sess.JobID != nil {
		t.Fatalf("session = preset %q cv %v job %v, want a base session bound to the CV and no vacancy", sess.Preset, sess.CVID, sess.JobID)
	}
	rec, _ := f.h.cvStore.Get(ctx, f.first.ID, f.user)
	if rec.AgentSessionID != first.SessionID {
		t.Fatalf("the CV does not remember its conversation: %q", rec.AgentSessionID)
	}

	// Reopening resumes the same conversation instead of starting another.
	if _, again := start(f.first.ID.String(), f.tok); again.SessionID != first.SessionID {
		t.Errorf("second start = %s, want the same session %s", again.SessionID, first.SessionID)
	}
	// Each base has its own.
	if _, other := start(f.other.ID.String(), f.tok); other.SessionID == first.SessionID {
		t.Error("two bases share one conversation")
	}

	// A tailored copy has its own route; a foreign base is invisible.
	_, tailored := f.tailor(t, "bases-job-1", tailorCVRequest{})
	if code, _ := start(tailored.TailorCVID, f.tok); code != fiber.StatusConflict {
		t.Errorf("tailored copy = %d, want 409", code)
	}
	stranger := seedSubscriber(t, f.pool, "stranger3@example.test", true)
	sTok, _ := f.iss.Issue(stranger, testTokenVersion)
	if code, _ := start(f.first.ID.String(), sTok); code != fiber.StatusNotFound {
		t.Errorf("foreign base = %d, want 404", code)
	}
}

// An agent edit on a base counts as the candidate's own work: the automatic refresh after an
// upload must leave it alone.
func TestAgentEditMarksTheBaseHandEdited(t *testing.T) {
	f := newBasesFixture(t)
	ctx := context.Background()
	withSkills, err := f.h.cvStore.Create(ctx, f.user, "With skills", cv.DefaultTemplateID, cv.Document{
		Skills: []cv.SkillGroup{{Items: []string{"Go", "SQL"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	path, err := cvedit.ParsePath("skills[0].items[1]")
	if err != nil {
		t.Fatal(err)
	}
	to := 0
	// Reordering what is already on the page states nothing about the candidate, so the agent
	// may do it without an evidence_id.
	if _, _, err := f.h.editor.Commit(ctx, withSkills.ID, f.user, cvedit.Change{
		Actor: cvedit.ActorAgent, Origin: cvedit.OriginTailorAgent,
		Ops: []cvedit.Op{{Kind: cvedit.OpMove, Path: path, To: &to}},
	}); err != nil {
		t.Fatal(err)
	}
	rec, _ := f.h.cvStore.Get(ctx, withSkills.ID, f.user)
	if !rec.HandEdited {
		t.Fatal("an agent edit did not mark the base hand-edited")
	}
}
