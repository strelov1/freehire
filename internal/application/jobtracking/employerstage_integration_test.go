//go:build integration

// Integration tests for EmployerStages over the real applications and application_events
// rows: the compare-and-set, the ledger event, and the candidate's own word winning.
// Run with: go test -tags=integration ./internal/application/jobtracking/
package jobtracking_test

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestEmployerStagesMirror(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)
	mirror := jobtracking.NewEmployerStages(q)

	var userID, jobID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('c@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug)
		VALUES ('employer', 'freehire:1', '', 'Platform Engineer', 'Acme', 'acme', 'acme-platform') RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	stage := func() string {
		t.Helper()
		var s *string
		if err := pool.QueryRow(ctx, `SELECT stage FROM applications WHERE user_id = $1 AND job_id = $2`, userID, jobID).Scan(&s); err != nil {
			t.Fatalf("read stage: %v", err)
		}
		if s == nil {
			return ""
		}
		return *s
	}

	if moved, err := mirror.Mirror(ctx, userID, jobID, "interview"); err != nil || moved {
		t.Fatalf("Mirror with no record = %v, %v; want nothing created", moved, err)
	}

	svc := jobtracking.New(jobtracking.NewQueriesRepository(q, pool))
	if _, err := svc.MarkApplied(ctx, userID, "acme-platform", appevent.SourceUser); err != nil {
		t.Fatalf("MarkApplied: %v", err)
	}
	if moved, err := mirror.Mirror(ctx, userID, jobID, "interview"); err != nil || !moved || stage() != "interview" {
		t.Fatalf("Mirror(interview) = %v, %v, stage %q", moved, err, stage())
	}
	// The employer's move is activity: the silence clock restarts from it.
	if _, err := pool.Exec(ctx, `UPDATE applications SET applied_at = now() - interval '30 days' WHERE user_id = $1 AND job_id = $2`, userID, jobID); err != nil {
		t.Fatalf("backdate applied_at: %v", err)
	}
	app, err := q.GetUserApplication(ctx, db.GetUserApplicationParams{UserID: userID, JobID: jobID})
	if err != nil || !app.LastActivityAt.Valid || time.Since(app.LastActivityAt.Time) > time.Hour {
		t.Fatalf("last activity after an employer move = %v, %v; want the move, not the 30-day-old apply date", app.LastActivityAt, err)
	}

	var source string
	if err := pool.QueryRow(ctx, `
		SELECT source FROM application_events WHERE user_id = $1 AND job_id = $2 AND kind = 'stage_set'
		ORDER BY id DESC LIMIT 1`, userID, jobID).Scan(&source); err != nil || source != appevent.SourceEmployer {
		t.Fatalf("ledger source = %q, %v; want employer", source, err)
	}

	// The employer corrects its own stage backwards.
	if moved, err := mirror.Mirror(ctx, userID, jobID, "screening"); err != nil || !moved || stage() != "screening" {
		t.Fatalf("Mirror(screening) over its own stage = %v, %v, stage %q", moved, err, stage())
	}

	// A stage mail set AFTER the employer's move but dated earlier (the message's own
	// received_at) is still the latest word: the employer no longer owns the stage.
	if _, err := pool.Exec(ctx, `UPDATE applications SET stage = 'offer' WHERE user_id = $1 AND job_id = $2`, userID, jobID); err != nil {
		t.Fatalf("mail stage: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO application_events (user_id, application_id, job_id, company_slug, kind, signal, occurred_at, source)
		SELECT $1, a.id, $2, 'acme', 'stage_set', 'offer', now() - interval '3 days', 'mail_gmail'
		FROM applications a WHERE a.user_id = $1 AND a.job_id = $2`, userID, jobID); err != nil {
		t.Fatalf("mail event: %v", err)
	}
	if moved, err := mirror.Mirror(ctx, userID, jobID, "screening"); err != nil || moved || stage() != "offer" {
		t.Fatalf("Mirror(screening) over a later, back-dated mail stage = %v, %v, stage %q", moved, err, stage())
	}

	// Once the candidate withdraws, the employer can no longer move them.
	withdrawn := "withdrawn"
	if _, err := svc.Track(ctx, userID, "acme-platform", &withdrawn, nil, appevent.SourceUser); err != nil {
		t.Fatalf("Track(withdrawn): %v", err)
	}
	if moved, err := mirror.Mirror(ctx, userID, jobID, "offer"); err != nil || moved || stage() != "withdrawn" {
		t.Fatalf("Mirror(offer) over a withdrawal = %v, %v, stage %q", moved, err, stage())
	}

	// After clearing and re-applying, the record is the candidate's again: an older employer
	// event no longer makes the employer its author, so it cannot be moved backward.
	if _, err := svc.ClearProgress(ctx, userID, "acme-platform"); err != nil {
		t.Fatalf("ClearProgress: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO application_events (user_id, application_id, job_id, company_slug, kind, signal, occurred_at, source)
		SELECT $1, a.id, $2, 'acme', 'stage_set', 'rejected', now(), 'employer'
		FROM applications a WHERE a.user_id = $1 AND a.job_id = $2`, userID, jobID); err != nil {
		t.Fatalf("employer rejection event: %v", err)
	}
	if _, err := svc.MarkApplied(ctx, userID, "acme-platform", appevent.SourceUser); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if moved, err := mirror.Mirror(ctx, userID, jobID, "preparing"); err != nil || moved || stage() != "applied" {
		t.Fatalf("Mirror(preparing) after a re-apply = %v, %v, stage %q; want the candidate's applied kept", moved, err, stage())
	}

	// A record whose progress the candidate cleared is not revived by the employer.
	if _, err := svc.ClearProgress(ctx, userID, "acme-platform"); err != nil {
		t.Fatalf("ClearProgress: %v", err)
	}
	if moved, err := mirror.Mirror(ctx, userID, jobID, "interview"); err != nil || moved || stage() != "" {
		t.Fatalf("Mirror(interview) over cleared progress = %v, %v, stage %q", moved, err, stage())
	}
}
