//go:build integration

// Integration tests for the application-event ledger's two structural claims: that a
// mail-derived event is idempotent under replay without any coordination between the
// worker and the backfill, and that retraction stamps rather than deletes. Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ts lives in reminders_integration_test.go — same package, one helper.
func i8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }

func countEvents(t *testing.T, q *Queries, userID int64) int {
	t.Helper()
	var n int
	if err := q.db.QueryRow(context.Background(),
		`SELECT count(*) FROM application_events WHERE user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// The worker and the backfill can both reach the same email. Nothing serializes them, by
// design — the alternative was a flock between two processes, the mechanism that has
// already deadlocked reindex against reindex-companies twice.
func TestRecordApplicationEvent_MailEventIsIdempotentUnderReplay(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "replay@example.test", true)
	job := seedResponseJob(t, q, "replay-1", "acme")
	arg := RecordApplicationEventParams{
		UserID:      user,
		JobID:       i8(job),
		CompanySlug: "acme",
		Kind:        "employer_reply",
		Signal:      "rejection",
		OccurredAt:  ts(time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)),
		Source:      "mail_gmail",
		SourceRef:   i8(4242),
	}

	for i := 0; i < 3; i++ {
		if err := q.RecordApplicationEvent(ctx, arg); err != nil {
			t.Fatalf("record attempt %d: %v", i+1, err)
		}
	}
	if got := countEvents(t, q, user); got != 1 {
		t.Errorf("three identical recordings produced %d rows, want 1", got)
	}
}

// A second chase is a second fact. The single followed_up_at column used to erase the
// first one; manual events carry no source_ref and so sit outside the dedup index.
func TestRecordApplicationEvent_ManualEventsAreNotDeduplicated(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "chase@example.test", true)
	job := seedResponseJob(t, q, "chase-1", "acme")
	chase := func(day int) RecordApplicationEventParams {
		return RecordApplicationEventParams{
			UserID:      user,
			JobID:       i8(job),
			CompanySlug: "acme",
			Kind:        "follow_up_sent",
			OccurredAt:  ts(time.Date(2026, 7, day, 9, 0, 0, 0, time.UTC)),
			Source:      "user",
		}
	}
	for _, day := range []int{3, 17} {
		if err := q.RecordApplicationEvent(ctx, chase(day)); err != nil {
			t.Fatalf("record chase on day %d: %v", day, err)
		}
	}

	events, err := q.ListApplicationEventsForUserJob(ctx, ListApplicationEventsForUserJobParams{UserID: user, JobID: i8(job)})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("two chases produced %d events, want 2", len(events))
	}
	if got := events[0].OccurredAt.Time.Day(); got != 3 {
		t.Errorf("oldest event is from day %d, want the first chase on day 3 — the earlier chase must stay readable", got)
	}
}

// Retraction is the link-correction path. The row stays: an event recorded in error is
// itself a fact, and the ledger is append-only.
func TestRetractApplicationEvents_StampsRatherThanDeletes(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "mislink@example.test", true)
	job := seedResponseJob(t, q, "mislink-1", "workable")
	if err := q.RecordApplicationEvent(ctx, RecordApplicationEventParams{
		UserID:      user,
		JobID:       i8(job),
		CompanySlug: "workable",
		Kind:        "employer_reply",
		Signal:      "acknowledgement",
		OccurredAt:  ts(time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)),
		Source:      "mail_gmail",
		SourceRef:   i8(77),
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	n, err := q.RetractApplicationEventsBySourceRef(ctx, RetractApplicationEventsBySourceRefParams{
		UserID: user, Kind: "employer_reply", SourceRef: i8(77),
	})
	if err != nil {
		t.Fatalf("retract: %v", err)
	}
	if n != 1 {
		t.Fatalf("retracted %d rows, want 1", n)
	}
	if got := countEvents(t, q, user); got != 1 {
		t.Errorf("retraction left %d rows, want the row to survive as 1", got)
	}

	events, err := q.ListApplicationEventsForUserJob(ctx, ListApplicationEventsForUserJobParams{UserID: user, JobID: i8(job)})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("a retracted event is still listed as live (%d rows)", len(events))
	}

	again, err := q.RetractApplicationEventsBySourceRef(ctx, RetractApplicationEventsBySourceRefParams{
		UserID: user, Kind: "employer_reply", SourceRef: i8(77),
	})
	if err != nil {
		t.Fatalf("retract again: %v", err)
	}
	if again != 0 {
		t.Errorf("a repeated correction touched %d rows, want 0 — the stamp must not move forward", again)
	}
}
