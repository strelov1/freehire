//go:build integration

// Integration tests for the ledger's dated read — the statement behind
// internal/apptimeline and the calendar view.
//
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// linkedReply seeds a message against the caller's application to jobID and reconciles it
// into the ledger the way every link path does, returning the message's id.
func linkedReply(t *testing.T, q *Queries, userID, jobID int64, subject string, at time.Time) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := q.db.QueryRow(ctx,
		`INSERT INTO emails (user_id, source, external_id, subject, received_at, job_id, application_id, status_signal)
		 VALUES ($1, 'gmail', $2, $3, $4, $5,
		         (SELECT a.id FROM applications a WHERE a.user_id = $1 AND a.job_id = $5), 'acknowledgement')
		 RETURNING id`, userID, subject, subject, at, jobID).Scan(&id); err != nil {
		t.Fatalf("seed email %q: %v", subject, err)
	}
	if _, err := q.RetractSupersededEmailEvent(ctx, RetractSupersededEmailEventParams{ID: id, UserID: userID}); err != nil {
		t.Fatalf("retract: %v", err)
	}
	if err := q.RecordEmailApplicationEvent(ctx, RecordEmailApplicationEventParams{
		ID: id, UserID: userID, EventSource: "mail_gmail",
	}); err != nil {
		t.Fatalf("record reply event: %v", err)
	}
	return id
}

// wideRange lists every event the caller has, by asking for a span no test date escapes.
func wideRange(t *testing.T, q *Queries, userID int64) []ListApplicationEventsInRangeRow {
	t.Helper()
	rows, err := q.ListApplicationEventsInRange(context.Background(), ListApplicationEventsInRangeParams{
		UserID:      userID,
		FromAt:      ts(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
		ToAt:        ts(time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)),
		SrcGmail:    "mail_gmail",
		SrcHosted:   "mail_hosted",
		SrcExternal: "mail_external",
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	return rows
}

// The asymmetry the ledger is built on, seen from the read side. Deleting a message hides
// its content and does not un-happen the reply, so the event must still be served — with
// the subject withheld, because the reader asked to be rid of it.
func TestListApplicationEventsInRange_DeletionHidesTheSubjectNotTheEvent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "timeline-delete@example.test", true)
	job := seedResponseJob(t, q, "timeline-del-1", "derq")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	emailID := linkedReply(t, q, user, job, "Invitation to interview", time.Date(2026, 7, 14, 9, 30, 0, 0, time.UTC))

	reply := func() ListApplicationEventsInRangeRow {
		t.Helper()
		for _, r := range wideRange(t, q, user) {
			if r.Kind == "employer_reply" {
				return r
			}
		}
		t.Fatal("the employer_reply event is missing from the range")
		return ListApplicationEventsInRangeRow{}
	}

	before := reply()
	if before.EmailSubject.String != "Invitation to interview" || !before.EmailSubject.Valid {
		t.Errorf("a live message lent subject %q (valid=%v), want its own", before.EmailSubject.String, before.EmailSubject.Valid)
	}
	if before.EmailID.Int64 != emailID {
		t.Errorf("event points at email %d, want %d", before.EmailID.Int64, emailID)
	}

	if _, err := q.db.Exec(ctx, `UPDATE emails SET deleted_at = now() WHERE id = $1`, emailID); err != nil {
		t.Fatalf("delete the message: %v", err)
	}

	after := reply()
	if after.EmailSubject.Valid {
		t.Errorf("a deleted message still lent subject %q — deletion hides content", after.EmailSubject.String)
	}
	if after.EmailID.Valid {
		t.Errorf("a deleted message is still addressable as %d — the reader has no message to open", after.EmailID.Int64)
	}
	if !after.OccurredAt.Time.Equal(before.OccurredAt.Time) || after.CompanySlug != before.CompanySlug {
		t.Errorf("the event moved when its message was deleted: %v/%q, was %v/%q",
			after.OccurredAt.Time, after.CompanySlug, before.OccurredAt.Time, before.CompanySlug)
	}
}

// source_ref names an emails.id only for a mail-derived event — the table's own comment
// says so, and the idempotency index keys on (user_id, kind, source_ref) precisely because
// the referent is namespaced per kind. A read that joined emails on the bare column would
// hand the next kind's event whatever message happens to share its id.
func TestListApplicationEventsInRange_OnlyMailEventsBorrowAMessage(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "timeline-ref@example.test", true)
	job := seedResponseJob(t, q, "timeline-ref-1", "acme")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	emailID := linkedReply(t, q, user, job, "Unfortunately we are moving forward with others",
		time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC))

	// The shape a later kind takes: an event of the caller's own, carrying a source_ref
	// into some other table that happens to collide with one of their message ids.
	if _, err := q.db.Exec(ctx,
		`INSERT INTO application_events (user_id, job_id, company_slug, kind, signal, occurred_at, source, source_ref)
		 VALUES ($1, $2, 'acme', 'interview_scheduled', '', $3, 'user', $4)`,
		user, job, time.Date(2026, 7, 21, 15, 0, 0, 0, time.UTC), emailID); err != nil {
		t.Fatalf("seed the next kind's event: %v", err)
	}

	for _, r := range wideRange(t, q, user) {
		if r.Kind != "interview_scheduled" {
			continue
		}
		if r.EmailSubject.Valid || r.EmailID.Valid {
			t.Errorf("a non-mail event borrowed message %d (%q) — source_ref is only an emails.id for mail-derived events",
				r.EmailID.Int64, r.EmailSubject.String)
		}
		return
	}
	t.Fatal("the seeded event is missing from the range — an unrecognised kind must still be served")
}

// Nothing in the range read may mark mail read. read_at means "a human saw this", and a
// reader that browsed a month through the message endpoint would zero its owner's unread
// count without anyone opening anything.
func TestListApplicationEventsInRange_DoesNotMarkMailRead(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "timeline-unread@example.test", true)
	job := seedResponseJob(t, q, "timeline-unread-1", "acme")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	emailID := linkedReply(t, q, user, job, "Thanks for applying", time.Date(2026, 7, 9, 7, 0, 0, 0, time.UTC))

	if len(wideRange(t, q, user)) == 0 {
		t.Fatal("the range served nothing to read")
	}

	var readAt *time.Time
	if err := q.db.QueryRow(ctx, `SELECT read_at FROM emails WHERE id = $1`, emailID).Scan(&readAt); err != nil {
		t.Fatalf("read read_at: %v", err)
	}
	if readAt != nil {
		t.Errorf("reading the range marked the message read at %v — nobody opened it", *readAt)
	}
}

// A retracted event is excluded from every aggregate that reads the ledger, and the dated
// read is one of them: a correction that still showed the wrong employer on the calendar
// would be a correction the reader cannot see they made.
func TestListApplicationEventsInRange_RetractedEventsAreNotServed(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "timeline-retract@example.test", true)
	wrong := seedResponseJob(t, q, "timeline-ret-a", "workable")
	right := seedResponseJob(t, q, "timeline-ret-b", "derq")
	for _, j := range []int64{wrong, right} {
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: j, EventSource: "user"}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	emailID := linkedReply(t, q, user, wrong, "Thanks for applying", time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC))

	// The correction: the message belonged to the other employer all along.
	if _, err := q.db.Exec(ctx, `UPDATE emails SET job_id = $2,
		    application_id = (SELECT a.id FROM applications a
		                       WHERE a.user_id = emails.user_id AND a.job_id = $2)
		 WHERE emails.id = $1`, emailID, right); err != nil {
		t.Fatalf("relink: %v", err)
	}
	if _, err := q.RetractSupersededEmailEvent(ctx, RetractSupersededEmailEventParams{ID: emailID, UserID: user}); err != nil {
		t.Fatalf("retract: %v", err)
	}
	if err := q.RecordEmailApplicationEvent(ctx, RecordEmailApplicationEventParams{
		ID: emailID, UserID: user, EventSource: "mail_gmail",
	}); err != nil {
		t.Fatalf("re-record: %v", err)
	}

	var replies []ListApplicationEventsInRangeRow
	for _, r := range wideRange(t, q, user) {
		if r.Kind == "employer_reply" {
			replies = append(replies, r)
		}
	}
	if len(replies) != 1 || replies[0].CompanySlug != "derq" {
		t.Fatalf("the range served %d replies (first for %q), want exactly one for derq — the retracted row must not appear",
			len(replies), replies[0].CompanySlug)
	}
	// The correction moves who the reply belongs to, never when it arrived: the message's
	// own received_at is the date, and re-linking is not a new event in time.
	if want := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC); !replies[0].OccurredAt.Time.Equal(want) {
		t.Errorf("the replacement is dated %v, want the message's own %v", replies[0].OccurredAt.Time, want)
	}
}

// Someone else's events are not the caller's history. The ledger's per-user index is the
// only thing standing between the two, so the read must lead with it.
func TestListApplicationEventsInRange_ServesOnlyTheCallersOwnEvents(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	mine := seedResponseUser(t, q, "timeline-mine@example.test", true)
	theirs := seedResponseUser(t, q, "timeline-theirs@example.test", true)
	job := seedResponseJob(t, q, "timeline-shared", "acme")
	for _, u := range []int64{mine, theirs} {
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: u, JobID: job, EventSource: "user"}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}

	rows := wideRange(t, q, mine)
	if len(rows) != 1 {
		t.Fatalf("the caller's range holds %d events, want 1 — both users applied to the same posting", len(rows))
	}
}

// The bounds are the request. An event outside them is not a near miss to be rounded in:
// the caller is painting one month and the margin either side of it is deliberate.
func TestListApplicationEventsInRange_BoundsAreInclusiveAndExcludeTheRest(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "timeline-bounds@example.test", true)
	at := func(day int) time.Time { return time.Date(2026, 7, day, 12, 0, 0, 0, time.UTC) }
	for _, day := range []int{1, 15, 31} {
		job := seedResponseJob(t, q, fmt.Sprintf("timeline-bound-%d", day), "acme")
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{
			UserID: user, JobID: job, At: ts(at(day)), EventSource: "mail_hosted",
		}); err != nil {
			t.Fatalf("apply on day %d: %v", day, err)
		}
	}

	rows, err := q.ListApplicationEventsInRange(ctx, ListApplicationEventsInRangeParams{
		UserID: user, FromAt: ts(at(1)), ToAt: ts(at(15)),
		SrcGmail: "mail_gmail", SrcHosted: "mail_hosted", SrcExternal: "mail_external",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("a range covering two of three events served %d, want 2 — both bounds are inclusive", len(rows))
	}
	if !rows[0].OccurredAt.Time.Before(rows[1].OccurredAt.Time) {
		t.Errorf("events came back as %v then %v, want oldest first", rows[0].OccurredAt.Time, rows[1].OccurredAt.Time)
	}
}
