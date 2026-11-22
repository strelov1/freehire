//go:build integration

// Integration tests for add-auto-apply-outcome-notifications: the three new
// candidate scans read the right rows and exclude the wrong ones, and MATCH
// stays idempotent across two passes over an unchanged terminal row — the same
// guarantee ListInterviewPrepCandidates/SettleClosedApplications already have,
// proven here against a real Postgres rather than the in-memory fakeStore.
//
// Run with: go test -tags=integration ./internal/engage/nudge/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package nudge

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// seedAppliedEvent inserts an application_events row of kind='applied' with the
// given source, mirroring what MarkJobApplied writes (see cmd/auto-apply/store.go
// and internal/application/userjob's own manual-apply path).
func seedAppliedEvent(t *testing.T, pool *pgxpool.Pool, userID, jobID int64, source string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO application_events (user_id, job_id, company_slug, kind, occurred_at, source)
		 VALUES ($1, $2, 'acme', 'applied', now(), $3)`,
		userID, jobID, source); err != nil {
		t.Fatalf("insert application_events: %v", err)
	}
}

// seedAutoApplyQueueEntry inserts an auto_apply_queue row, optionally already
// blocked or failed.
func seedAutoApplyQueueEntry(t *testing.T, pool *pgxpool.Pool, userID, jobID int64, blocked, failed bool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO auto_apply_queue (user_id, job_id, blocked_at, failed_at)
		 VALUES ($1, $2,
		         CASE WHEN $3 THEN now() ELSE NULL END,
		         CASE WHEN $4 THEN now() ELSE NULL END)
		 RETURNING id`,
		userID, jobID, blocked, failed).Scan(&id)
	if err != nil {
		t.Fatalf("insert auto_apply_queue: %v", err)
	}
	return id
}

func TestListAutoApplySubmittedCandidates_OnlyAutoApplySource(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	autoApplyUser := seedNudgeIntegrationUser(t, pool, "submitted-auto-apply@example.test")
	autoApplyJob := seedClosedJob(t, pool, "submitted-auto-apply-job")
	seedNotificationSettings(t, pool, autoApplyUser, []string{"push"})
	seedAppliedEvent(t, pool, autoApplyUser, autoApplyJob, "auto_apply")

	manualUser := seedNudgeIntegrationUser(t, pool, "submitted-manual@example.test")
	manualJob := seedClosedJob(t, pool, "submitted-manual-job")
	seedNotificationSettings(t, pool, manualUser, []string{"push"})
	seedAppliedEvent(t, pool, manualUser, manualJob, "user")

	rows, err := queries.ListAutoApplySubmittedCandidates(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (only the auto_apply-sourced event)", len(rows))
	}
	if rows[0].UserID != autoApplyUser || rows[0].JobID.Int64 != autoApplyJob {
		t.Errorf("row = %+v, want the auto_apply-sourced user/job", rows[0])
	}
}

func TestListAutoApplyBlockedCandidates_OnlyBlockedRows(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	blockedUser := seedNudgeIntegrationUser(t, pool, "blocked-yes@example.test")
	blockedJob := seedClosedJob(t, pool, "blocked-yes-job")
	seedNotificationSettings(t, pool, blockedUser, []string{"push"})
	seedAutoApplyQueueEntry(t, pool, blockedUser, blockedJob, true, false)

	activeUser := seedNudgeIntegrationUser(t, pool, "blocked-no@example.test")
	activeJob := seedClosedJob(t, pool, "blocked-no-job")
	seedNotificationSettings(t, pool, activeUser, []string{"push"})
	seedAutoApplyQueueEntry(t, pool, activeUser, activeJob, false, false)

	rows, err := queries.ListAutoApplyBlockedCandidates(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (only the blocked row)", len(rows))
	}
	if rows[0].UserID != blockedUser || rows[0].JobID != blockedJob {
		t.Errorf("row = %+v, want the blocked user/job", rows[0])
	}
}

// A row can carry both blocked_at and failed_at (a lease-timeout race between a
// park and a fail — see AutoApplyQueueMetrics' own comment, metrics.sql), and the
// dead-letter marker must win: this attempt is a failed nudge, never also a
// blocked one.
func TestListAutoApplyBlockedCandidates_ExcludesRowsAlsoFailed(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := seedNudgeIntegrationUser(t, pool, "blocked-and-failed@example.test")
	jobID := seedClosedJob(t, pool, "blocked-and-failed-job")
	seedNotificationSettings(t, pool, userID, []string{"push"})
	seedAutoApplyQueueEntry(t, pool, userID, jobID, true, true)

	blocked, err := queries.ListAutoApplyBlockedCandidates(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 0 {
		t.Errorf("blocked rows = %d, want 0 (failed_at wins over blocked_at)", len(blocked))
	}

	failed, err := queries.ListAutoApplyFailedCandidates(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Errorf("failed rows = %d, want 1", len(failed))
	}
}

func TestListAutoApplyFailedCandidates_OnlyFailedRows(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	failedUser := seedNudgeIntegrationUser(t, pool, "failed-yes@example.test")
	failedJob := seedClosedJob(t, pool, "failed-yes-job")
	seedNotificationSettings(t, pool, failedUser, []string{"push"})
	seedAutoApplyQueueEntry(t, pool, failedUser, failedJob, false, true)

	activeUser := seedNudgeIntegrationUser(t, pool, "failed-no@example.test")
	activeJob := seedClosedJob(t, pool, "failed-no-job")
	seedNotificationSettings(t, pool, activeUser, []string{"push"})
	seedAutoApplyQueueEntry(t, pool, activeUser, activeJob, false, false)

	rows, err := queries.ListAutoApplyFailedCandidates(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (only the failed row)", len(rows))
	}
	if rows[0].UserID != failedUser || rows[0].JobID != failedJob {
		t.Errorf("row = %+v, want the failed user/job", rows[0])
	}
}

// TestMatch_AutoApplyOutcomeKinds_SecondPassDoesNotReRecord proves RecordNudge's
// ON CONFLICT DO NOTHING is idempotent for all three new kinds across two real
// MATCH passes over an unchanged row — blocked_at/failed_at never move (see
// nudge.go's actionable() doc), so a naive re-scan must not double-record.
func TestMatch_AutoApplyOutcomeKinds_SecondPassDoesNotReRecord(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := seedNudgeIntegrationUser(t, pool, "auto-apply-idempotent@example.test")
	jobID := seedClosedJob(t, pool, "auto-apply-idempotent-job")
	seedNotificationSettings(t, pool, userID, []string{"push"})
	// A real successful submission deletes its own queue row (cmd/auto-apply/store.go's
	// Submit), so jobID here carries only the applied event, never a queue row.
	seedAppliedEvent(t, pool, userID, jobID, "auto_apply")
	blockedJob := seedClosedJob(t, pool, "auto-apply-idempotent-blocked-job")
	seedAutoApplyQueueEntry(t, pool, userID, blockedJob, true, false)
	failedJob := seedClosedJob(t, pool, "auto-apply-idempotent-failed-job")
	seedAutoApplyQueueEntry(t, pool, userID, failedJob, false, true)

	// No channel registered in the Router: deliver soft-skips every claimed nudge
	// (released, not delivered), so a second Run's MATCH re-scans the identical
	// rows rather than finding them already terminal.
	runner := New(queries, Router{}, DefaultConfig())

	stats1, err := runner.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats1.Matched != 3 {
		t.Fatalf("first pass Matched = %d, want 3 (submitted + blocked + failed)", stats1.Matched)
	}

	stats2, err := runner.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats2.Matched != 0 {
		t.Errorf("second pass Matched = %d, want 0 (already recorded, ON CONFLICT DO NOTHING)", stats2.Matched)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM application_nudges WHERE user_id = $1 AND kind IN ('auto_apply_submitted', 'auto_apply_blocked', 'auto_apply_failed')`,
		userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("application_nudges rows = %d, want 3 (one per kind, no duplicates)", count)
	}
}

// Only a tailored, previewed, undecided entry on an open job is waiting for review.
func TestListAutoApplyReviewReadyCandidates_OnlyPendingOnOpenJobs(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := seedNudgeIntegrationUser(t, pool, "review-ready@example.test")
	seedNotificationSettings(t, pool, userID, []string{"email"})
	seed := func(externalID string, closed bool, set string) int64 {
		t.Helper()
		var jobID int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, company, public_slug, closed_at)
			 VALUES ('test', $1, 'http://example.test', 'Go Dev', 'Acme', $1, CASE WHEN $2 THEN now() END)
			 RETURNING id`, externalID, closed).Scan(&jobID); err != nil {
			t.Fatalf("insert job: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`WITH cv AS (INSERT INTO cvs (user_id, title, data) VALUES ($1, 'CV', '{}') RETURNING id)
			 INSERT INTO auto_apply_queue (user_id, job_id, tailored_cv_id, resolved_preview)
			 SELECT $1, $2, cv.id, '{"fields":[]}' FROM cv`, userID, jobID); err != nil {
			t.Fatalf("insert queue entry: %v", err)
		}
		if set != "" {
			if _, err := pool.Exec(ctx, "UPDATE auto_apply_queue SET "+set+" WHERE job_id = $1", jobID); err != nil {
				t.Fatal(err)
			}
		}
		return jobID
	}
	want := seed("review-ready-open", false, "")
	seed("review-ready-closed", true, "")
	seed("review-ready-approved", false, "review_decision = 'approved'")
	seed("review-ready-blocked", false, "blocked_at = now()")
	seed("review-ready-no-preview", false, "resolved_preview = NULL")

	rows, err := queries.ListAutoApplyReviewReadyCandidates(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].JobID != want {
		t.Errorf("rows = %+v, want only job %d", rows, want)
	}
}

// A review-ready nudge is bound to its queue entry: once the candidate withdraws and re-queues
// the job, the held nudge no longer reads as pending (the new entry records its own).
func TestGetNudgeForDelivery_ReviewPendingBoundToEpisode(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := seedNudgeIntegrationUser(t, pool, "review-episode@example.test")
	var jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('test', 'review-episode', 'http://example.test', 'Go Dev', 'Acme', 'review-episode')
		 RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	queue := func() pgtype.Timestamptz {
		t.Helper()
		var created pgtype.Timestamptz
		if err := pool.QueryRow(ctx,
			`WITH cv AS (INSERT INTO cvs (user_id, title, data) VALUES ($1, 'CV', '{}') RETURNING id)
			 INSERT INTO auto_apply_queue (user_id, job_id, tailored_cv_id, resolved_preview)
			 SELECT $1, $2, cv.id, '{"fields":[]}' FROM cv RETURNING created_at`, userID, jobID).Scan(&created); err != nil {
			t.Fatalf("insert queue entry: %v", err)
		}
		return created
	}
	episode := queue()
	if _, err := queries.RecordNudge(ctx, db.RecordNudgeParams{UserID: userID, JobID: jobID, Kind: KindAutoApplyReviewReady, EpisodeKey: episode}); err != nil {
		t.Fatal(err)
	}
	var nudgeID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM application_nudges WHERE user_id = $1 AND job_id = $2`, userID, jobID).Scan(&nudgeID); err != nil {
		t.Fatal(err)
	}
	pending := func() bool {
		t.Helper()
		info, err := queries.GetNudgeForDelivery(ctx, nudgeID)
		if err != nil {
			t.Fatal(err)
		}
		return info.AutoApplyReviewPending
	}
	if !pending() {
		t.Fatal("pending = false for the nudge's own entry, want true")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM auto_apply_queue WHERE job_id = $1`, jobID); err != nil {
		t.Fatal(err)
	}
	queue()
	if pending() {
		t.Error("pending = true against a re-queued entry, want false")
	}
}
