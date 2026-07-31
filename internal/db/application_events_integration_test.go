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

// The applied event is written by MarkJobApplied's own statement, under the same
// predicate as the applied_count bump. Re-applying refreshes the timestamp and must not
// produce a second event, or the ledger and the counter would drift apart — which is
// exactly what a separate write would eventually do.
func TestMarkJobApplied_RecordsOneEventPerTransition(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "applied@example.test", true)
	job := seedResponseJob(t, q, "applied-1", "acme")
	arg := MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}

	for i := 0; i < 3; i++ {
		if _, err := q.MarkJobApplied(ctx, arg); err != nil {
			t.Fatalf("apply attempt %d: %v", i+1, err)
		}
	}

	var events, appliedCount int
	if err := q.db.QueryRow(ctx,
		`SELECT count(*) FROM application_events WHERE user_id = $1 AND kind = 'applied'`, user).Scan(&events); err != nil {
		t.Fatalf("count applied events: %v", err)
	}
	if err := q.db.QueryRow(ctx, `SELECT applied_count FROM jobs WHERE id = $1`, job).Scan(&appliedCount); err != nil {
		t.Fatalf("read applied_count: %v", err)
	}
	if events != 1 {
		t.Errorf("three applies produced %d applied events, want 1", events)
	}
	if appliedCount != events {
		t.Errorf("applied_count is %d but the ledger holds %d applied events — the two records of one transition disagree", appliedCount, events)
	}
}

// The ledger holds transitions, not writes. Re-setting the stage a row already carries,
// or editing only the note, records nothing — otherwise "how long did this stage last"
// would be unanswerable, drowned in rows that describe no movement.
func TestTrackJob_RecordsOnlyRealStageTransitions(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "stages@example.test", true)
	job := seedResponseJob(t, q, "stages-1", "acme")
	track := func(stage, notes *string) {
		t.Helper()
		if _, err := q.TrackJob(ctx, TrackJobParams{
			UserID: user, JobID: job,
			Stage:       pgtype.Text{String: derefOr(stage), Valid: stage != nil},
			Notes:       pgtype.Text{String: derefOr(notes), Valid: notes != nil},
			EventSource: "user",
		}); err != nil {
			t.Fatalf("track: %v", err)
		}
	}
	sPtr := func(s string) *string { return &s }

	track(sPtr("applied"), nil)      // transition: nothing -> applied
	track(sPtr("applied"), nil)      // no movement
	track(nil, sPtr("call Tuesday")) // a note is not a transition
	track(sPtr("interview"), nil)    // transition: applied -> interview

	rows, err := q.db.Query(ctx,
		`SELECT signal FROM application_events
		  WHERE user_id = $1 AND kind = 'stage_set' ORDER BY id`, user)
	if err != nil {
		t.Fatalf("read stage events: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, s)
	}
	want := []string{"applied", "interview"}
	if len(got) != len(want) {
		t.Fatalf("recorded %v, want %v — one row per movement, not per write", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func derefOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// An application reconstructed from mail is dated by the message. The event must carry
// that date too: reading now() here would compress a year of recovered history into the
// day the mailbox was connected.
func TestMarkJobApplied_DatedRecordingCarriesItsDateIntoTheLedger(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "dated@example.test", true)
	job := seedResponseJob(t, q, "dated-1", "acme")
	wrote := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{
		UserID: user, JobID: job, At: ts(wrote), EventSource: "mail_hosted",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	var occurred time.Time
	var source, slug string
	if err := q.db.QueryRow(ctx,
		`SELECT occurred_at, source, company_slug FROM application_events
		  WHERE user_id = $1 AND kind = 'applied'`, user).Scan(&occurred, &source, &slug); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if !occurred.Equal(wrote) {
		t.Errorf("event occurred_at = %v, want the message's %v", occurred, wrote)
	}
	if source != "mail_hosted" {
		t.Errorf("event source = %q, want mail_hosted", source)
	}
	if slug != "acme" {
		t.Errorf("event company_slug = %q, want the job's acme — the slug is denormalized so cmd/prune cannot orphan it", slug)
	}
}

// The single followed_up_at column cannot tell a resubmit from a real second chase — it
// just overwrites. The ledger can, and must: the first chase is a fact the board's own
// documentation calls a deliberate decision.
func TestRecordApplicationFollowUp_SuppressesAResubmitButNotASecondChase(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "followup@example.test", true)
	job := seedResponseJob(t, q, "followup-1", "acme")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	chase := func() {
		t.Helper()
		if _, err := q.RecordApplicationFollowUp(ctx, RecordApplicationFollowUpParams{
			UserID: user, JobID: job, EventSource: "user",
		}); err != nil {
			t.Fatalf("record follow-up: %v", err)
		}
	}

	chase()
	chase() // the resubmit, within the hour

	count := func() int {
		t.Helper()
		var n int
		if err := q.db.QueryRow(ctx,
			`SELECT count(*) FROM application_events WHERE user_id = $1 AND kind = 'follow_up_sent'`,
			user).Scan(&n); err != nil {
			t.Fatalf("count chases: %v", err)
		}
		return n
	}
	if got := count(); got != 1 {
		t.Fatalf("a double submit recorded %d chases, want 1", got)
	}

	// Age the recorded chase past the window; the next one is a genuine second attempt.
	if _, err := q.db.Exec(ctx,
		`UPDATE user_jobs SET followed_up_at = now() - interval '9 days'
		  WHERE user_id = $1 AND job_id = $2`, user, job); err != nil {
		t.Fatalf("age the chase: %v", err)
	}
	chase()
	if got := count(); got != 2 {
		t.Errorf("a second chase nine days later recorded %d events in total, want 2 — the first must survive", got)
	}
}

// The load-bearing asymmetry. Deleting a message hides content and leaves the fact
// standing; re-linking asserts the fact belongs to another employer and must move it.
// Getting this wrong in either direction is a defect on a public page: a deletion that
// retracted would let inbox hygiene rewrite a company's rate, and a re-link that did not
// would leave the wrong company's rate poisoned forever — the case the mail stack met
// when a catalogue company sharing an ATS brand name collected twenty-three
// acknowledgements belonging to other employers.
func TestSyncEmailApplicationEvent_DeletionKeepsTheFactRelinkMovesIt(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "sync@example.test", true)
	jobA := seedResponseJob(t, q, "sync-a", "workable")
	jobB := seedResponseJob(t, q, "sync-b", "derq")

	var emailID int64
	if err := q.db.QueryRow(ctx,
		`INSERT INTO emails (user_id, source, external_id, received_at, job_id, status_signal)
		 VALUES ($1, 'gmail', 'm-1', now() - interval '3 days', $2, 'acknowledgement')
		 RETURNING id`, user, jobA).Scan(&emailID); err != nil {
		t.Fatalf("seed email: %v", err)
	}
	sync := func() {
		t.Helper()
		if _, err := q.RetractSupersededEmailEvent(ctx, RetractSupersededEmailEventParams{ID: emailID, UserID: user}); err != nil {
			t.Fatalf("retract: %v", err)
		}
		if err := q.RecordEmailApplicationEvent(ctx, RecordEmailApplicationEventParams{
			ID: emailID, UserID: user, EventSource: "mail_gmail",
		}); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	live := func() (slug string, n int) {
		t.Helper()
		if err := q.db.QueryRow(ctx,
			`SELECT coalesce(max(company_slug), ''), count(*) FROM application_events
			  WHERE user_id = $1 AND kind = 'employer_reply' AND retracted_at IS NULL`,
			user).Scan(&slug, &n); err != nil {
			t.Fatalf("read live events: %v", err)
		}
		return
	}

	sync()
	sync() // idempotent
	if slug, n := live(); n != 1 || slug != "workable" {
		t.Fatalf("after two syncs: %d live events for %q, want 1 for workable", n, slug)
	}

	// Deleting the message must change nothing.
	if _, err := q.db.Exec(ctx, `UPDATE emails SET deleted_at = now() WHERE id = $1`, emailID); err != nil {
		t.Fatalf("delete email: %v", err)
	}
	sync()
	if _, n := live(); n != 1 {
		t.Errorf("deleting the message left %d live events, want the fact to stand at 1", n)
	}

	// Re-linking to the right employer must move it.
	if _, err := q.db.Exec(ctx, `UPDATE emails SET job_id = $2 WHERE id = $1`, emailID, jobB); err != nil {
		t.Fatalf("relink: %v", err)
	}
	sync()
	slug, n := live()
	if n != 1 || slug != "derq" {
		t.Errorf("after the correction: %d live events for %q, want 1 for derq", n, slug)
	}
	var retracted int
	if err := q.db.QueryRow(ctx,
		`SELECT count(*) FROM application_events
		  WHERE user_id = $1 AND company_slug = 'workable' AND retracted_at IS NOT NULL`,
		user).Scan(&retracted); err != nil {
		t.Fatalf("count retracted: %v", err)
	}
	if retracted != 1 {
		t.Errorf("the mislinked event was retracted %d times, want 1 — the row must survive as evidence", retracted)
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

// The backfill replays only what carries a real date, and re-running it must add nothing:
// the pass is restarted after an interruption rather than repaired, and it may run while
// cmd/classify-mail is working on the same mail.
func TestBackfill_ReplaysDatedFactsOnlyAndIsIdempotent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "backfill@example.test", true)
	job := seedResponseJob(t, q, "backfill-1", "acme")
	applied := time.Date(2026, 5, 2, 8, 0, 0, 0, time.UTC)
	chased := time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC)
	replied := time.Date(2026, 5, 30, 8, 0, 0, 0, time.UTC)

	// A tracked application with a stage the user set by hand, a chase, and one linked,
	// classified, and since-deleted reply.
	if _, err := q.db.Exec(ctx,
		`INSERT INTO user_jobs (user_id, job_id, applied_at, followed_up_at, stage)
		 VALUES ($1, $2, $3, $4, 'interview')`, user, job, applied, chased); err != nil {
		t.Fatalf("seed application: %v", err)
	}
	if _, err := q.db.Exec(ctx,
		`INSERT INTO emails (user_id, source, external_id, received_at, job_id, status_signal, deleted_at)
		 VALUES ($1, 'gmail', 'bf-1', $2, $3, 'interview_invitation', now())`, user, replied, job); err != nil {
		t.Fatalf("seed email: %v", err)
	}

	replay := func() {
		t.Helper()
		if _, err := q.BackfillEmployerReplyEvents(ctx, BackfillEmployerReplyEventsParams{
			ID: 0, BatchSize: 100,
			SrcGmail: "mail_gmail", SrcHosted: "mail_hosted", SrcExternal: "mail_external",
		}); err != nil {
			t.Fatalf("replay replies: %v", err)
		}
		if _, err := q.BackfillAppliedEvents(ctx, BackfillAppliedEventsParams{
			LastUserID: 0, LastJobID: 0, BatchSize: 100, EventSource: "user",
		}); err != nil {
			t.Fatalf("replay applications: %v", err)
		}
	}

	replay()
	replay() // idempotent

	kinds := map[string]int{}
	rows, err := q.db.Query(ctx,
		`SELECT kind, count(*) FROM application_events WHERE user_id = $1 GROUP BY kind`, user)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		kinds[k] = n
	}

	for kind, want := range map[string]int{"applied": 1, "follow_up_sent": 1, "employer_reply": 1} {
		if kinds[kind] != want {
			t.Errorf("%s events = %d, want %d (two passes must produce the same as one)", kind, kinds[kind], want)
		}
	}
	if kinds["stage_set"] != 0 {
		t.Errorf("the backfill invented %d stage_set events; the stage column carries no transition date, so any date given to one would be fabricated", kinds["stage_set"])
	}

	// The deleted message's reply is present, at its own date.
	var occurred time.Time
	if err := q.db.QueryRow(ctx,
		`SELECT occurred_at FROM application_events WHERE user_id = $1 AND kind = 'employer_reply'`,
		user).Scan(&occurred); err != nil {
		t.Fatalf("read reply event: %v", err)
	}
	if !occurred.Equal(replied) {
		t.Errorf("reply event dated %v, want the message's %v", occurred, replied)
	}
}
