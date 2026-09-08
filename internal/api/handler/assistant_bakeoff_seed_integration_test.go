//go:build integration

// Seeding for the model bake-off: one case's posting, the CV that will be tailored against
// it, and the tailoring session binding the two. The bake-off itself is behind the llmlive
// tag because it spends money; this half is behind integration alone, so the seed is
// exercised by the ordinary tagged suite rather than only by a run that costs tokens.
//
//	go test -tags=integration ./internal/api/handler/ -run BakeoffSeed
package handler

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvmatch"
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// seedBakeoffCase writes one case's posting and a CV bound to it, and opens the tailoring
// session the autopilot runs on. It returns the session and the CV it addresses.
//
// The posting is written whole. Truncating a description here would be the same mistake as
// a case fixture with none: the run walks the vacancy's requirements, so a shortened
// posting is a shorter measurement wearing the same case id.
func seedBakeoffCase(
	t *testing.T,
	pool *pgxpool.Pool,
	h *assistantHandlers,
	userID int64,
	c bakeoffCase,
	cvDocument string,
) (assistant.Session, uuid.UUID, int64) {
	t.Helper()
	ctx := context.Background()

	var jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, company_slug, description, public_slug)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id`,
		c.Vacancy.Source, c.Vacancy.ExternalID, c.Vacancy.URL, c.Vacancy.Title,
		c.Vacancy.Company, normalize.CompanySlug(c.Vacancy.Company),
		c.Vacancy.Description, c.Vacancy.Slug,
	).Scan(&jobID); err != nil {
		t.Fatalf("seed case %q posting: %v", c.ID, err)
	}

	var cvID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO cvs (user_id, title, template_id, data, job_id)
		 VALUES ($1, $2, 'classic-ats', $3::jsonb, $4)
		 RETURNING id`,
		userID, "Bake-off — "+c.ID, cvDocument, jobID,
	).Scan(&cvID); err != nil {
		t.Fatalf("seed case %q cv: %v", c.ID, err)
	}

	sess, err := h.store.CreateSession(ctx, userID, assistant.PresetTailor, &cvID, &jobID)
	if err != nil {
		t.Fatalf("seed case %q session: %v", c.ID, err)
	}

	return sess, cvID, jobID
}

// The seed is read back through the binding the tools themselves close over — a tailoring
// session's CV tools take their CV and vacancy ids from it, so a session that binds the
// wrong pair would hand every candidate model a different vacancy while the report
// insisted they ran the same case.
func TestBakeoffSeedBindsTheSessionToItsOwnCaseAndPosting(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	h, _ := newAutopilotHarness(t, pool, iss, walkedTheRequirements(), nil)
	userID, _ := assistantUser(t, pool, iss, "bakeoff-seed@example.test", true)

	set, err := loadBakeoffCases(bakeoffCaseFixture)
	if err != nil {
		t.Fatalf("loadBakeoffCases: %v", err)
	}
	c := set.Cases[0]

	sess, cvID, jobID := seedBakeoffCase(t, pool, h, userID, c, `{"summary":"before the run"}`)

	if sess.CVID == nil || *sess.CVID != cvID {
		t.Errorf("session CV = %v, want %v", sess.CVID, cvID)
	}
	if sess.JobID == nil || *sess.JobID != jobID {
		t.Errorf("session job = %v, want %v", sess.JobID, jobID)
	}

	// The description is what the run walks, and it is the field most easily lost on the
	// way in — big, and defaulted to empty by the schema rather than rejected.
	var stored string
	if err := pool.QueryRow(context.Background(),
		`SELECT description FROM jobs WHERE id = $1`, jobID).Scan(&stored); err != nil {
		t.Fatalf("read back the posting: %v", err)
	}
	if stored != c.Vacancy.Description {
		t.Errorf("stored description is %d chars, the case carries %d", len(stored), len(c.Vacancy.Description))
	}
}

// scorableBakeoffCV is a document Typst renders to a page with a readable text layer: a
// header, a summary, one employment with bullets, and a skills group. It is written here
// rather than taken from the profile fixture because the fixture is a real CV and is not in
// this repository — a seam guarded only by a file CI cannot have is a seam CI never checks.
//
// It says "Golang" and never a bare "Go", which is not stylistic: skilltag declines the bare
// word (it is a preposition far more often than a language), so a CV written that way parses
// with the skill missing and the test would read a working toolchain as a broken one.
func scorableBakeoffCV() cv.Document {
	return cv.Document{
		Margins: cv.DefaultMargins(),
		Header:  cv.Header{FullName: "Jane Roe", Email: "jane@example.test", Phone: "+1 415 555 0134"},
		Summary: "Backend engineer. Core stack: Golang, Kafka, PostgreSQL.",
		Experience: []cv.ExperienceItem{{
			Role: "Senior Backend Engineer", Company: "Acme",
			Start: &perioddate.PeriodDate{Year: 2019}, Current: true,
			Bullets: []string{"Built Golang services carrying 2M requests a day.", "Ran the Kafka pipelines four teams read from."},
		}},
		Skills: []cv.SkillGroup{{Group: "Languages", Items: []string{"Golang", "Kafka", "PostgreSQL"}}},
	}
}

// The bake-off ranks on cvmatch and atscheck, and BOTH read the text layer of the rendered
// PDF rather than the stored document (see renderedCVText). A harness with no renderer does
// not fail that read — it degrades to an unavailable score — so a bake-off run over one
// would finish green and report a column of absences, which reads exactly like a set of
// models that all tailored nothing.
//
// This is the seam the bake-off needs from newAutopilotHarness, and it is checked here under
// the integration tag alone so a regression costs no tokens to catch.
func TestBakeoffHarnessScoresTheRenderedTailoredCV(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping rendered-CV scoring")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed; skipping rendered-CV scoring")
	}
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	h, _ := newAutopilotHarness(t, pool, iss, walkedTheRequirements(), nil,
		withRenderedCVScoring(cv.NewTypstRenderer(bin), resume.ExtractPDFText))
	userID, _ := assistantUser(t, pool, iss, "bakeoff-score@example.test", true)

	set, err := loadBakeoffCases(bakeoffCaseFixture)
	if err != nil {
		t.Fatalf("loadBakeoffCases: %v", err)
	}
	c := set.Cases[0]
	_, cvID, _ := seedBakeoffCase(t, pool, h, userID, c, string(mustJSON(t, scorableBakeoffCV())))

	rec, err := h.cv.cvStore.Get(context.Background(), cvID, userID)
	if err != nil {
		t.Fatalf("read seeded cv: %v", err)
	}
	tmpl, err := cv.ResolveTemplate(rec.TemplateID)
	if err != nil {
		t.Fatalf("resolve template: %v", err)
	}

	// The text layer first, and named: a score can come out positive off a title alone, so
	// asserting only on the number would pass over a toolchain that rendered nothing.
	text, err := h.cv.renderedCVText(context.Background(), rec.Document, tmpl)
	if err != nil {
		t.Fatalf("render the seeded cv: %v", err)
	}
	for _, want := range []string{"Jane Roe", "Kafka"} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered text layer is missing %q; the scores would read an empty document:\n%s", want, text)
		}
	}

	score, err := h.cv.cvJobMatchScore(context.Background(), rec.Document, tmpl, cvmatch.Input{
		JobTitle:  c.Vacancy.Title,
		JobSkills: []string{"go", "kafka", "terraform"},
	})
	if err != nil {
		t.Fatalf("score the rendered cv: %v", err)
	}
	if len(score.Contributing) == 0 {
		t.Fatal("nothing was scored; the report would rank an absence")
	}
	// The document states Go and Kafka and not Terraform, so the score has to have read it —
	// a keyword split it could not produce from the title or from an empty text layer.
	if len(score.MissingSkills) != 1 || score.MissingSkills[0] != "terraform" {
		t.Errorf("missing skills = %v, want [terraform] read off the rendered CV", score.MissingSkills)
	}
	if score.Overall <= 0 {
		t.Errorf("overall = %d, want a positive score off the rendered text layer", score.Overall)
	}
}

// The default is unchanged: a harness nobody asked for the toolchain has none, so the six
// existing callers keep the handler they had. The bake-off's option is additive or it is a
// change to every autopilot test that never asked for one.
func TestAutopilotHarnessHasNoCVToolchainByDefault(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	h, _ := newAutopilotHarness(t, pool, iss, walkedTheRequirements(), nil)

	_, err := h.cv.renderedCVText(context.Background(), scorableBakeoffCV(), cv.Template{})
	if !errors.Is(err, errNoRenderer) {
		t.Errorf("renderedCVText err = %v, want errNoRenderer — the harness must not acquire a toolchain nobody asked it for", err)
	}
}

// Every case must seed cleanly, not just the first: a case that fails to seed on the third
// posting would end the bake-off halfway through one model's pass and leave a report whose
// models were not measured over the same set.
func TestBakeoffSeedTakesEveryCommittedCase(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	h, _ := newAutopilotHarness(t, pool, iss, walkedTheRequirements(), nil)
	userID, _ := assistantUser(t, pool, iss, "bakeoff-seed-all@example.test", true)

	set, err := loadBakeoffCases(bakeoffCaseFixture)
	if err != nil {
		t.Fatalf("loadBakeoffCases: %v", err)
	}
	for _, c := range set.Cases {
		if _, _, jobID := seedBakeoffCase(t, pool, h, userID, c, `{"summary":"before the run"}`); jobID == 0 {
			t.Errorf("case %q seeded no posting", c.ID)
		}
	}
}
