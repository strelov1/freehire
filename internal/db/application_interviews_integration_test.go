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
			Status:        "confirmed",
			Source:        "calendar_google",
			EventSource:   "calendar_google",
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
		ProviderEventID: "evt-cancel-me",
		StartsAt:        ts(time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)),
		Status:          "confirmed", Source: "calendar_google", EventSource: "calendar_google",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if _, err := q.CancelApplicationInterview(ctx, CancelApplicationInterviewParams{
		UserID: user, EventID: "cancel-me@ashbyhq.com",
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

// The link the whole feature turns on, assembled the way the sync reads it: one query
// per candidate, each application carrying the identifiers of the invitations already
// tied to it. calmatch then compares a calendar entry's own identifier against those.
func TestListCalendarMatchCandidates_CarriesTheLinkedInvitationsIdentifiers(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	mine := seedResponseUser(t, q, "iv-uid@example.test", true)
	theirs := seedResponseUser(t, q, "iv-uid-other@example.test", true)
	jobID, appID := seedApplication(t, q, mine, "iv-uid-1", "derq")
	seedApplication(t, q, mine, "iv-uid-2", "vercel") // no invitation, so no identifier

	if _, err := q.db.Exec(ctx,
		`INSERT INTO emails (user_id, source, external_id, subject, received_at, job_id, application_id, ical_uid, status_signal)
		 VALUES ($1, 'gmail', 'inv-1', 'Interview', now(), $2, $3, 'derq-interview@ashbyhq.com', 'interview_invitation')`,
		mine, jobID, appID); err != nil {
		t.Fatalf("seed invitation: %v", err)
	}

	got, err := q.ListCalendarMatchCandidates(ctx, mine)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2", len(got))
	}
	byApp := map[int64][]string{}
	for _, c := range got {
		byApp[c.ApplicationID] = c.IcalUids
	}
	if uids := byApp[appID]; len(uids) != 1 || uids[0] != "derq-interview@ashbyhq.com" {
		t.Errorf("the invited application carries %v, want its one identifier", uids)
	}
	for id, uids := range byApp {
		if id != appID && len(uids) != 0 {
			t.Errorf("application %d carries %v, want none — it has no invitation", id, uids)
		}
	}

	// One candidate's invitation says nothing about another's applications.
	other, err := q.ListCalendarMatchCandidates(ctx, theirs)
	if err != nil {
		t.Fatalf("list candidates for the other user: %v", err)
	}
	if len(other) != 0 {
		t.Errorf("a user with no applications got %d candidates", len(other))
	}
}

// The identifier that linked a meeting is a fact, and a later sync that only recognises
// the title has learned nothing new. Letting it downgrade would flip a settled interview
// back into a question the candidate has to answer again, every run.
func TestUpsertApplicationInterview_AConfirmedMeetingNeverFallsBackToASuggestion(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "iv-status@example.test", true)
	_, appID := seedApplication(t, q, user, "iv-status-1", "derq")
	at := ts(time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC))
	upsert := func(status string) {
		t.Helper()
		if _, err := q.UpsertApplicationInterview(ctx, UpsertApplicationInterviewParams{
			UserID: user, ApplicationID: appID, IcalUid: "status@ashbyhq.com",
			StartsAt: at, Status: status, Source: "calendar_google", EventSource: "calendar_google",
		}); err != nil {
			t.Fatalf("upsert %s: %v", status, err)
		}
	}
	read := func() string {
		t.Helper()
		var s string
		if err := q.db.QueryRow(ctx,
			`SELECT status FROM application_interviews WHERE user_id = $1`, user).Scan(&s); err != nil {
			t.Fatalf("read status: %v", err)
		}
		return s
	}

	upsert("suggested")
	if got := read(); got != "suggested" {
		t.Fatalf("status = %q, want suggested", got)
	}
	upsert("confirmed") // the invitation's identifier turned up
	if got := read(); got != "confirmed" {
		t.Fatalf("status = %q, want confirmed", got)
	}
	upsert("suggested") // a later run recognises only the title
	if got := read(); got != "confirmed" {
		t.Errorf("status = %q, want it to stay confirmed", got)
	}
}

// The appointment and the record of it being made ride in one statement, so they cannot
// drift. The event is dated by the observation and written once: a reschedule moves the
// meeting, and the scheduling still happened only the one time.
func TestUpsertApplicationInterview_NotesTheSchedulingOnceInTheLedger(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "iv-ledger@example.test", true)
	_, appID := seedApplication(t, q, user, "iv-ledger-1", "derq")
	upsert := func(at time.Time) {
		t.Helper()
		if _, err := q.UpsertApplicationInterview(ctx, UpsertApplicationInterviewParams{
			UserID: user, ApplicationID: appID, IcalUid: "ledger@ashbyhq.com",
			StartsAt: ts(at), Status: "confirmed",
			Source: "calendar_google", EventSource: "calendar_google",
		}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	upsert(time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC))
	upsert(time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)) // moved

	var events int
	var occurred time.Time
	if err := q.db.QueryRow(ctx,
		`SELECT count(*), coalesce(max(occurred_at), now()) FROM application_events
		  WHERE user_id = $1 AND kind = 'interview_scheduled'`, user).Scan(&events, &occurred); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if events != 1 {
		t.Fatalf("a reschedule produced %d interview_scheduled events, want 1 — the scheduling happened once", events)
	}
	// Dated by the observation, so nothing in the ledger sits in the future. The meeting
	// itself is in August 2026; this row must not be.
	if occurred.After(time.Now().Add(time.Minute)) {
		t.Errorf("the ledger event is dated %v, in the future — occurred_at means when it happened", occurred)
	}
}

// A calendar-only grant is not a mailbox, and every reader that treats it as one causes
// a different injury. The mail sync would call an API its token cannot answer and take
// the 403 as a revoked grant, flipping the SHARED status and killing the calendar sync
// the candidate actually asked for. The response-rate rollup would count their
// applications as observable — in the denominator, structurally barred from the
// numerator, making named employers read as more silent than they are.
func TestACalendarOnlyGrantIsNotAMailbox(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	withMail := seedResponseUser(t, q, "grant-mail@example.test", true)
	calendarOnly := seedResponseUser(t, q, "grant-cal@example.test", false)
	if err := q.UpsertCalendarGrant(ctx, UpsertCalendarGrantParams{
		UserID:          calendarOnly,
		RefreshTokenEnc: "enc",
		Scopes:          []string{"https://www.googleapis.com/auth/calendar.readonly"},
	}); err != nil {
		t.Fatalf("grant the calendar: %v", err)
	}

	// The mail sync must not see them.
	rows, err := q.ListConnectedGmailUsers(ctx)
	if err != nil {
		t.Fatalf("list connected: %v", err)
	}
	for _, r := range rows {
		if r.UserID == calendarOnly {
			t.Error("the mail sync picked up a calendar-only grant; its 403 would revoke the shared connection")
		}
	}
	var sawMailUser bool
	for _, r := range rows {
		if r.UserID == withMail {
			sawMailUser = true
		}
	}
	if !sawMailUser {
		t.Error("the mail sync lost a real mailbox — the predicate is too narrow")
	}

	// And the calendar sync must see them.
	cal, err := q.ListCalendarConnections(ctx, "https://www.googleapis.com/auth/calendar.readonly")
	if err != nil {
		t.Fatalf("list calendar connections: %v", err)
	}
	if len(cal) != 1 || cal[0] != calendarOnly {
		t.Errorf("calendar connections = %v, want just the calendar-only user %d", cal, calendarOnly)
	}
}

// A title match is a guess offered to the candidate, and a row in the ledger carrying an
// application id is a link — into an append-only table with no retraction path for this
// kind. Letting a guess write one would make "Q3 ramp-up planning" a permanent,
// unremovable interview against an application to an employer called Ramp.
func TestUpsertApplicationInterview_ASuggestionWritesNoLedgerLink(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "iv-suggest@example.test", true)
	_, appID := seedApplication(t, q, user, "iv-suggest-1", "ramp")
	upsert := func(status string) {
		t.Helper()
		if _, err := q.UpsertApplicationInterview(ctx, UpsertApplicationInterviewParams{
			UserID: user, ApplicationID: appID, IcalUid: "guess@google.com",
			StartsAt: ts(time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)),
			Status:   status, Source: "calendar_google", EventSource: "calendar_google",
		}); err != nil {
			t.Fatalf("upsert %s: %v", status, err)
		}
	}
	events := func() int {
		t.Helper()
		var n int
		if err := q.db.QueryRow(ctx,
			`SELECT count(*) FROM application_events WHERE user_id = $1 AND kind = 'interview_scheduled'`,
			user).Scan(&n); err != nil {
			t.Fatalf("count events: %v", err)
		}
		return n
	}

	upsert("suggested")
	if got := events(); got != 0 {
		t.Fatalf("a suggestion wrote %d ledger events, want none — only the identifier may link", got)
	}
	// Confirming it later does write one: the identifier turned up, and now it is a fact.
	upsert("confirmed")
	if got := events(); got != 1 {
		t.Errorf("confirming the meeting wrote %d ledger events, want 1", got)
	}
}

// Google guarantees a deleted event carries only its own id — no iCalUID, no time, no
// title. A cancellation therefore has to be able to find the meeting by the provider's
// identifier alone, or a called-off interview stands on the calendar forever.
func TestCancelApplicationInterview_FindsTheMeetingByTheProvidersIdAlone(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "iv-provider-cancel@example.test", true)
	_, appID := seedApplication(t, q, user, "iv-provider-1", "derq")
	if _, err := q.UpsertApplicationInterview(ctx, UpsertApplicationInterviewParams{
		UserID: user, ApplicationID: appID, IcalUid: "round-1@ashbyhq.com",
		ProviderEventID: "evt-abc123",
		StartsAt:        ts(time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)),
		Status:          "confirmed", Source: "calendar_google", EventSource: "calendar_google",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	rows, err := q.CancelApplicationInterview(ctx, CancelApplicationInterviewParams{
		UserID: user, EventID: "evt-abc123",
	})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if rows != 1 {
		t.Fatalf("cancelling by the provider id touched %d rows, want 1", rows)
	}
	var status string
	if err := q.db.QueryRow(ctx,
		`SELECT status FROM application_interviews WHERE user_id = $1`, user).Scan(&status); err != nil {
		t.Fatalf("read: %v", err)
	}
	if status != "cancelled" {
		t.Errorf("status = %q, want cancelled", status)
	}
}

// A ledger row is only as good as its provenance, and application_events.source has no
// CHECK constraint — an empty one is accepted silently and then reads as an unknown
// source, which TrustedForDayMath refuses. Three tests here wrote exactly that without
// noticing until review; this one makes the omission visible.
func TestUpsertApplicationInterview_WritesTheLedgerWithARealSource(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "iv-source@example.test", true)
	_, appID := seedApplication(t, q, user, "iv-source-1", "derq")
	if _, err := q.UpsertApplicationInterview(ctx, UpsertApplicationInterviewParams{
		UserID: user, ApplicationID: appID, IcalUid: "src@ashbyhq.com",
		StartsAt: ts(time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)),
		Status:   "confirmed", Source: "calendar_google", EventSource: "calendar_google",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	var source string
	if err := q.db.QueryRow(ctx,
		`SELECT source FROM application_events WHERE user_id = $1 AND kind = 'interview_scheduled'`,
		user).Scan(&source); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if source != "calendar_google" {
		t.Errorf("ledger source = %q, want calendar_google — an empty one reads as unknown provenance", source)
	}
}
