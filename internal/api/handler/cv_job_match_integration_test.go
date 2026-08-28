//go:build integration

// Integration tests for the tailoring job-match score (tailor-job-match): owner isolation,
// the two no-vacancy conflicts, the degrade-not-fail path when no renderer is configured,
// and the score following the document rather than any cached analysis.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvmatch"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The tailored CV's rendered text: a clean CV naming Go and Kafka but not Terraform, under
// a title that matches the seeded vacancy's.
const jobMatchCVText = "Jane Roe\njane@example.com  +1 415 555 0134\n\nSummary\nBackend Engineer. Core stack: Golang, Kafka, PostgreSQL.\n\nExperience\n2020-2024 Acme — Backend Engineer\n- Built Go services handling 2M requests/day\n- Ran Kafka pipelines for 4 teams\n\nEducation\n2016-2020 BSc Computer Science\n\nSkills\nGolang, Kafka, PostgreSQL"

type jobMatchFixture struct {
	h        *cvHandlers
	app      *fiber.App
	token    string
	renderer *fakeCVRenderer
	base     cv.Meta
	tailored cv.Meta
	store    *cv.Store
	userID   int64
	jobID    int64
}

func newJobMatchFixture(t *testing.T, pool *pgxpool.Pool) jobMatchFixture {
	t.Helper()
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, jobs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	store := cv.NewStore(cv.NewQueriesRepository(queries))
	renderer := &fakeCVRenderer{pdf: []byte(jobMatchCVText)}
	h := &cvHandlers{
		queries: queries, jobReader: queries, cvStore: store,
		fit:            fitanalysis.New(queries, nil, matchanalysis.NewAnalyzer(nil)),
		resume:         resume.New(nil, resume.NewQueriesRepository(queries)),
		cvRenderer:     renderer,
		extractPDFText: textFromPDF,
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{
		cookie: auth.RequireAuth(iss, testVersions),
		key:    auth.RequireAuth(iss, testVersions),
	})

	userID := seedAccount(t, pool, "jobmatch@example.test", false)
	token, _ := iss.Issue(userID, testTokenVersion)
	jobID := seedJobWithSkills(t, pool, "backend-kafka-match", []string{"go", "kafka", "terraform"})

	ctx := context.Background()
	base, err := store.Create(ctx, userID, "Base", "classic-ats", cv.Document{
		Header:  cv.Header{FullName: "Jane Roe", Email: "jane@example.com"},
		Summary: "base summary",
	})
	if err != nil {
		t.Fatalf("create base cv: %v", err)
	}
	tailored, err := store.CreateTailored(ctx, userID, jobID, "Tailored", "classic-ats", cv.Document{
		Header:  cv.Header{FullName: "Jane Roe", Email: "jane@example.com"},
		Summary: "tailored summary",
	})
	if err != nil {
		t.Fatalf("create tailored cv: %v", err)
	}
	return jobMatchFixture{h: h, app: app, token: token, renderer: renderer,
		base: base, tailored: tailored, store: store, userID: userID, jobID: jobID}
}

func (f jobMatchFixture) get(t *testing.T, id string) (cvJobMatchResponse, int) {
	t.Helper()
	resp := doCV(t, f.app, fiber.MethodGet, "/api/v1/me/cvs/"+id+"/job-match", f.token, nil)
	defer resp.Body.Close()
	var body struct {
		Data cvJobMatchResponse `json:"data"`
	}
	if resp.StatusCode == fiber.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return body.Data, resp.StatusCode
}

func (f jobMatchFixture) getErr(t *testing.T, id string) (string, int) {
	t.Helper()
	resp := doCV(t, f.app, fiber.MethodGet, "/api/v1/me/cvs/"+id+"/job-match", f.token, nil)
	defer resp.Body.Close()
	var body struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return body.Error, resp.StatusCode
}

// The score is served against the vacancy alone, with every category carrying the weight it
// was scored out of.
func TestJobMatch_ScoresTheTailoredCopyAgainstItsVacancy(t *testing.T) {
	pool := startPostgres(t)
	f := newJobMatchFixture(t, pool)

	got, status := f.get(t, f.tailored.ID.String())

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !got.Available || got.Score == nil {
		t.Fatalf("score unavailable: %q", got.Reason)
	}
	if got.Score.Overall <= 0 {
		t.Errorf("overall = %d, want a positive score", got.Score.Overall)
	}
	// The vacancy names Terraform; this CV does not.
	if len(got.Score.MissingSkills) != 1 || got.Score.MissingSkills[0] != "terraform" {
		t.Errorf("missing skills = %v, want [terraform]", got.Score.MissingSkills)
	}
	for _, c := range got.Score.Categories {
		if c.Weight <= 0 {
			t.Errorf("category %q carries weight %d; the client renders impact from it", c.ID, c.Weight)
		}
	}
}

// The base CV renders exactly zero times: this score reads one document, and that halving is
// what pays for refreshing it on every save.
func TestJobMatch_RendersTheTailoredCopyOnce(t *testing.T) {
	pool := startPostgres(t)
	f := newJobMatchFixture(t, pool)

	if _, status := f.get(t, f.tailored.ID.String()); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if f.renderer.calls != 1 {
		t.Errorf("render calls = %d, want exactly 1", f.renderer.calls)
	}
}

// With no fit analysis cached for the pair, Requirements Coverage reports itself unavailable
// and the remaining categories still produce a score.
func TestJobMatch_WithoutACachedAnalysisScoresTheOtherCategories(t *testing.T) {
	pool := startPostgres(t)
	f := newJobMatchFixture(t, pool)

	got, status := f.get(t, f.tailored.ID.String())

	if status != fiber.StatusOK || !got.Available {
		t.Fatalf("status = %d, available = %v, reason = %q", status, got.Available, got.Reason)
	}
	var found bool
	for _, c := range got.Score.Categories {
		if c.ID != cvmatch.CategoryRequirements {
			continue
		}
		found = true
		if c.Available {
			t.Error("requirements coverage must be unavailable with no cached analysis")
		}
		if c.Reason == "" {
			t.Error("an unavailable category must say why")
		}
	}
	if !found {
		t.Error("the response dropped the requirements category instead of reporting it unavailable")
	}
	if got.Score.Overall <= 0 {
		t.Errorf("overall = %d, want the other three categories to still score", got.Score.Overall)
	}
}

// The two ways a CV has no vacancy are told apart, so the owner is not handed the wrong
// explanation. A pruned vacancy leaves a tailored copy that is still a tailored copy.
func TestJobMatch_RefusalNamesWhichCaseItIs(t *testing.T) {
	pool := startPostgres(t)
	f := newJobMatchFixture(t, pool)

	msg, status := f.getErr(t, f.base.ID.String())
	if status != fiber.StatusConflict {
		t.Fatalf("base CV status = %d, want 409", status)
	}
	if !strings.Contains(msg, "base CV") {
		t.Errorf("base CV refusal = %q, want it to name the base case", msg)
	}

	if _, err := pool.Exec(context.Background(), "DELETE FROM jobs WHERE id = $1", f.jobID); err != nil {
		t.Fatalf("prune job: %v", err)
	}
	msg, status = f.getErr(t, f.tailored.ID.String())
	if status != fiber.StatusConflict {
		t.Fatalf("pruned-vacancy status = %d, want 409", status)
	}
	if !strings.Contains(msg, "no longer exists") {
		t.Errorf("pruned-vacancy refusal = %q, want it to name the pruned vacancy", msg)
	}
}

// A foreign CV is indistinguishable from one that does not exist.
func TestJobMatch_ForeignCVIsNotFound(t *testing.T) {
	pool := startPostgres(t)
	f := newJobMatchFixture(t, pool)

	iss := auth.NewIssuer("test-secret", time.Hour)
	otherID := seedAccount(t, pool, "stranger@example.test", false)
	otherToken, _ := iss.Issue(otherID, testTokenVersion)

	resp := doCV(t, f.app, fiber.MethodGet, "/api/v1/me/cvs/"+f.tailored.ID.String()+"/job-match", otherToken, nil)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// No renderer is an unavailable score with a success status, never a 500: the workspace has
// to keep loading and editing without it.
func TestJobMatch_WithoutARendererDegradesInsteadOfFailing(t *testing.T) {
	pool := startPostgres(t)
	f := newJobMatchFixture(t, pool)
	f.h.cvRenderer = nil

	got, status := f.get(t, f.tailored.ID.String())

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got.Available || got.Score != nil {
		t.Errorf("got %+v, want an unavailable score", got)
	}
	if got.Reason == "" {
		t.Error("an unavailable score must carry a reason")
	}
}

// Editing the document moves the score on the next read, with nothing to invalidate.
func TestJobMatch_FollowsTheDocumentOnTheNextRead(t *testing.T) {
	pool := startPostgres(t)
	f := newJobMatchFixture(t, pool)

	before, _ := f.get(t, f.tailored.ID.String())

	// The candidate adds the vacancy's remaining skill; the renderer's text layer follows.
	f.renderer.pdf = []byte(jobMatchCVText + "\n- Provisioned infrastructure with Terraform")

	after, _ := f.get(t, f.tailored.ID.String())

	if before.Score == nil || after.Score == nil {
		t.Fatal("expected a score on both reads")
	}
	if after.Score.Overall <= before.Score.Overall {
		t.Errorf("score went %d → %d; adding a required skill must raise it",
			before.Score.Overall, after.Score.Overall)
	}
	if len(after.Score.MissingSkills) != 0 {
		t.Errorf("missing skills = %v, want none once Terraform is stated", after.Score.MissingSkills)
	}
}
