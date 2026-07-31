//go:build integration

// Integration tests for the carry-over that gives every existing tracked application a
// record of its own, and points the facts already recorded against it — ledger events and
// linked mail — at that record.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"
)

// drainApplicationBackfill runs the keyset pass to exhaustion and reports how many
// applications it created.
func drainApplicationBackfill(t *testing.T, q *Queries) int64 {
	t.Helper()
	ctx := context.Background()
	var lastUser, lastJob, inserted int64
	for {
		row, err := q.BackfillApplications(ctx, BackfillApplicationsParams{
			LastUserID: lastUser, LastJobID: lastJob, BatchSize: 100,
		})
		if err != nil {
			t.Fatalf("BackfillApplications: %v", err)
		}
		if row.Scanned == 0 {
			return inserted
		}
		lastUser, lastJob, inserted = row.LastUserID, row.LastJobID, inserted+row.Inserted
	}
}

// seedTrackedApplication records an interaction the way the current tracker does: a
// user_jobs row with applied_at set.
func seedTrackedApplication(t *testing.T, q *Queries, userID, jobID int64, appliedAt time.Time, stage string) {
	t.Helper()
	if _, err := q.db.Exec(context.Background(),
		`INSERT INTO user_jobs (user_id, job_id, applied_at, stage, notes)
		 VALUES ($1, $2, $3, $4, 'a note')`, userID, jobID, appliedAt, stage); err != nil {
		t.Fatalf("seed tracked application: %v", err)
	}
}

// An interaction that was only viewed has no application to carry over, and inventing one
// would put a date on something that never happened.
func TestBackfillApplications_CarriesOverAppliedInteractionsOnly(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "carry@example.test", false)
	applied := seedResponseJob(t, q, "carry-1", "carryco")
	viewedOnly := seedResponseJob(t, q, "carry-2", "carryco")
	at := time.Now().Add(-10 * 24 * time.Hour).UTC().Truncate(time.Second)

	seedTrackedApplication(t, q, user, applied, at, "screening")
	if _, err := pool.Exec(ctx, `INSERT INTO user_jobs (user_id, job_id) VALUES ($1, $2)`, user, viewedOnly); err != nil {
		t.Fatalf("seed viewed interaction: %v", err)
	}

	if got := drainApplicationBackfill(t, q); got != 1 {
		t.Fatalf("carried over %d applications, want 1", got)
	}

	var jobID int64
	var slug, title, stage string
	var appliedAt time.Time
	if err := pool.QueryRow(ctx,
		`SELECT job_id, company_slug, role_title, stage, applied_at FROM applications WHERE user_id = $1`,
		user).Scan(&jobID, &slug, &title, &stage, &appliedAt); err != nil {
		t.Fatalf("read carried-over application: %v", err)
	}
	if jobID != applied {
		t.Errorf("job_id = %d, want %d — the viewed-only interaction must not have produced this row", jobID, applied)
	}
	if slug != "carryco" || title != "Go Dev" {
		t.Errorf("employer/role = %q/%q, want carryco/Go Dev, taken from the posting at carry-over", slug, title)
	}
	if stage != "screening" {
		t.Errorf("stage = %q, want screening", stage)
	}
	if !appliedAt.UTC().Equal(at) {
		t.Errorf("applied_at = %s, want %s — the original date, not the day of the migration", appliedAt, at)
	}
}

// Events recorded before this change name a posting and no application. Left that way they
// would be correlated through job_id forever, which is the defect being fixed.
func TestBackfillApplications_AttachesExistingLedgerEvents(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "attach-events@example.test", true)
	job := seedResponseJob(t, q, "attach-1", "attachco")
	at := time.Now().Add(-5 * 24 * time.Hour).UTC().Truncate(time.Second)
	seedTrackedApplication(t, q, user, job, at, "applied")
	if _, err := pool.Exec(ctx,
		`INSERT INTO application_events (user_id, job_id, company_slug, kind, occurred_at, source)
		 VALUES ($1, $2, 'attachco', 'applied', $3, 'user'),
		        ($1, $2, 'attachco', 'employer_reply', now(), 'mail_gmail')`,
		user, job, at); err != nil {
		t.Fatalf("seed pre-existing events: %v", err)
	}

	drainApplicationBackfill(t, q)
	if _, err := q.BackfillApplicationEventLinks(ctx, 100); err != nil {
		t.Fatalf("BackfillApplicationEventLinks: %v", err)
	}

	var unattached int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM application_events WHERE user_id = $1 AND application_id IS NULL`,
		user).Scan(&unattached); err != nil {
		t.Fatalf("count unattached: %v", err)
	}
	if unattached != 0 {
		t.Errorf("%d events still name no application; every fact recorded before this change must find its record", unattached)
	}
}

// The same for mail: a thread linked to a posting must end up linked to the application,
// or a later deletion detaches it from a record that still exists.
func TestBackfillApplications_AttachesExistingMail(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "attach-mail@example.test", true)
	job := seedResponseJob(t, q, "attach-mail-1", "attachmailco")
	at := time.Now().Add(-3 * 24 * time.Hour).UTC().Truncate(time.Second)
	seedTrackedApplication(t, q, user, job, at, "applied")
	var mailID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO emails (user_id, external_id, source, job_id, received_at)
		 VALUES ($1, 'attach-mail', 'gmail', $2, now()) RETURNING id`, user, job).Scan(&mailID); err != nil {
		t.Fatalf("seed mail: %v", err)
	}

	drainApplicationBackfill(t, q)
	if _, err := q.BackfillEmailApplicationLinks(ctx, 100); err != nil {
		t.Fatalf("BackfillEmailApplicationLinks: %v", err)
	}

	var appID *int64
	if err := pool.QueryRow(ctx, `SELECT application_id FROM emails WHERE id = $1`, mailID).Scan(&appID); err != nil {
		t.Fatalf("read mail: %v", err)
	}
	if appID == nil {
		t.Error("the mail still names no application after the carry-over")
	}
}

// An interrupted pass is restarted, not repaired, so a second run must be a no-op.
func TestBackfillApplications_SecondRunChangesNothing(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "rerun@example.test", true)
	job := seedResponseJob(t, q, "rerun-1", "rerunco")
	at := time.Now().Add(-7 * 24 * time.Hour).UTC().Truncate(time.Second)
	seedTrackedApplication(t, q, user, job, at, "applied")
	if _, err := pool.Exec(ctx,
		`INSERT INTO application_events (user_id, job_id, company_slug, kind, occurred_at, source)
		 VALUES ($1, $2, 'rerunco', 'applied', $3, 'user')`, user, job, at); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	drainApplicationBackfill(t, q)
	if _, err := q.BackfillApplicationEventLinks(ctx, 100); err != nil {
		t.Fatalf("first link pass: %v", err)
	}

	if got := drainApplicationBackfill(t, q); got != 0 {
		t.Errorf("a second carry-over created %d applications, want 0", got)
	}
	if _, err := q.BackfillApplicationEventLinks(ctx, 100); err != nil {
		t.Fatalf("second link pass: %v", err)
	}

	var apps, events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM applications WHERE user_id = $1`, user).Scan(&apps); err != nil {
		t.Fatalf("count applications: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM application_events WHERE user_id = $1`, user).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if apps != 1 || events != 1 {
		t.Errorf("after two runs: %d applications and %d events, want 1 and 1", apps, events)
	}
}
