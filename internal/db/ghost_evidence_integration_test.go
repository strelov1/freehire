//go:build integration

// Integration tests for the ghost-signal evidence selection, which is SQL behavior
// and can only be verified against a real Postgres. The judgement that turns these
// rows into a verdict lives in internal/ghost and is tested there without a
// database. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// connectGmail gives a user a connected Gmail mailbox.
func connectGmail(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO gmail_connections (user_id, email, refresh_token_enc, status)
		 VALUES ($1, 'x@example.test', 'enc', 'connected')`, userID)
	if err != nil {
		t.Fatalf("connect gmail: %v", err)
	}
}

// allocateMailbox gives a user a hosted mailbox.
func allocateMailbox(t *testing.T, pool *pgxpool.Pool, userID int64, address string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO mailboxes (user_id, address) VALUES ($1, $2)`, userID, address)
	if err != nil {
		t.Fatalf("allocate mailbox: %v", err)
	}
}

func applyTo(t *testing.T, q *Queries, userID, jobID int64) {
	t.Helper()
	if _, err := q.MarkJobApplied(context.Background(), MarkJobAppliedParams{UserID: userID, JobID: jobID}); err != nil {
		t.Fatalf("MarkJobApplied: %v", err)
	}
}

// The gate that keeps a gap in our data from being reported as an employer's
// silence: with no connected mailbox, no reply can ever be linked, so the
// application would read silent whether or not it was answered.
func TestListGhostApplicationEvidence_RequiresAConnectedMailbox(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	jid := insertJob(t, pool, "ghost-mailbox-gate")
	noMail := seedAPIKeyUser(t, pool, "nomail@example.test")
	withGmail := seedAPIKeyUser(t, pool, "gmail@example.test")
	withHosted := seedAPIKeyUser(t, pool, "hosted@example.test")

	applyTo(t, q, noMail, jid)
	applyTo(t, q, withGmail, jid)
	applyTo(t, q, withHosted, jid)
	connectGmail(t, pool, withGmail)
	allocateMailbox(t, pool, withHosted, "hosted@mail.example.test")

	rows, err := q.ListGhostApplicationEvidence(ctx, []int64{jid})
	if err != nil {
		t.Fatalf("ListGhostApplicationEvidence: %v", err)
	}

	got := map[int64]bool{}
	for _, r := range rows {
		got[r.UserID] = true
	}
	if got[noMail] {
		t.Error("the user with no connected mailbox was returned; their silence is unobserved, not observed")
	}
	if !got[withGmail] {
		t.Error("the user with a connected Gmail was not returned")
	}
	if !got[withHosted] {
		t.Error("the user with a hosted mailbox was not returned")
	}
}

// A revoked grant is not a connected mailbox: mail stops arriving, so an
// application under it would again read silent for want of observation.
func TestListGhostApplicationEvidence_ExcludesAReconsentingConnection(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	jid := insertJob(t, pool, "ghost-reconsent")
	uid := seedAPIKeyUser(t, pool, "reconsent@example.test")
	applyTo(t, q, uid, jid)

	if _, err := pool.Exec(ctx,
		`INSERT INTO gmail_connections (user_id, email, refresh_token_enc, status)
		 VALUES ($1, 'x@example.test', 'enc', 'needs_reconsent')`, uid); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rows, err := q.ListGhostApplicationEvidence(ctx, []int64{jid})
	if err != nil {
		t.Fatalf("ListGhostApplicationEvidence: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want none — the grant needs reconsent, so no mail arrives", rows)
	}
}

// A merely viewed or saved job is not an application and is waiting on nobody.
func TestListGhostApplicationEvidence_IgnoresNonApplications(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	jid := insertJob(t, pool, "ghost-saved-only")
	uid := seedAPIKeyUser(t, pool, "savedonly@example.test")
	connectGmail(t, pool, uid)
	if _, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: jid}); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}

	rows, err := q.ListGhostApplicationEvidence(ctx, []int64{jid})
	if err != nil {
		t.Fatalf("ListGhostApplicationEvidence: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want none — a saved job is not an application", rows)
	}
}

func TestListGhostReportEvidence_ExcludesRetracted(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	jid := insertJob(t, pool, "ghost-report-retracted")
	live := seedAPIKeyUser(t, pool, "livereport@example.test")
	gone := seedAPIKeyUser(t, pool, "gonereport@example.test")

	for _, uid := range []int64{live, gone} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO ghost_reports (user_id, job_id, applied_on) VALUES ($1, $2, current_date - 30)`,
			uid, jid); err != nil {
			t.Fatalf("seed report: %v", err)
		}
	}
	if _, err := pool.Exec(ctx,
		`UPDATE ghost_reports SET retracted_at = now() WHERE user_id = $1`, gone); err != nil {
		t.Fatalf("retract: %v", err)
	}

	rows, err := q.ListGhostReportEvidence(ctx, []int64{jid})
	if err != nil {
		t.Fatalf("ListGhostReportEvidence: %v", err)
	}
	if len(rows) != 1 || rows[0].UserID != live {
		t.Errorf("rows = %+v, want only the live report from user %d", rows, live)
	}
}

// One person may hold at most one report per job, so a second file cannot inflate
// the evidence. The bound is the schema's, not a check the service could forget.
func TestGhostReports_OneReportPerPersonPerJob(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	jid := insertJob(t, pool, "ghost-report-unique")
	uid := seedAPIKeyUser(t, pool, "dupreport@example.test")

	insert := func() error {
		_, err := pool.Exec(ctx,
			`INSERT INTO ghost_reports (user_id, job_id, applied_on) VALUES ($1, $2, current_date - 30)`,
			uid, jid)
		return err
	}
	if err := insert(); err != nil {
		t.Fatalf("first report: %v", err)
	}
	if err := insert(); err == nil {
		t.Error("second report was accepted, want the uniqueness to reject it")
	}
}
