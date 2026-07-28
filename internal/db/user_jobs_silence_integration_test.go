//go:build integration

// Integration tests for the silence inputs the tracking listing derives: an
// application's last activity (its apply date, or the newest linked message when
// that is later) and whether any unconfirmed suggestion points at it. Both are
// SQL — a GREATEST over a correlated aggregate, and an EXISTS — so they can only
// be verified against a real Postgres. Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// listOne returns the caller's single tracked row.
func listOne(t *testing.T, q *Queries, uid int64) ListUserJobsRow {
	t.Helper()
	rows, err := q.ListUserJobs(context.Background(), ListUserJobsParams{
		UserID: uid, Filter: "all", Limit: 10, Offset: 0,
	})
	if err != nil {
		t.Fatalf("ListUserJobs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("listing returned %d rows, want 1", len(rows))
	}
	return rows[0]
}

// TestLastActivityFallsBackToTheApplyDate asserts an application with no linked
// mail is dated by its apply date rather than reporting nothing — a silence
// clock that only starts once mail arrives would never fire on the applications
// that were ignored outright, which are exactly the ones worth reporting.
func TestLastActivityFallsBackToTheApplyDate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "silence-nomail@example.test")
	jid := insertJob(t, pool, "silence-1")
	applied := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(time.Second)
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{
		UserID: uid, JobID: jid, At: pgtype.Timestamptz{Time: applied, Valid: true},
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	row := listOne(t, q, uid)
	if !row.LastActivityAt.Valid {
		t.Fatal("last_activity_at is NULL, want the apply date")
	}
	if got := row.LastActivityAt.Time.UTC().Truncate(time.Second); !got.Equal(applied) {
		t.Errorf("last_activity_at = %v, want the apply date %v", got, applied)
	}
	if row.HasPendingSuggestion {
		t.Error("has_pending_suggestion = true with no mail at all")
	}
}

// TestLastActivityMovesForwardWithMail asserts a linked message later than the
// apply date becomes the last activity, and that mail older than the apply date
// does not drag it backwards.
func TestLastActivityMovesForwardWithMail(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "silence-mail@example.test")
	jid := insertJob(t, pool, "silence-2")
	applied := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(time.Second)
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{
		UserID: uid, JobID: jid, At: pgtype.Timestamptz{Time: applied, Valid: true},
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	older := applied.Add(-5 * 24 * time.Hour)
	newer := applied.Add(9 * 24 * time.Hour)
	for _, m := range []struct {
		ext string
		at  time.Time
	}{{"sil-old", older}, {"sil-new", newer}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO emails (user_id, source, external_id, subject, body_text, received_at, job_id)
			 VALUES ($1,'hosted',$2,'s','b',$3,$4)`, uid, m.ext, m.at, jid); err != nil {
			t.Fatalf("seed mail %s: %v", m.ext, err)
		}
	}

	row := listOne(t, q, uid)
	if got := row.LastActivityAt.Time.UTC().Truncate(time.Second); !got.Equal(newer) {
		t.Errorf("last_activity_at = %v, want the newest linked message %v", got, newer)
	}
}

// TestLastActivityIgnoresUnlinkedAndDeletedMail asserts the aggregate counts only
// mail actually attached to this application: a pending suggestion is not
// activity (the whole point of the confirm step), another application's mail is
// not activity, and soft-deleted mail is gone.
func TestLastActivityIgnoresUnlinkedAndDeletedMail(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "silence-ignore@example.test")
	jid := insertJob(t, pool, "silence-3")
	other := insertJob(t, pool, "silence-3-other")
	applied := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(time.Second)
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{
		UserID: uid, JobID: jid, At: pgtype.Timestamptz{Time: applied, Valid: true},
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	recent := time.Now().Add(-time.Hour)

	// A suggestion awaiting confirmation.
	if _, err := pool.Exec(ctx,
		`INSERT INTO emails (user_id, source, external_id, subject, body_text, received_at, suggested_job_id)
		 VALUES ($1,'hosted','sil-sug','s','b',$2,$3)`, uid, recent, jid); err != nil {
		t.Fatalf("seed suggestion: %v", err)
	}
	// Mail linked to a different application.
	if _, err := pool.Exec(ctx,
		`INSERT INTO emails (user_id, source, external_id, subject, body_text, received_at, job_id)
		 VALUES ($1,'hosted','sil-other','s','b',$2,$3)`, uid, recent, other); err != nil {
		t.Fatalf("seed other-job mail: %v", err)
	}
	// Linked but soft-deleted.
	if _, err := pool.Exec(ctx,
		`INSERT INTO emails (user_id, source, external_id, subject, body_text, received_at, job_id, deleted_at)
		 VALUES ($1,'hosted','sil-del','s','b',$2,$3, now())`, uid, recent, jid); err != nil {
		t.Fatalf("seed deleted mail: %v", err)
	}

	rows, err := q.ListUserJobs(ctx, ListUserJobsParams{UserID: uid, Filter: "applied", Limit: 10})
	if err != nil {
		t.Fatalf("ListUserJobs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("applied listing returned %d rows, want 1", len(rows))
	}
	row := rows[0]
	if got := row.LastActivityAt.Time.UTC().Truncate(time.Second); !got.Equal(applied) {
		t.Errorf("last_activity_at = %v, want the apply date %v — none of that mail is activity", got, applied)
	}
	if !row.HasPendingSuggestion {
		t.Error("has_pending_suggestion = false, want true — a suggestion is pending on this application")
	}
}

// TestPendingSuggestionClearsOnceConfirmed asserts confirming the link both
// removes the pending flag and turns that message into activity.
func TestPendingSuggestionClearsOnceConfirmed(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "silence-confirm@example.test")
	jid := insertJob(t, pool, "silence-4")
	applied := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(time.Second)
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{
		UserID: uid, JobID: jid, At: pgtype.Timestamptz{Time: applied, Valid: true},
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	recent := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx,
		`INSERT INTO emails (user_id, source, external_id, subject, body_text, received_at, suggested_job_id)
		 VALUES ($1,'hosted','sil-c','s','b',$2,$3)`, uid, recent, jid); err != nil {
		t.Fatalf("seed suggestion: %v", err)
	}
	if !listOne(t, q, uid).HasPendingSuggestion {
		t.Fatal("suggestion not reported as pending before confirmation")
	}

	if _, err := pool.Exec(ctx,
		`UPDATE emails SET job_id = suggested_job_id, suggested_job_id = NULL, link_source = 'manual'
		  WHERE user_id = $1 AND external_id = 'sil-c'`, uid); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	row := listOne(t, q, uid)
	if row.HasPendingSuggestion {
		t.Error("has_pending_suggestion is still true after confirming")
	}
	if got := row.LastActivityAt.Time.UTC().Truncate(time.Second); !got.Equal(recent) {
		t.Errorf("last_activity_at = %v, want the now-linked message %v", got, recent)
	}
}

// TestLastActivityNullForNonApplications asserts a job merely viewed or saved
// reports no last activity: it is not waiting on anyone, and a clock on it would
// invite the UI to report a silence nobody is owed.
func TestLastActivityNullForNonApplications(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "silence-saved@example.test")
	jid := insertJob(t, pool, "silence-5")
	if _, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: jid}); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}

	row := listOne(t, q, uid)
	if row.LastActivityAt.Valid {
		t.Errorf("last_activity_at = %v on a saved-only job, want NULL", row.LastActivityAt.Time)
	}
	if row.HasPendingSuggestion {
		t.Error("has_pending_suggestion = true on a saved-only job")
	}
}
