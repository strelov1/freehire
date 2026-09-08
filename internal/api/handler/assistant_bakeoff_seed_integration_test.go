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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/assistant"
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
