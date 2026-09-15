//go:build integration

// Integration test for RejectAndBlockHost's race-safety: when the target submission is no
// longer pending by the time the transaction runs (a concurrent decision, e.g. another
// moderator's approve landing between Service.Reject's initial Get() and this
// transactional re-check), the WHOLE transaction rolls back — not just the target's own
// status. Needs a real Postgres, since the atomicity claim is about a real transaction,
// which a fake repository (see submission_test.go) cannot model.
// Run with: go test -tags=integration ./internal/ingest/submission/
package submission_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/submission"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// seedUser inserts a bare user row — job_submissions.submitted_by/.reviewed_by are foreign
// keys, so every actor in these tests needs a real row.
func seedUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

// seedSubmission inserts a job_submissions row at the given status directly, bypassing
// Service.Submit — used here to plant a target already past 'pending', which the public
// API can never do (Submit only ever writes 'pending').
func seedSubmission(t *testing.T, pool *pgxpool.Pool, submittedBy int64, url, status string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO job_submissions (submitted_by, url, title, company, status)
		 VALUES ($1, $2, 'Some Role', 'Some Co', $3) RETURNING id`,
		submittedBy, url, status).Scan(&id); err != nil {
		t.Fatalf("seed submission %s: %v", url, err)
	}
	return id
}

func TestRejectAndBlockHost_TargetAlreadyDecided_RollsBackWholeTransaction(t *testing.T) {
	pool := testdb.Pool(t)
	repo := submission.NewQueriesRepository(db.New(pool), pool)
	ctx := context.Background()

	submitter := seedUser(t, pool, "submitter@example.test")
	reviewer := seedUser(t, pool, "reviewer@example.test")

	// The target is already 'approved' — simulating the exact race window Service.Reject
	// cannot close alone: its own Get() saw 'pending', but by the time this transactional
	// call runs, someone else decided it.
	targetID := seedSubmission(t, pool, submitter, "https://gridnaut.site/jobs/already-decided/", "approved")
	// A genuine sibling that WOULD have been bulk-rejected had the transaction committed.
	siblingID := seedSubmission(t, pool, submitter, "https://gridnaut.site/jobs/sibling/", "pending")

	_, err := repo.RejectAndBlockHost(ctx, targetID, "gridnaut.site", reviewer, "referral spam")
	if !errors.Is(err, submission.ErrAlreadyDecided) {
		t.Fatalf("err = %v, want ErrAlreadyDecided", err)
	}

	var siblingStatus string
	if err := pool.QueryRow(ctx, "SELECT status FROM job_submissions WHERE id = $1", siblingID).Scan(&siblingStatus); err != nil {
		t.Fatalf("read sibling: %v", err)
	}
	if siblingStatus != "pending" {
		t.Errorf("sibling status = %q, want pending — the transaction must roll back entirely, not just skip the target", siblingStatus)
	}

	var blockedCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM submission_domain_blocklist WHERE host = 'gridnaut.site'").Scan(&blockedCount); err != nil {
		t.Fatalf("count blocklist: %v", err)
	}
	if blockedCount != 0 {
		t.Errorf("blocklist rows for gridnaut.site = %d, want 0 (the block insert must roll back with everything else)", blockedCount)
	}
}
