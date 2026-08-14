//go:build integration

// Integration tests for the close-reason contract (openspec change
// telegram-vacancy-expiry): every mechanism that writes closed_at records which one it
// was, and every path that reopens a job clears that record. Without this, "closed"
// stands equally for five unrelated facts — and a sixth, the age rule, closes on a guess
// rather than on evidence, which is exactly the one worth telling apart.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// closeReason reads the recorded mechanism for one job.
func closeReason(t *testing.T, pool *pgxpool.Pool, id int64) string {
	t.Helper()
	var reason string
	if err := pool.QueryRow(context.Background(),
		"SELECT closed_reason FROM jobs WHERE id = $1", id).Scan(&reason); err != nil {
		t.Fatalf("read closed_reason: %v", err)
	}
	return reason
}

func TestCloseUnseenJobsRecordsItsReason(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:1", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	ageJob(t, pool, job.ID, 72*time.Hour)

	n, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{
		Source:       "greenhouse",
		Cutoff:       pgTimestamptz(time.Now().Add(-48 * time.Hour)),
		CompanySlugs: []string{"acme"},
	})
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 1 {
		t.Fatalf("sweep closed %d jobs, want 1", n)
	}
	if got := closeReason(t, pool, job.ID); got != "unseen" {
		t.Errorf("closed_reason = %q, want %q", got, "unseen")
	}
}

func TestCloseUnseenJobsBySourceRecordsItsReason(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:2", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	ageJob(t, pool, job.ID, 72*time.Hour)

	if _, err := q.CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{
		Source: "greenhouse",
		Cutoff: pgTimestamptz(time.Now().Add(-48 * time.Hour)),
	}); err != nil {
		t.Fatalf("source sweep: %v", err)
	}
	if got := closeReason(t, pool, job.ID); got != "unseen" {
		t.Errorf("closed_reason = %q, want %q", got, "unseen")
	}
}

func TestCloseJobBySourceExternalIDRecordsItsReason(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:3", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if _, err := q.CloseJobBySourceExternalID(ctx, CloseJobBySourceExternalIDParams{
		Source:     "greenhouse",
		ExternalID: "acme:3",
	}); err != nil {
		t.Fatalf("self-close: %v", err)
	}
	if got := closeReason(t, pool, job.ID); got != "feed_removed" {
		t.Errorf("closed_reason = %q, want %q", got, "feed_removed")
	}
}

func TestCloseJobByIDRecordsItsReason(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:4", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if _, err := q.CloseJobByID(ctx, job.ID); err != nil {
		t.Fatalf("moderator close: %v", err)
	}
	if got := closeReason(t, pool, job.ID); got != "moderated" {
		t.Errorf("closed_reason = %q, want %q", got, "moderated")
	}
}

func TestMarkLivenessExpiredRecordsItsReasonOnlyWhenItCloses(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:5", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// First strike: below the threshold, so nothing is closed and nothing is labelled.
	first, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: job.ID, Threshold: 2})
	if err != nil {
		t.Fatalf("first strike: %v", err)
	}
	if first.ClosedAt.Valid {
		t.Fatal("one strike must not close the job")
	}
	if got := closeReason(t, pool, job.ID); got != "" {
		t.Errorf("closed_reason after one strike = %q, want empty", got)
	}

	// Second strike reaches the threshold: the job closes and carries the probe's label.
	second, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: job.ID, Threshold: 2})
	if err != nil {
		t.Fatalf("second strike: %v", err)
	}
	if !second.ClosedAt.Valid {
		t.Fatal("second strike must close the job")
	}
	if got := closeReason(t, pool, job.ID); got != "probe_expired" {
		t.Errorf("closed_reason = %q, want %q", got, "probe_expired")
	}
}

func TestMarkLivenessExpiredPreservesClosedAtFromAnEarlierClose(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:7", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// First strike: below the threshold, so nothing is closed yet.
	if _, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: job.ID, Threshold: 2}); err != nil {
		t.Fatalf("first strike: %v", err)
	}

	// Another mechanism (a moderator resolving a report) closes the job first, between
	// candidate selection and the probe's second strike.
	if _, err := q.CloseJobByID(ctx, job.ID); err != nil {
		t.Fatalf("moderator close: %v", err)
	}
	before, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job after moderator close: %v", err)
	}
	if !before.ClosedAt.Valid {
		t.Fatal("moderator close must set closed_at")
	}

	// Sleep past Postgres's clock resolution so a real overwrite would be observable.
	time.Sleep(5 * time.Millisecond)

	// Second strike reaches the threshold. It must not touch either closed_at or
	// closed_reason: the moderator's close already recorded the true story.
	second, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: job.ID, Threshold: 2})
	if err != nil {
		t.Fatalf("second strike: %v", err)
	}
	if !second.ClosedAt.Time.Equal(before.ClosedAt.Time) {
		t.Errorf("closed_at = %v, want unchanged from moderator close %v", second.ClosedAt.Time, before.ClosedAt.Time)
	}
	if got := closeReason(t, pool, job.ID); got != "moderated" {
		t.Errorf("closed_reason = %q, want %q (moderator's close must survive the probe)", got, "moderated")
	}
}

func TestReopeningAJobClearsItsCloseReason(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:6", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	ageJob(t, pool, job.ID, 72*time.Hour)
	if _, err := q.CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{
		Source: "greenhouse",
		Cutoff: pgTimestamptz(time.Now().Add(-48 * time.Hour)),
	}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := closeReason(t, pool, job.ID); got != "unseen" {
		t.Fatalf("precondition: closed_reason = %q, want %q", got, "unseen")
	}

	// The posting reappears in a crawl. A reopened job must not keep the label of the
	// mechanism that closed it — otherwise "closed by the sweep" outlives the close.
	reopened, err := ingestUpsert(ctx, q, ingestParams("acme:6", "Engineer"))
	if err != nil {
		t.Fatalf("re-ingest: %v", err)
	}
	if reopened.ClosedAt.Valid {
		t.Fatal("re-ingest must reopen the job")
	}
	if got := closeReason(t, pool, job.ID); got != "" {
		t.Errorf("closed_reason after reopen = %q, want empty", got)
	}
}

func TestTouchJobClearsItsCloseReason(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:7", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	ageJob(t, pool, job.ID, 72*time.Hour)
	if _, err := q.CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{
		Source: "greenhouse",
		Cutoff: pgTimestamptz(time.Now().Add(-48 * time.Hour)),
	}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := closeReason(t, pool, job.ID); got != "unseen" {
		t.Fatalf("precondition: closed_reason = %q, want %q", got, "unseen")
	}

	// A hydrating source re-lists the offer without re-fetching its content: TouchJob is
	// the reopen half of UpsertJob's ON CONFLICT, so it owes the same clearing.
	if _, err := q.TouchJob(ctx, TouchJobParams{Source: "greenhouse", ExternalID: "acme:7"}); err != nil {
		t.Fatalf("touch: %v", err)
	}
	if isClosed(t, pool, job.ID) {
		t.Fatal("touch must reopen the job")
	}
	if got := closeReason(t, pool, job.ID); got != "" {
		t.Errorf("closed_reason after touch = %q, want empty", got)
	}
}

func TestUpsertManualJobClearsItsCloseReason(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	author := insertUser(t, pool, "moderator@example.test")
	manual := manualParams("https://example.test/manual", "Engineer", author, author)
	job, err := q.UpsertManualJob(ctx, manual)
	if err != nil {
		t.Fatalf("manual upsert: %v", err)
	}
	if _, err := q.CloseJobByID(ctx, job.ID); err != nil {
		t.Fatalf("moderator close: %v", err)
	}
	if got := closeReason(t, pool, job.ID); got != "moderated" {
		t.Fatalf("precondition: closed_reason = %q, want %q", got, "moderated")
	}

	// Re-submitting the same posting re-asserts it. It must not carry the moderator's
	// label into its second life.
	if _, err := q.UpsertManualJob(ctx, manual); err != nil {
		t.Fatalf("manual re-upsert: %v", err)
	}
	if isClosed(t, pool, job.ID) {
		t.Fatal("manual re-upsert must reopen the job")
	}
	if got := closeReason(t, pool, job.ID); got != "" {
		t.Errorf("closed_reason after manual re-upsert = %q, want empty", got)
	}
}
