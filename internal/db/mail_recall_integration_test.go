//go:build integration

// Integration tests for the pull direction of mail-to-application linking: from an
// application, the mail that might belong to it.
//
// The two queries under test carry the whole safety story. The net decides what may be
// looked at; the write decides what may be changed. Neither trusts its caller.
//
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// seedRecallEmail inserts one message with the attachment state the caller names, so a
// test reads as the state it is about rather than as a column list.
func seedRecallEmail(t *testing.T, q *Queries, userID int64, extID string, received time.Time, mutate string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := q.db.QueryRow(context.Background(),
		`INSERT INTO emails (user_id, source, external_id, subject, body_text, received_at)
		 VALUES ($1, 'gmail', $2, 'Your application', 'hello', $3) RETURNING id`,
		userID, extID, received).Scan(&id); err != nil {
		t.Fatalf("seed email %s: %v", extID, err)
	}
	if mutate != "" {
		if _, err := q.db.Exec(context.Background(), mutate, append([]any{id}, args...)...); err != nil {
			t.Fatalf("seed email %s state: %v", extID, err)
		}
	}
	return id
}

func recallIDs(t *testing.T, q *Queries, userID int64, since time.Time, limit int32) map[int64]bool {
	t.Helper()
	rows, err := q.ListEmailsForRecall(context.Background(), ListEmailsForRecallParams{
		UserID: userID, Since: ts(since), Lim: limit,
	})
	if err != nil {
		t.Fatalf("list for recall: %v", err)
	}
	got := make(map[int64]bool, len(rows))
	for _, r := range rows {
		got[r.ID] = true
	}
	return got
}

// The net's whole job is deciding what may be looked at. Mail already attached to an
// application is out of scope — re-linking retracts and re-records on a public response
// rate, and this path is not allowed to reach that.
func TestListEmailsForRecall_TakesOnlyUnattachedMailInsideTheWindow(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	user := seedResponseUser(t, q, "recall-net@example.test", true)
	_, appID := seedApplication(t, q, user, "recall-net-1", "derq")
	applied := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	since := applied.AddDate(0, 0, -7)

	unlinked := seedRecallEmail(t, q, user, "recall-unlinked", applied.Add(time.Hour), "")
	suggested := seedRecallEmail(t, q, user, "recall-suggested", applied.Add(2*time.Hour),
		`UPDATE emails SET suggested_job_id = (SELECT job_id FROM applications WHERE id = $2) WHERE id = $1`, appID)
	linked := seedRecallEmail(t, q, user, "recall-linked", applied.Add(3*time.Hour),
		`UPDATE emails SET application_id = $2 WHERE id = $1`, appID)
	deleted := seedRecallEmail(t, q, user, "recall-deleted", applied.Add(4*time.Hour),
		`UPDATE emails SET deleted_at = now() WHERE id = $1`)
	tooOld := seedRecallEmail(t, q, user, "recall-old", since.AddDate(0, 0, -1), "")

	got := recallIDs(t, q, user, since, 40)

	for id, what := range map[int64]string{
		unlinked:  "unlinked mail",
		suggested: "mail carrying an unconfirmed suggestion",
	} {
		if !got[id] {
			t.Errorf("%s was not in the net, and it is exactly what the net is for", what)
		}
	}
	for id, what := range map[int64]string{
		linked:  "mail already attached to an application",
		deleted: "soft-deleted mail",
		tooOld:  "mail older than the window",
	} {
		if got[id] {
			t.Errorf("%s reached the net", what)
		}
	}
}

// One caller's mailbox is not another's, and the limit is what bounds a run's cost.
func TestListEmailsForRecall_IsScopedToTheCallerAndCapped(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	mine := seedResponseUser(t, q, "recall-mine@example.test", true)
	theirs := seedResponseUser(t, q, "recall-theirs@example.test", true)
	at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	since := at.AddDate(0, 0, -7)

	for i := range 3 {
		seedRecallEmail(t, q, mine, fmt.Sprintf("recall-mine-%d", i), at.Add(time.Duration(i)*time.Hour), "")
	}
	stranger := seedRecallEmail(t, q, theirs, "recall-theirs-1", at, "")

	if got := recallIDs(t, q, mine, since, 40); got[stranger] {
		t.Error("another user's mail reached the net")
	}
	if got := recallIDs(t, q, mine, since, 2); len(got) != 2 {
		t.Errorf("limit 2 returned %d rows — the cap is what bounds a run's cost", len(got))
	}
}

// The predicate in the statement is the guard, not an optimisation: even if the net, the
// model and the service layer all went wrong at once, a linked message is unreachable.
func TestSuggestApplicationForEmail_CannotTouchLinkedMail(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "recall-guard@example.test", true)
	ownerJob, ownerApp := seedApplication(t, q, user, "recall-guard-owner", "derq")
	otherJob, _ := seedApplication(t, q, user, "recall-guard-other", "ramp")
	at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	linked := seedRecallEmail(t, q, user, "recall-guard-1", at,
		`UPDATE emails SET job_id = (SELECT job_id FROM applications WHERE id = $2), application_id = $2 WHERE id = $1`, ownerApp)

	rows, err := q.SuggestApplicationForEmail(ctx, SuggestApplicationForEmailParams{
		ID: linked, UserID: user, SuggestedJobID: pgtype.Int8{Int64: otherJob, Valid: true}, Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if rows != 0 {
		t.Fatalf("the write changed %d rows on mail that is already linked, want 0", rows)
	}

	var jobID int64
	var suggested *int64
	if err := q.db.QueryRow(ctx,
		`SELECT job_id, suggested_job_id FROM emails WHERE id = $1`, linked).Scan(&jobID, &suggested); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if jobID != ownerJob {
		t.Errorf("the link moved to job %d, want %d — this path may not relink", jobID, ownerJob)
	}
	if suggested != nil {
		t.Errorf("a suggestion was planted on linked mail (job %d)", *suggested)
	}
}

// A suggestion is a proposal, not a decision. The caller asked about THIS application
// explicitly, so an unconfirmed proposal naming another one gives way.
func TestSuggestApplicationForEmail_ReplacesAnUnconfirmedSuggestion(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "recall-replace@example.test", true)
	wantedJob, _ := seedApplication(t, q, user, "recall-replace-wanted", "derq")
	staleJob, _ := seedApplication(t, q, user, "recall-replace-stale", "ramp")
	at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	email := seedRecallEmail(t, q, user, "recall-replace-1", at,
		`UPDATE emails SET suggested_job_id = $2 WHERE id = $1`, staleJob)

	rows, err := q.SuggestApplicationForEmail(ctx, SuggestApplicationForEmailParams{
		ID: email, UserID: user, SuggestedJobID: pgtype.Int8{Int64: wantedJob, Valid: true}, Confidence: 0.82,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if rows != 1 {
		t.Fatalf("the write changed %d rows, want 1", rows)
	}

	var suggested int64
	var confidence float32
	if err := q.db.QueryRow(ctx,
		`SELECT suggested_job_id, match_confidence FROM emails WHERE id = $1`, email).Scan(&suggested, &confidence); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if suggested != wantedJob {
		t.Errorf("suggestion names job %d, want %d", suggested, wantedJob)
	}
	if confidence != 0.82 {
		t.Errorf("confidence %v, want 0.82", confidence)
	}
}

// Someone else's message is not the caller's to propose anything about.
func TestSuggestApplicationForEmail_IsScopedToTheCaller(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	mine := seedResponseUser(t, q, "recall-scope-mine@example.test", true)
	theirs := seedResponseUser(t, q, "recall-scope-theirs@example.test", true)
	job, _ := seedApplication(t, q, mine, "recall-scope-1", "derq")
	stranger := seedRecallEmail(t, q, theirs, "recall-scope-stranger", time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC), "")

	rows, err := q.SuggestApplicationForEmail(context.Background(), SuggestApplicationForEmailParams{
		ID: stranger, UserID: mine, SuggestedJobID: pgtype.Int8{Int64: job, Valid: true}, Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if rows != 0 {
		t.Fatalf("the write reached another user's mail (%d rows)", rows)
	}
}
