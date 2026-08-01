//go:build integration

// Integration tests for the interview schedule: the meeting a candidate's calendar holds,
// attached to the application it belongs to.
//
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"
)

// seedApplication records an application the real way, so the row under test hangs off
// the same shape production makes.
func seedApplication(t *testing.T, q *Queries, userID int64, extID, company string) (jobID, appID int64) {
	t.Helper()
	ctx := context.Background()
	jobID = seedResponseJob(t, q, extID, company)
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: userID, JobID: jobID, EventSource: "user"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := q.db.QueryRow(ctx,
		`SELECT id FROM applications WHERE user_id = $1 AND job_id = $2`, userID, jobID).Scan(&appID); err != nil {
		t.Fatalf("read application: %v", err)
	}
	return jobID, appID
}

// The meeting moves, and the row moves with it. An append-only record could not express
// a reschedule, which is why the schedule is a table of its own and not a ledger kind.
func TestUpsertApplicationInterview_MovesInPlaceAndStaysOneRow(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "iv-move@example.test", true)
	_, appID := seedApplication(t, q, user, "iv-move-1", "derq")
	first := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	moved := time.Date(2026, 8, 15, 14, 30, 0, 0, time.UTC)

	upsert := func(at time.Time, title string) {
		t.Helper()
		if _, err := q.UpsertApplicationInterview(ctx, UpsertApplicationInterviewParams{
			UserID:        user,
			ApplicationID: appID,
			IcalUid:       "derq-interview@ashbyhq.com",
			StartsAt:      ts(at),
			EndsAt:        ts(at.Add(time.Hour)),
			Title:         title,
			JoinUrl:       "https://meet.google.com/abc-defg-hij",
			Source:        "calendar_google",
		}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	upsert(first, "Technical screen")
	upsert(first, "Technical screen") // a re-sync changes nothing
	upsert(moved, "Technical screen — moved")

	var rows int
	var startsAt time.Time
	var title, status string
	if err := q.db.QueryRow(ctx,
		`SELECT count(*) FROM application_interviews WHERE user_id = $1`, user).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("three syncs of one meeting left %d rows, want 1 — the key is (user, ical_uid)", rows)
	}
	if err := q.db.QueryRow(ctx,
		`SELECT starts_at, title, status FROM application_interviews WHERE user_id = $1`,
		user).Scan(&startsAt, &title, &status); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !startsAt.Equal(moved) {
		t.Errorf("starts_at = %v, want the moved %v", startsAt, moved)
	}
	if title != "Technical screen — moved" || status != "confirmed" {
		t.Errorf("row = %q/%q, want the moved title and confirmed", title, status)
	}
}

// A cancellation is a fact about the meeting, not an erasure of it. A candidate who
// remembers an interview on Thursday and finds nothing cannot tell a cancellation from a
// fault in the calendar.
func TestCancelApplicationInterview_MarksRatherThanDeletes(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "iv-cancel@example.test", true)
	_, appID := seedApplication(t, q, user, "iv-cancel-1", "derq")
	if _, err := q.UpsertApplicationInterview(ctx, UpsertApplicationInterviewParams{
		UserID: user, ApplicationID: appID, IcalUid: "cancel-me@ashbyhq.com",
		StartsAt: ts(time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)), Source: "calendar_google",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if _, err := q.CancelApplicationInterview(ctx, CancelApplicationInterviewParams{
		UserID: user, IcalUid: "cancel-me@ashbyhq.com",
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	var status string
	var rows int
	if err := q.db.QueryRow(ctx,
		`SELECT count(*), coalesce(max(status), '') FROM application_interviews WHERE user_id = $1`,
		user).Scan(&rows, &status); err != nil {
		t.Fatalf("read: %v", err)
	}
	if rows != 1 || status != "cancelled" {
		t.Errorf("after cancelling: %d rows with status %q, want 1 cancelled", rows, status)
	}
}

// The privacy boundary as a constraint rather than as a habit. A meeting the sync could
// not attach has no application to name, and the column refuses it — so a bug in the
// worker cannot put a candidate's dentist appointment in the database.
func TestApplicationInterviews_RefuseAMeetingWithNoApplication(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "iv-unmatched@example.test", true)
	_, err := q.db.Exec(ctx,
		`INSERT INTO application_interviews (user_id, ical_uid, starts_at, source)
		 VALUES ($1, 'dentist@personal', now(), 'calendar_google')`, user)
	if err == nil {
		t.Fatal("an unattached meeting was stored; application_id must be NOT NULL so only matched meetings can exist")
	}
}

// The link the whole feature turns on: the invitation is already tied to an application,
// and its UID says the calendar entry is that same meeting.
func TestApplicationForCalendarUID_ResolvesThroughTheLinkedInvitation(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	mine := seedResponseUser(t, q, "iv-uid@example.test", true)
	theirs := seedResponseUser(t, q, "iv-uid-other@example.test", true)
	jobID, appID := seedApplication(t, q, mine, "iv-uid-1", "derq")

	if _, err := q.db.Exec(ctx,
		`INSERT INTO emails (user_id, source, external_id, subject, received_at, job_id, application_id, ical_uid, status_signal)
		 VALUES ($1, 'gmail', 'inv-1', 'Interview', now(), $2, $3, 'derq-interview@ashbyhq.com', 'interview_invitation')`,
		mine, jobID, appID); err != nil {
		t.Fatalf("seed invitation: %v", err)
	}

	got, err := q.ApplicationForCalendarUID(ctx, ApplicationForCalendarUIDParams{
		UserID: mine, IcalUid: "derq-interview@ashbyhq.com",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Int64 != appID {
		t.Errorf("resolved application %d, want %d", got.Int64, appID)
	}

	// The same UID must not reach across accounts: one candidate's invitation says
	// nothing about another candidate's applications.
	if _, err := q.ApplicationForCalendarUID(ctx, ApplicationForCalendarUIDParams{
		UserID: theirs, IcalUid: "derq-interview@ashbyhq.com",
	}); err == nil {
		t.Error("a UID resolved an application for a user whose mail does not carry it")
	}
}

// The identifier has to survive the write, not just the parse. Adding a field to a
// generated params struct compiles whether or not any caller fills it, so the failure
// this guards against is silent: mail keeps arriving, the column stays empty, and the
// only automatic link the feature makes never fires.
func TestEmailUpserts_CarryTheCalendarUID(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "iv-carry@example.test", true)
	const uid = "carried@ashbyhq.com"
	at := ts(time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC))

	if err := q.UpsertEmail(ctx, UpsertEmailParams{
		UserID: user, ExternalID: "g-1", Subject: "Interview", ReceivedAt: at, IcalUid: uid,
	}); err != nil {
		t.Fatalf("gmail upsert: %v", err)
	}
	if err := q.InsertHostedMessage(ctx, InsertHostedMessageParams{
		UserID: user, ExternalID: "h-1", Subject: "Interview", ReceivedAt: at, IcalUid: uid,
	}); err != nil {
		t.Fatalf("hosted insert: %v", err)
	}

	for _, source := range []string{"gmail", "hosted"} {
		var got string
		if err := q.db.QueryRow(ctx,
			`SELECT ical_uid FROM emails WHERE user_id = $1 AND source = $2`, user, source).Scan(&got); err != nil {
			t.Fatalf("read %s: %v", source, err)
		}
		if got != uid {
			t.Errorf("%s stored ical_uid %q, want %q", source, got, uid)
		}
	}

	// The pushed tier refreshes content columns on re-push, and the meeting identifier
	// belongs to the message rather than to the reader — so it refreshes with them.
	for _, want := range []string{"", "pushed@ashbyhq.com"} {
		if _, err := q.UpsertExternalEmail(ctx, UpsertExternalEmailParams{
			UserID: user, ExternalID: "x-1", Subject: "Interview", ReceivedAt: at, IcalUid: want,
		}); err != nil {
			t.Fatalf("external upsert: %v", err)
		}
		var got string
		if err := q.db.QueryRow(ctx,
			`SELECT ical_uid FROM emails WHERE user_id = $1 AND source = 'external'`, user).Scan(&got); err != nil {
			t.Fatalf("read external: %v", err)
		}
		if got != want {
			t.Errorf("external stored ical_uid %q, want %q", got, want)
		}
	}
}
