//go:build integration

// Integration tests for the application record: the guarantee that it outlives the
// posting it was made against, and that the facts hanging off it — ledger events and
// linked mail — stay attached when the posting goes.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// cmd/prune is the only hard-delete path for jobs, and it deletes on a schedule that has
// nothing to do with any candidate. An application is a record of something a person did;
// only its link to the catalogue may be cleared.
func TestApplication_OutlivesItsPosting(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "outlive@example.test", true)
	job := seedResponseJob(t, q, "outlive-1", "outliveco")
	appliedAt := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(time.Second)

	var appID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO applications (user_id, company_slug, role_title, job_id, applied_at, stage, notes)
		 VALUES ($1, 'outliveco', 'Go Dev', $2, $3, 'interview', 'liked the team')
		 RETURNING id`, user, job, appliedAt).Scan(&appID); err != nil {
		t.Fatalf("seed application: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO application_events (user_id, application_id, job_id, company_slug, kind, occurred_at, source)
		 VALUES ($1, $2, $3, 'outliveco', 'applied', $4, 'user')`,
		user, appID, job, appliedAt); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	var mailID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO emails (user_id, external_id, source, subject, job_id, application_id, received_at)
		 VALUES ($1, 'outlive-mail', 'gmail', 'Next steps', $2, $3, now()) RETURNING id`,
		user, job, appID).Scan(&mailID); err != nil {
		t.Fatalf("seed mail: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job); err != nil {
		t.Fatalf("prune the posting: %v", err)
	}

	var (
		gotJob                      *int64
		gotSlug, gotTitle, gotStage string
		gotNotes                    string
		gotApplied                  time.Time
	)
	if err := pool.QueryRow(ctx,
		`SELECT job_id, company_slug, role_title, stage, notes, applied_at FROM applications WHERE id = $1`,
		appID).Scan(&gotJob, &gotSlug, &gotTitle, &gotStage, &gotNotes, &gotApplied); err != nil {
		t.Fatalf("the application did not survive the deletion: %v", err)
	}
	if gotJob != nil {
		t.Errorf("job_id = %v after the posting was pruned, want NULL", *gotJob)
	}
	if gotSlug != "outliveco" || gotTitle != "Go Dev" {
		t.Errorf("employer/role = %q/%q, want outliveco/Go Dev — they are stored on the record so they can survive", gotSlug, gotTitle)
	}
	if gotStage != "interview" || gotNotes != "liked the team" {
		t.Errorf("stage/notes = %q/%q, want interview/'liked the team'", gotStage, gotNotes)
	}
	if !gotApplied.UTC().Equal(appliedAt) {
		t.Errorf("applied_at = %s, want %s", gotApplied, appliedAt)
	}

	var events int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM application_events WHERE application_id = $1`, appID).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 1 {
		t.Errorf("events still attached = %d, want 1 — an event belongs to its application, not to the posting", events)
	}

	var mailApp *int64
	if err := pool.QueryRow(ctx,
		`SELECT application_id FROM emails WHERE id = $1`, mailID).Scan(&mailApp); err != nil {
		t.Fatalf("the mail did not survive the deletion: %v", err)
	}
	if mailApp == nil || *mailApp != appID {
		t.Errorf("mail application_id = %v, want %d — a pruned posting must not detach a thread", mailApp, appID)
	}
}

// The one-application-per-posting rule survives the move, and only while a posting is
// named: two applications to one employer with no posting are two different roles.
func TestApplication_UniquePerPostingOnlyWhileLinked(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "unique@example.test", false)
	job := seedResponseJob(t, q, "unique-1", "uniqueco")

	insert := func(jobID *int64) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO applications (user_id, company_slug, role_title, job_id, applied_at)
			 VALUES ($1, 'uniqueco', 'Go Dev', $2, now())`, user, jobID)
		return err
	}
	if err := insert(&job); err != nil {
		t.Fatalf("first application: %v", err)
	}
	if err := insert(&job); err == nil {
		t.Error("a second application to the same posting was accepted; applying twice must update one record")
	}
	if err := insert(nil); err != nil {
		t.Fatalf("first unlinked application: %v", err)
	}
	if err := insert(nil); err != nil {
		t.Errorf("a second unlinked application to the same employer was rejected: %v — two roles at one employer are two applications", err)
	}
}

// Applying records two things that must never disagree: the application and the ledger
// event that says it happened. One statement writes both, under one predicate.
func TestMarkJobApplied_CreatesTheApplicationAndNamesItOnTheEvent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "live-apply@example.test", true)
	job := seedResponseJob(t, q, "live-1", "liveco")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("MarkJobApplied: %v", err)
	}

	var appID int64
	var slug, title string
	if err := pool.QueryRow(ctx,
		`SELECT id, company_slug, role_title FROM applications WHERE user_id = $1 AND job_id = $2`,
		user, job).Scan(&appID, &slug, &title); err != nil {
		t.Fatalf("applying created no application: %v", err)
	}
	if slug != "liveco" || title != "Go Dev" {
		t.Errorf("employer/role = %q/%q, want liveco/Go Dev", slug, title)
	}

	var eventApp *int64
	if err := pool.QueryRow(ctx,
		`SELECT application_id FROM application_events WHERE user_id = $1 AND kind = 'applied'`,
		user).Scan(&eventApp); err != nil {
		t.Fatalf("read applied event: %v", err)
	}
	if eventApp == nil || *eventApp != appID {
		t.Errorf("applied event names application %v, want %d", eventApp, appID)
	}

	// Re-applying must not create a second record, and must not detach the event.
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	var apps int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM applications WHERE user_id = $1`, user).Scan(&apps); err != nil {
		t.Fatalf("count: %v", err)
	}
	if apps != 1 {
		t.Errorf("applications after re-applying = %d, want 1", apps)
	}
}

// A reply recorded from mail must name the application too, or the pair it forms with the
// apply event is only findable through the posting.
func TestRecordEmailApplicationEvent_NamesTheApplication(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "live-reply@example.test", true)
	job := seedResponseJob(t, q, "live-reply-1", "livereplyco")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	seedReply(t, q, user, job, "live-reply-mail")

	var eventApp, appID *int64
	if err := pool.QueryRow(ctx,
		`SELECT application_id FROM application_events WHERE user_id = $1 AND kind = 'employer_reply'`,
		user).Scan(&eventApp); err != nil {
		t.Fatalf("read reply event: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT id FROM applications WHERE user_id = $1`, user).Scan(&appID); err != nil {
		t.Fatalf("read application: %v", err)
	}
	if eventApp == nil || appID == nil || *eventApp != *appID {
		t.Errorf("reply event names application %v, want %v", eventApp, appID)
	}
}

// Setting a stage records a stage_set event, and it must name the application like every
// other event does — otherwise the one kind of event the candidate produces by hand is
// the one that cannot be correlated after a posting is pruned.
func TestTrackJob_StageEventNamesTheApplication(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "stage-event@example.test", true)
	job := seedResponseJob(t, q, "stage-event-1", "stageco")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := q.TrackJob(ctx, TrackJobParams{
		UserID: user, JobID: job,
		Stage:       pgtype.Text{String: "interview", Valid: true},
		EventSource: "user",
	}); err != nil {
		t.Fatalf("TrackJob: %v", err)
	}

	var eventApp, appID *int64
	if err := pool.QueryRow(ctx,
		`SELECT application_id FROM application_events WHERE user_id = $1 AND kind = 'stage_set'`,
		user).Scan(&eventApp); err != nil {
		t.Fatalf("read stage_set event: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM applications WHERE user_id = $1`, user).Scan(&appID); err != nil {
		t.Fatalf("read application: %v", err)
	}
	if eventApp == nil || appID == nil || *eventApp != *appID {
		t.Errorf("stage_set event names application %v, want %v", eventApp, appID)
	}
}

// Every path that links mail to a posting must keep application_id in step, or the
// column quietly goes stale on each new link and the carry-over would have to be run
// again to repair it.
func TestLinkingMailKeepsTheApplicationInStep(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "link-step@example.test", true)
	job := seedResponseJob(t, q, "link-step-1", "linkco")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var appID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM applications WHERE user_id = $1`, user).Scan(&appID); err != nil {
		t.Fatalf("read application: %v", err)
	}
	var mailID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO emails (user_id, external_id, source, received_at)
		 VALUES ($1, 'link-step-mail', 'gmail', now()) RETURNING id`, user).Scan(&mailID); err != nil {
		t.Fatalf("seed mail: %v", err)
	}

	appOf := func() *int64 {
		t.Helper()
		var got *int64
		if err := pool.QueryRow(ctx, `SELECT application_id FROM emails WHERE id = $1`, mailID).Scan(&got); err != nil {
			t.Fatalf("read link: %v", err)
		}
		return got
	}

	if _, err := q.LinkEmailToJob(ctx, LinkEmailToJobParams{ID: mailID, UserID: user, JobID: pgtype.Int8{Int64: job, Valid: true}}); err != nil {
		t.Fatalf("LinkEmailToJob: %v", err)
	}
	if got := appOf(); got == nil || *got != appID {
		t.Errorf("after a manual link, application_id = %v, want %d", got, appID)
	}

	if _, err := q.UnlinkEmail(ctx, UnlinkEmailParams{ID: mailID, UserID: user}); err != nil {
		t.Fatalf("UnlinkEmail: %v", err)
	}
	if got := appOf(); got != nil {
		t.Errorf("after unlinking, application_id = %v, want NULL — the link cleared on both columns", *got)
	}
}

// Task 6.5: mail linked to an application survives the removal of the posting, and the
// pruned pair must not read as a correction — nobody re-linked anything.
func TestLinkedMailSurvivesAPrunedPosting(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "mail-survives@example.test", true)
	job := seedResponseJob(t, q, "mail-survives-1", "survivco")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	mail := seedReply(t, q, user, job, "survives-reply")
	var appID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM applications WHERE user_id = $1`, user).Scan(&appID); err != nil {
		t.Fatalf("read application: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job); err != nil {
		t.Fatalf("prune: %v", err)
	}

	var linked *int64
	if err := pool.QueryRow(ctx, `SELECT application_id FROM emails WHERE id = $1`, mail).Scan(&linked); err != nil {
		t.Fatalf("read mail: %v", err)
	}
	if linked == nil || *linked != appID {
		t.Errorf("mail names application %v, want %d — a removal must not detach a thread", linked, appID)
	}

	// Reconciling after the prune must retract nothing: the pair changed on neither side.
	if n, err := q.RetractSupersededEmailEvent(ctx, RetractSupersededEmailEventParams{ID: mail, UserID: user}); err != nil {
		t.Fatalf("retract: %v", err)
	} else if n != 0 {
		t.Errorf("the reconcile retracted %d events after a prune, want 0 — nobody corrected anything", n)
	}
	var live int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM application_events
		  WHERE user_id = $1 AND kind = 'employer_reply' AND retracted_at IS NULL`, user).Scan(&live); err != nil {
		t.Fatalf("count: %v", err)
	}
	if live != 1 {
		t.Errorf("%d live reply events after the prune, want 1", live)
	}
}
