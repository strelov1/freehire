//go:build integration

// Integration tests for the tailoring ATS delta (tailor-ats-delta): the comparison holds the
// template, margins and keyword baseline constant, owner isolation, the not-a-tailored-copy
// conflict, and the degrade-not-fail path when no renderer is configured.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resume"
)

// The base CV's rendered text: a clean CV that carries Go but not Kafka.
const baseCVText = "Jane Roe\njane@example.com  +1 415 555 0134\n\nSummary\nBackend engineer.\n\nExperience\n2020-2024 Acme — Senior Engineer\n- Built Go services\n\nEducation\n2016-2020 BSc Computer Science\n\nSkills\nGolang"

// The tailored CV's rendered text: the same CV with the vacancy's Kafka evidence added.
const tailoredCVText = "Jane Roe\njane@example.com  +1 415 555 0134\n\nSummary\nBackend engineer. Core stack: Golang, Kafka.\n\nExperience\n2020-2024 Acme — Senior Engineer\n- Built Go services handling 2M requests/day\n- Ran Kafka pipelines for 4 teams\n\nEducation\n2016-2020 BSc Computer Science\n\nSkills\nGolang, Kafka"

// seedJobWithSkills inserts a vacancy carrying canonical skills — the keyword baseline both
// sides of the delta are scored against.
func seedJobWithSkills(t *testing.T, pool *pgxpool.Pool, slug string, skills []string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, skills)
		 VALUES ('test', $1, 'https://e.test/'||$1, 'Backend Engineer', $1, $2) RETURNING id`,
		slug, skills).Scan(&id); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return id
}

// atsDeltaFixture stands up the CV surface with a base CV and one tailored copy bound to a
// vacancy. The base deliberately differs from the copy in BOTH template and margins, so a
// test can prove the comparison holds them constant.
type atsDeltaFixture struct {
	h        *cvHandlers
	app      *fiber.App
	token    string
	renderer *fakeCVRenderer
	base     cv.Meta
	tailored cv.Meta
	store    *cv.Store
	userID   int64
}

func newATSDeltaFixture(t *testing.T, pool *pgxpool.Pool) atsDeltaFixture {
	t.Helper()
	queries := db.New(pool)
	if _, err := pool.Exec(context.Background(), "TRUNCATE cvs, jobs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	store := cv.NewStore(cv.NewQueriesRepository(queries))
	renderer := &fakeCVRenderer{renderFn: func(doc cv.Document) []byte {
		// The document's summary decides which side this is, so the assertion never depends
		// on the order the handler renders them in.
		if strings.Contains(doc.Summary, "tailored") {
			return []byte(tailoredCVText)
		}
		return []byte(baseCVText)
	}}
	h := &cvHandlers{
		queries: queries, jobReader: queries, cvStore: store,
		resume:         resume.New(nil, resume.NewQueriesRepository(queries)),
		cvRenderer:     renderer,
		extractPDFText: textFromPDF,
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{
		cookie: auth.RequireAuth(iss, testVersions),
		key:    auth.RequireAuth(iss, testVersions),
	})

	userID := seedAccount(t, pool, "delta@example.test", false)
	token, _ := iss.Issue(userID, testTokenVersion)
	jobID := seedJobWithSkills(t, pool, "backend-kafka", []string{"go", "kafka", "terraform"})

	ctx := context.Background()
	base, err := store.Create(ctx, userID, "Base", "centered", cv.Document{
		Margins: cv.Margins{Top: 1.2, Right: 1.2, Bottom: 1.2, Left: 1.2},
		Header:  cv.Header{FullName: "Jane Roe", Email: "jane@example.com"},
		Summary: "base summary",
	})
	if err != nil {
		t.Fatalf("create base cv: %v", err)
	}
	tailored, err := store.CreateTailored(ctx, userID, jobID, "Tailored", "classic-ats", cv.Document{
		Margins: cv.Margins{Top: 0.4, Right: 0.4, Bottom: 0.4, Left: 0.4},
		Header:  cv.Header{FullName: "Jane Roe", Email: "jane@example.com"},
		Summary: "tailored summary",
	})
	if err != nil {
		t.Fatalf("create tailored cv: %v", err)
	}
	return atsDeltaFixture{h: h, app: app, token: token, renderer: renderer,
		base: base, tailored: tailored, store: store, userID: userID}
}

func (f atsDeltaFixture) get(t *testing.T, id string) (atsDeltaResponse, int) {
	t.Helper()
	resp := doCV(t, f.app, fiber.MethodGet, "/api/v1/me/cvs/"+id+"/ats-delta", f.token, nil)
	defer resp.Body.Close()
	var body struct {
		Data atsDeltaResponse `json:"data"`
	}
	if resp.StatusCode == fiber.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return body.Data, resp.StatusCode
}

func TestATSDelta_ComparesTheTailoredCopyAgainstTheBase(t *testing.T) {
	f := newATSDeltaFixture(t, startPostgres(t))

	got, status := f.get(t, f.tailored.ID.String())

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !got.Available || got.Delta == nil {
		t.Fatalf("available = %v delta = %v, want an available delta", got.Available, got.Delta)
	}
	if got.BaseCVID != f.base.ID.String() {
		t.Errorf("base_cv_id = %q, want the base CV %q", got.BaseCVID, f.base.ID)
	}
	if got.Delta.Change <= 0 {
		t.Errorf("change = %d, want positive: the tailored text adds the vacancy's Kafka evidence", got.Delta.Change)
	}
	if got.Delta.Regressed {
		t.Error("regressed = true, want false when the tailored score rose")
	}
	if len(got.Delta.Categories) == 0 {
		t.Error("categories = empty, want the scorer's categories")
	}
}

func TestATSDelta_RendersBothSidesWithTheTailoredCopysTemplateAndMargins(t *testing.T) {
	f := newATSDeltaFixture(t, startPostgres(t))

	if _, status := f.get(t, f.tailored.ID.String()); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if f.renderer.calls != 2 {
		t.Fatalf("render calls = %d, want 2 (base + tailored)", f.renderer.calls)
	}
	wantMargins := cv.Margins{Top: 0.4, Right: 0.4, Bottom: 0.4, Left: 0.4}
	for i, doc := range f.renderer.docsSeen {
		if doc.Margins != wantMargins {
			t.Errorf("render %d margins = %+v, want the tailored copy's %+v", i, doc.Margins, wantMargins)
		}
	}
	for i, tmpl := range f.renderer.tmplsSeen {
		if tmpl.ID != "classic-ats" {
			t.Errorf("render %d template = %q, want the tailored copy's classic-ats", i, tmpl.ID)
		}
	}
}

func TestATSDelta_LeavesTheBaseCVUntouched(t *testing.T) {
	f := newATSDeltaFixture(t, startPostgres(t))
	ctx := context.Background()
	before, err := f.store.Get(ctx, f.base.ID, f.userID)
	if err != nil {
		t.Fatalf("read base before: %v", err)
	}

	if _, status := f.get(t, f.tailored.ID.String()); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	after, err := f.store.Get(ctx, f.base.ID, f.userID)
	if err != nil {
		t.Fatalf("read base after: %v", err)
	}
	if after.TemplateID != before.TemplateID {
		t.Errorf("base template = %q, want it unchanged at %q", after.TemplateID, before.TemplateID)
	}
	if after.Document.Margins != before.Document.Margins {
		t.Errorf("base margins = %+v, want them unchanged at %+v", after.Document.Margins, before.Document.Margins)
	}
	if after.Document.Summary != before.Document.Summary {
		t.Errorf("base summary = %q, want it unchanged at %q", after.Document.Summary, before.Document.Summary)
	}
}

func TestATSDelta_ForeignCVIsNotFoundAndABaseCVIsAConflict(t *testing.T) {
	pool := startPostgres(t)
	f := newATSDeltaFixture(t, pool)

	// A CV owned by somebody else is indistinguishable from one that does not exist.
	otherID := seedAccount(t, pool, "other@example.test", false)
	other, err := f.store.Create(context.Background(), otherID, "Theirs", "classic-ats", cv.Document{Summary: "theirs"})
	if err != nil {
		t.Fatalf("create foreign cv: %v", err)
	}
	if _, status := f.get(t, other.ID.String()); status != fiber.StatusNotFound {
		t.Errorf("foreign cv = %d, want 404", status)
	}

	// A base CV has no baseline of its own.
	if _, status := f.get(t, f.base.ID.String()); status != fiber.StatusConflict {
		t.Errorf("base cv = %d, want 409", status)
	}
}

func TestATSDelta_WithoutARendererDegradesInsteadOfFailing(t *testing.T) {
	f := newATSDeltaFixture(t, startPostgres(t))
	f.h.cvRenderer = nil

	got, status := f.get(t, f.tailored.ID.String())

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — an accessory read must not fail the workspace", status)
	}
	if got.Available {
		t.Error("available = true, want false with no renderer configured")
	}
	if got.Reason == "" {
		t.Error("reason = empty, want a stated reason")
	}
	if got.Delta != nil {
		t.Errorf("delta = %+v, want nil when unavailable", got.Delta)
	}
}

// TestATSDelta_ComparesAgainstTheCurrentBase pins the documented trade-off: the baseline is
// the base CV as it stands NOW, not a snapshot from when the copy was made. Editing the base
// moves the delta, and that is the behaviour to notice if it ever needs to change.
func TestATSDelta_ComparesAgainstTheCurrentBase(t *testing.T) {
	f := newATSDeltaFixture(t, startPostgres(t))
	ctx := context.Background()

	first, _ := f.get(t, f.tailored.ID.String())

	// Rewrite the base so it now renders the same text as the tailored copy.
	if _, err := f.store.Update(ctx, f.base.ID, f.userID, "Base", "centered", cv.Document{
		Margins: cv.Margins{Top: 1.2, Right: 1.2, Bottom: 1.2, Left: 1.2},
		Header:  cv.Header{FullName: "Jane Roe", Email: "jane@example.com"},
		Summary: "tailored summary",
	}); err != nil {
		t.Fatalf("update base: %v", err)
	}

	second, _ := f.get(t, f.tailored.ID.String())

	if first.Delta == nil || second.Delta == nil {
		t.Fatalf("deltas = %v, %v, want both available", first.Delta, second.Delta)
	}
	if second.Delta.Change != 0 {
		t.Errorf("change after the base caught up = %d, want 0", second.Delta.Change)
	}
	if first.Delta.Change == second.Delta.Change {
		t.Errorf("change did not move (%d both times), want the baseline to track the current base CV",
			first.Delta.Change)
	}
}
