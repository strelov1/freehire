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
	"github.com/jackc/pgx/v5/pgxpool"
)

// applyAt records an application dated at, the state every test here starts from.
func applyAt(t *testing.T, q *Queries, uid, jobID int64, at time.Time) {
	t.Helper()
	if _, err := q.MarkJobApplied(context.Background(), MarkJobAppliedParams{
		UserID: uid, JobID: jobID, At: pgtype.Timestamptz{Time: at, Valid: true},
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
}

// mail is one seeded message. A zero JobID or SuggestedID leaves that column
// NULL, which is how the three link states are expressed: linked, suggested but
// unconfirmed, or neither.
type mail struct {
	ExternalID  string
	At          time.Time
	JobID       int64
	SuggestedID int64
	Deleted     bool
}

func seedMail(t *testing.T, pool *pgxpool.Pool, uid int64, m mail) {
	t.Helper()
	nullable := func(id int64) any {
		if id == 0 {
			return nil
		}
		return id
	}
	var deletedAt any
	if m.Deleted {
		deletedAt = time.Now()
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO emails (user_id, source, external_id, subject, body_text,
		                     received_at, job_id, suggested_job_id, deleted_at)
		 VALUES ($1, 'hosted', $2, 'subject', 'body', $3, $4, $5, $6)`,
		uid, m.ExternalID, m.At, nullable(m.JobID), nullable(m.SuggestedID), deletedAt)
	if err != nil {
		t.Fatalf("seed mail %s: %v", m.ExternalID, err)
	}
}

// trackedRow returns the caller's single tracked row under the given filter.
func trackedRow(t *testing.T, q *Queries, uid int64, filter string) ListUserJobsRow {
	t.Helper()
	rows, err := q.ListUserJobs(context.Background(), ListUserJobsParams{
		UserID: uid, Filter: filter, Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListUserJobs(%s): %v", filter, err)
	}
	if len(rows) != 1 {
		t.Fatalf("listing returned %d rows, want 1", len(rows))
	}
	return rows[0]
}

// sameSecond compares a returned timestamp against an expected one, which the
// tests build at second precision.
func sameSecond(got pgtype.Timestamptz, want time.Time) bool {
	return got.Valid && got.Time.UTC().Truncate(time.Second).Equal(want)
}

// TestLastActivityFallsBackToTheApplyDate asserts an application with no linked
// mail is dated by its apply date rather than reporting nothing — a silence
// clock that only starts once mail arrives would never fire on the applications
// that were ignored outright, which are exactly the ones worth reporting.
func TestLastActivityFallsBackToTheApplyDate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	uid := seedAPIKeyUser(t, pool, "silence-nomail@example.test")
	jid := insertJob(t, pool, "silence-1")
	applied := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(time.Second)
	applyAt(t, q, uid, jid, applied)

	row := trackedRow(t, q, uid, "all")
	if !sameSecond(row.LastActivityAt, applied) {
		t.Errorf("last_activity_at = %v, want the apply date %v", row.LastActivityAt, applied)
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

	uid := seedAPIKeyUser(t, pool, "silence-mail@example.test")
	jid := insertJob(t, pool, "silence-2")
	applied := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(time.Second)
	applyAt(t, q, uid, jid, applied)

	newer := applied.Add(9 * 24 * time.Hour)
	seedMail(t, pool, uid, mail{ExternalID: "sil-old", At: applied.Add(-5 * 24 * time.Hour), JobID: jid})
	seedMail(t, pool, uid, mail{ExternalID: "sil-new", At: newer, JobID: jid})

	if row := trackedRow(t, q, uid, "all"); !sameSecond(row.LastActivityAt, newer) {
		t.Errorf("last_activity_at = %v, want the newest linked message %v", row.LastActivityAt, newer)
	}
}

// TestLastActivityIgnoresUnlinkedAndDeletedMail asserts the aggregate counts only
// mail actually attached to this application: a pending suggestion is not
// activity (the whole point of the confirm step), another application's mail is
// not activity, and soft-deleted mail is gone.
func TestLastActivityIgnoresUnlinkedAndDeletedMail(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	uid := seedAPIKeyUser(t, pool, "silence-ignore@example.test")
	jid := insertJob(t, pool, "silence-3")
	other := insertJob(t, pool, "silence-3-other")
	applied := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(time.Second)
	applyAt(t, q, uid, jid, applied)

	recent := time.Now().Add(-time.Hour)
	seedMail(t, pool, uid, mail{ExternalID: "sil-sug", At: recent, SuggestedID: jid})
	seedMail(t, pool, uid, mail{ExternalID: "sil-other", At: recent, JobID: other})
	seedMail(t, pool, uid, mail{ExternalID: "sil-del", At: recent, JobID: jid, Deleted: true})

	row := trackedRow(t, q, uid, "applied")
	if !sameSecond(row.LastActivityAt, applied) {
		t.Errorf("last_activity_at = %v, want the apply date %v — none of that mail is activity",
			row.LastActivityAt, applied)
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
	applyAt(t, q, uid, jid, time.Now().Add(-30*24*time.Hour).UTC().Truncate(time.Second))

	recent := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	seedMail(t, pool, uid, mail{ExternalID: "sil-c", At: recent, SuggestedID: jid})
	if !trackedRow(t, q, uid, "all").HasPendingSuggestion {
		t.Fatal("suggestion not reported as pending before confirmation")
	}

	if _, err := pool.Exec(ctx,
		`UPDATE emails SET job_id = suggested_job_id, suggested_job_id = NULL, link_source = 'manual'
		  WHERE user_id = $1 AND external_id = 'sil-c'`, uid); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	row := trackedRow(t, q, uid, "all")
	if row.HasPendingSuggestion {
		t.Error("has_pending_suggestion is still true after confirming")
	}
	if !sameSecond(row.LastActivityAt, recent) {
		t.Errorf("last_activity_at = %v, want the now-linked message %v", row.LastActivityAt, recent)
	}
}

// TestLastActivityNullForNonApplications asserts a job merely viewed or saved
// reports no last activity: it is not waiting on anyone, and a clock on it would
// invite the UI to report a silence nobody is owed.
func TestLastActivityNullForNonApplications(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	uid := seedAPIKeyUser(t, pool, "silence-saved@example.test")
	jid := insertJob(t, pool, "silence-5")
	if _, err := q.SaveJob(context.Background(), SaveJobParams{UserID: uid, JobID: jid}); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}

	row := trackedRow(t, q, uid, "all")
	if row.LastActivityAt.Valid {
		t.Errorf("last_activity_at = %v on a saved-only job, want NULL", row.LastActivityAt.Time)
	}
	if row.HasPendingSuggestion {
		t.Error("has_pending_suggestion = true on a saved-only job")
	}
}

// TestAFollowUpDoesNotStopTheSilenceClock is the invariant the follow-up feature is built on: a
// chase the candidate sends is not a reply, so recording one must leave last_activity_at — and
// therefore the days silent and the silence state — exactly where they were. Folding
// followed_up_at into the derivation would clear the badge at the moment it matters most, and
// would tell the board an answer arrived when none did.
func TestAFollowUpDoesNotStopTheSilenceClock(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	uid := seedAPIKeyUser(t, pool, "silence-followup@example.test")
	jid := insertJob(t, pool, "silence-followup-1")
	applied := time.Now().Add(-24 * 24 * time.Hour).UTC().Truncate(time.Second)
	applyAt(t, q, uid, jid, applied)

	before := trackedRow(t, q, uid, "all")

	got, err := q.RecordApplicationFollowUp(context.Background(),
		RecordApplicationFollowUpParams{UserID: uid, JobID: jid})
	if err != nil {
		t.Fatalf("record follow-up: %v", err)
	}
	if !got.Valid {
		t.Fatal("recorded follow-up returned no timestamp")
	}

	after := trackedRow(t, q, uid, "all")
	if !sameSecond(after.LastActivityAt, applied) {
		t.Errorf("last_activity_at = %v after a follow-up, want it unchanged at the apply date %v",
			after.LastActivityAt, applied)
	}
	if !sameSecond(after.LastActivityAt, before.LastActivityAt.Time) {
		t.Errorf("last_activity_at moved from %v to %v — a chase is not a reply",
			before.LastActivityAt, after.LastActivityAt)
	}
	if !after.FollowedUpAt.Valid {
		t.Error("followed_up_at is unset on the tracked row; the board cannot say the application was chased")
	}
}

// TestFollowUpIsRefusedForANonApplication: a job merely viewed or saved has nobody to chase.
func TestFollowUpIsRefusedForANonApplication(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	uid := seedAPIKeyUser(t, pool, "silence-followup-2@example.test")
	jid := insertJob(t, pool, "silence-followup-2")
	if _, err := q.SaveJob(context.Background(), SaveJobParams{UserID: uid, JobID: jid}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, err := q.RecordApplicationFollowUp(context.Background(),
		RecordApplicationFollowUpParams{UserID: uid, JobID: jid}); err == nil {
		t.Error("recording a follow-up on a saved-but-not-applied job succeeded; want no row")
	}
}
