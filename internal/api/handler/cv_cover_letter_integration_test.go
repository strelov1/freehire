//go:build integration

// Integration tests for the cover-letter surface (cover-letter-draft): the single-row upsert
// with no history, the read path that never calls a model, ownership, and the allowance being
// released when the chain produces nothing.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// letterModel answers the three chain stages in order, so the fixture can drive a real draft
// end to end. Past its script it errors, which is how a test asserts that a stage did not run.
type letterModel struct {
	responses []string
	calls     int
}

func (m *letterModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	i := m.calls
	m.calls++
	if i >= len(m.responses) {
		return nil, errors.New("letterModel: unexpected extra call")
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: m.responses[i]}}}, nil
}

func (m *letterModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

// chainFor scripts select/draft/audit around one atom, with a body long enough to clear the
// floor so the audited letter is the one that survives.
func chainFor(atomID uuid.UUID, body string) *letterModel {
	return &letterModel{responses: []string{
		`{"selected":["` + atomID.String() + `"]}`,
		`{"body":"DRAFT ` + strings.Repeat("word ", 80) + `"}`,
		`{"body":"` + body + " " + strings.Repeat("word ", 80) + `","cited_atom_ids":["` + atomID.String() + `"]}`,
	}}
}

type coverLetterFixture struct {
	h        *cvHandlers
	app      *fiber.App
	token    string
	letters  *coverletter.Store
	bank     *experience.Store
	tailored cv.Meta
	base     cv.Meta
	userID   int64
	jobID    int64
}

func newCoverLetterFixture(t *testing.T, pool *pgxpool.Pool) coverLetterFixture {
	t.Helper()
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE cover_letters, cvs, jobs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	store := cv.NewStore(cv.NewQueriesRepository(queries))
	letters := coverletter.NewStore(coverletter.NewQueriesRepository(queries))
	bank := experience.NewStore(experience.NewQueriesRepository(queries))

	h := &cvHandlers{
		queries: queries, jobReader: queries, cvStore: store,
		fit:    fitanalysis.New(queries, nil, matchanalysis.NewAnalyzer(nil)),
		resume: resume.New(nil, resume.NewQueriesRepository(queries)),
		// A nil chain is the unconfigured-LLM deployment: Draft returns (nil, nil), which is
		// exactly the "produced nothing" path the release rule is about. Tests that want a
		// real draft replace the chain.
		letter: coverLetterDeps{letters: letters, chain: coverletter.NewAnalyzer(nil), bank: bank},
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{
		cookie: auth.RequireAuth(iss, testVersions),
		key:    auth.RequireAuth(iss, testVersions),
	})

	userID := seedAccount(t, pool, "coverletter@example.test", false)
	token, _ := iss.Issue(userID, testTokenVersion)
	jobID := seedJobWithSkills(t, pool, "backend-kafka-letter", []string{"go", "kafka"})

	ctx := context.Background()
	base, err := store.Create(ctx, userID, "Base", "classic-ats", cv.Document{
		Header: cv.Header{FullName: "Jane Roe", Email: "jane@example.com"},
	})
	if err != nil {
		t.Fatalf("create base cv: %v", err)
	}
	tailored, err := store.CreateTailored(ctx, userID, jobID, "Tailored", "classic-ats", cv.Document{
		Header: cv.Header{FullName: "Jane Roe", Email: "jane@example.com"},
	})
	if err != nil {
		t.Fatalf("create tailored cv: %v", err)
	}
	return coverLetterFixture{h: h, app: app, token: token, letters: letters, bank: bank,
		tailored: tailored, base: base, userID: userID, jobID: jobID}
}

func (f coverLetterFixture) get(t *testing.T, id string) (coverLetterResponse, int) {
	t.Helper()
	resp := doCV(t, f.app, fiber.MethodGet, "/api/v1/me/cvs/"+id+"/cover-letter", f.token, nil)
	defer resp.Body.Close()
	var body struct {
		Data coverLetterResponse `json:"data"`
	}
	if resp.StatusCode == fiber.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return body.Data, resp.StatusCode
}

func (f coverLetterFixture) post(t *testing.T, id string) (coverLetterResponse, int) {
	t.Helper()
	resp := doCV(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/"+id+"/cover-letter", f.token, nil)
	defer resp.Body.Close()
	var body struct {
		Data coverLetterResponse `json:"data"`
	}
	if resp.StatusCode == fiber.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return body.Data, resp.StatusCode
}

// Task 4.3: the table holds ONE row per pair. A redraft overwrites; no history survives.
func TestCoverLetter_SecondDraftReplacesTheFirst(t *testing.T) {
	pool := startPostgres(t)
	f := newCoverLetterFixture(t, pool)
	ctx := context.Background()

	first := coverletter.Letter{Body: "first body", Language: "de", Cited: []uuid.UUID{uuid.New()}}
	if err := f.letters.Save(ctx, f.userID, f.jobID, first, "model-a"); err != nil {
		t.Fatalf("save first: %v", err)
	}
	second := coverletter.Letter{Body: "second body", Language: "en"}
	if err := f.letters.Save(ctx, f.userID, f.jobID, second, "model-b"); err != nil {
		t.Fatalf("save second: %v", err)
	}

	var rows int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM cover_letters WHERE user_id = $1 AND job_id = $2",
		f.userID, f.jobID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("rows = %d, want exactly 1 — a letter keeps no history", rows)
	}

	stored, err := f.letters.Get(ctx, f.userID, f.jobID)
	if err != nil || stored == nil {
		t.Fatalf("get: (%v, %v)", stored, err)
	}
	if stored.Body != "second body" || stored.Model != "model-b" || stored.Language != "en" {
		t.Errorf("stored = %+v, want the second draft with its own stamps", stored.Letter)
	}
	if len(stored.Cited) != 0 {
		t.Errorf("Cited = %v, want the second draft's empty list, not the first's", stored.Cited)
	}
}

// created_at records the FIRST draft of a pair; only updated_at moves on a redraft. Nothing
// meters on it today, but a row that re-ages itself would silently break any rule that did.
func TestCoverLetter_RedraftKeepsTheOriginalCreatedAt(t *testing.T) {
	pool := startPostgres(t)
	f := newCoverLetterFixture(t, pool)
	ctx := context.Background()

	if err := f.letters.Save(ctx, f.userID, f.jobID, coverletter.Letter{Body: "one", Language: "en"}, "m"); err != nil {
		t.Fatalf("save: %v", err)
	}
	first, err := f.letters.Get(ctx, f.userID, f.jobID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := f.letters.Save(ctx, f.userID, f.jobID, coverletter.Letter{Body: "two", Language: "en"}, "m"); err != nil {
		t.Fatalf("resave: %v", err)
	}
	second, err := f.letters.Get(ctx, f.userID, f.jobID)
	if err != nil {
		t.Fatalf("get again: %v", err)
	}

	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("created_at moved from %v to %v; it records the first draft", first.CreatedAt, second.CreatedAt)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Errorf("updated_at did not move: %v then %v", first.UpdatedAt, second.UpdatedAt)
	}
}

// Task 5.1: the read path serves what is stored and never calls a model — which is why it
// takes no allowance.
func TestCoverLetter_ReadServesTheStoredDraft(t *testing.T) {
	pool := startPostgres(t)
	f := newCoverLetterFixture(t, pool)

	got, status := f.get(t, f.tailored.ID.String())
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got.Present {
		t.Error("present = true for a pair never drafted; an absent draft is an empty state, not an error")
	}

	letter := coverletter.Letter{Body: "stored body", Language: "en"}
	if err := f.letters.Save(context.Background(), f.userID, f.jobID, letter, ""); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, status = f.get(t, f.tailored.ID.String())
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !got.Present || got.Letter == nil || got.Letter.Body != "stored body" {
		t.Errorf("got %+v, want the stored letter", got)
	}
}

// A base CV is bound to no vacancy, so there is nothing to write to. The caller cannot act on
// the wrong reason, which is why it is its own message.
func TestCoverLetter_BaseCVIsRefused(t *testing.T) {
	pool := startPostgres(t)
	f := newCoverLetterFixture(t, pool)

	if _, status := f.get(t, f.base.ID.String()); status != fiber.StatusConflict {
		t.Errorf("status = %d, want 409 for a CV with no vacancy", status)
	}
}

// Ownership is a WHERE clause all the way down: another account's CV is missing, never
// forbidden.
func TestCoverLetter_ForeignCVIsNotFound(t *testing.T) {
	pool := startPostgres(t)
	f := newCoverLetterFixture(t, pool)

	other := seedAccount(t, pool, "someone-else@example.test", false)
	iss := auth.NewIssuer("test-secret", time.Hour)
	otherToken, _ := iss.Issue(other, testTokenVersion)

	resp := doCV(t, f.app, fiber.MethodGet, "/api/v1/me/cvs/"+f.tailored.ID.String()+"/cover-letter", otherToken, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404 — a foreign id is missing, not forbidden", resp.StatusCode)
	}
}

// Task 5.3 + 9.2: a chain that produces nothing must leave the stored draft untouched AND give
// the allowance back. A candidate must not pay for a letter they did not get.
func TestCoverLetter_UnproducedDraftLeavesTheStoreAndTheAllowanceAlone(t *testing.T) {
	pool := startPostgres(t)
	f := newCoverLetterFixture(t, pool)
	ctx := context.Background()

	existing := coverletter.Letter{Body: "the letter they already have", Language: "en"}
	if err := f.letters.Save(ctx, f.userID, f.jobID, existing, "m"); err != nil {
		t.Fatalf("save: %v", err)
	}
	// The chain reads a tailoring context, which needs a cached analysis. Without one the
	// request refuses at 409 long before the allowance is touched, and the test would pass
	// while proving nothing about the release.
	seedAnalysis(t, f.h, f.userID, f.jobID)
	f.h.plans = plan.NewStore(db.New(pool), pool, plan.DefaultConfig())

	// The fixture's analyzer has no client, so Draft returns (nil, nil): the unconfigured
	// deployment, and the shape of every "produced nothing" outcome.
	resp := doCV(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/"+f.tailored.ID.String()+"/cover-letter", f.token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when nothing can be drafted", resp.StatusCode)
	}

	stored, err := f.letters.Get(ctx, f.userID, f.jobID)
	if err != nil || stored == nil {
		t.Fatalf("get: (%v, %v)", stored, err)
	}
	if stored.Body != existing.Body {
		t.Errorf("stored body = %q, want the untouched %q", stored.Body, existing.Body)
	}

	st, err := f.h.plans.Standing(ctx, f.userID, plan.FeatureCoverLetter)
	if err != nil {
		t.Fatalf("standing: %v", err)
	}
	if st.Used != 0 {
		t.Errorf("used = %d, want 0 — the allowance was released when nothing was produced", st.Used)
	}
}

// Task 5.2, the happy path the earlier fixture could not reach: a real chain runs, the letter
// is stored, and the response carries its evidence with the claims already resolved — the part
// the workspace renders and the part a candidate actually checks.
func TestCoverLetter_DraftsStoresAndResolvesItsEvidence(t *testing.T) {
	pool := startPostgres(t)
	f := newCoverLetterFixture(t, pool)
	ctx := context.Background()

	atom, err := f.bank.AddAtom(ctx, f.userID, experience.Atom{
		Claim: "cut p95 latency from 800ms to 120ms",
	}, experience.AuthorCandidate)
	if err != nil {
		t.Fatalf("bank atom: %v", err)
	}
	seedAnalysis(t, f.h, f.userID, f.jobID)
	model := chainFor(atom.ID, "AUDITED")
	f.h.letter.chain = coverletter.NewAnalyzer(llm.NewWithModel(model))
	f.h.llm = llmBinding{client: llm.NewWithModel(model)}

	got, status := f.post(t, f.tailored.ID.String())
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if model.calls != 3 {
		t.Errorf("model called %d times, want 3 — select, draft, audit", model.calls)
	}
	if got.Letter == nil || !strings.HasPrefix(got.Letter.Body, "AUDITED") {
		t.Fatalf("body = %+v, want the audited letter", got.Letter)
	}
	if len(got.Cited) != 1 || got.Cited[0].ID != atom.ID.String() {
		t.Fatalf("Cited = %+v, want the one banked atom", got.Cited)
	}
	if got.Cited[0].Claim != "cut p95 latency from 800ms to 120ms" {
		t.Errorf("claim = %q, want it resolved server-side — an id alone is not checkable", got.Cited[0].Claim)
	}

	stored, err := f.letters.Get(ctx, f.userID, f.jobID)
	if err != nil || stored == nil {
		t.Fatalf("stored: (%v, %v)", stored, err)
	}
	if !strings.HasPrefix(stored.Body, "AUDITED") {
		t.Errorf("stored body = %q, want the audited letter", stored.Body)
	}
}

// A candidate whose bank holds nothing they asserted gets a refusal and no model call at all —
// the chain does not run, because a letter written without evidence is what this feature
// exists not to produce.
func TestCoverLetter_RefusesWhenNothingIsPublishable(t *testing.T) {
	pool := startPostgres(t)
	f := newCoverLetterFixture(t, pool)
	ctx := context.Background()

	if _, err := f.bank.AddAtom(ctx, f.userID, experience.Atom{Claim: "a model's own reading"},
		experience.AuthorAgent); err != nil {
		t.Fatalf("bank atom: %v", err)
	}
	seedAnalysis(t, f.h, f.userID, f.jobID)
	model := &letterModel{}
	f.h.letter.chain = coverletter.NewAnalyzer(llm.NewWithModel(model))
	f.h.llm = llmBinding{client: llm.NewWithModel(model)}

	resp := doCV(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/"+f.tailored.ID.String()+"/cover-letter", f.token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Errorf("status = %d, want 409 when no evidence is the candidate's own", resp.StatusCode)
	}
	if model.calls != 0 {
		t.Errorf("model called %d times, want 0", model.calls)
	}
}

// Task 9.2: an exhausted allowance refuses the write with a 402, and the same account can
// still READ its stored draft — the read calls no model, so nothing entitles it to refuse.
func TestCoverLetter_ExhaustedAllowanceRefusesTheWriteButNotTheRead(t *testing.T) {
	pool := startPostgres(t)
	f := newCoverLetterFixture(t, pool)
	ctx := context.Background()

	if err := f.letters.Save(ctx, f.userID, f.jobID,
		coverletter.Letter{Body: "already written", Language: "en"}, "m"); err != nil {
		t.Fatalf("save: %v", err)
	}
	seedAnalysis(t, f.h, f.userID, f.jobID)
	// Enforcement ships OFF per feature, so the refusal has to be turned on to be tested at
	// all — which is itself worth pinning: a shadow-mode deployment never refuses.
	f.h.plans = plan.NewStore(db.New(pool), pool, plan.DefaultConfig().WithFreeDaily(plan.FeatureCoverLetter, 0).Enforcing())
	model := chainFor(uuid.New(), "NEVER")
	f.h.letter.chain = coverletter.NewAnalyzer(llm.NewWithModel(model))
	f.h.llm = llmBinding{client: llm.NewWithModel(model)}

	resp := doCV(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/"+f.tailored.ID.String()+"/cover-letter", f.token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402 for an exhausted allowance", resp.StatusCode)
	}
	if model.calls != 0 {
		t.Errorf("model called %d times, want 0 — the refusal precedes the chain", model.calls)
	}

	got, status := f.get(t, f.tailored.ID.String())
	if status != fiber.StatusOK || !got.Present || got.Letter.Body != "already written" {
		t.Errorf("read after refusal = (%d, %+v), want the stored letter served", status, got)
	}
}
