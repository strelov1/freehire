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
	"slices"
	"testing"
	"time"
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

func recallRows(t *testing.T, q *Queries, userID int64, since time.Time, limit int32) []ListEmailsForRecallRow {
	t.Helper()
	rows, err := q.ListEmailsForRecall(context.Background(), ListEmailsForRecallParams{
		UserID: userID, Since: ts(since), Until: ts(since.AddDate(0, 0, 97)), Lim: limit,
	})
	if err != nil {
		t.Fatalf("list for recall: %v", err)
	}
	return rows
}

// recallIDs keeps the query's order. A set would have been enough for the membership
// tests and wrong for the cap: ORDER BY plus LIMIT is what decides WHICH candidates a run
// spends its one model call on, and a set cannot tell a reversed sort from a correct one.
func recallIDs(t *testing.T, q *Queries, userID int64, since time.Time, limit int32) []int64 {
	t.Helper()
	rows := recallRows(t, q, userID, since, limit)
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
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
	// A message the matcher auto-linked before the application row existed: job_id set,
	// application_id still NULL. Reachable because the matcher is offered saved-only jobs
	// and nothing repairs the mail afterwards, so testing one column would admit it — and
	// confirming what this path proposed would then RE-LINK it.
	linkedWithoutApp := seedRecallEmail(t, q, user, "recall-halflinked", applied.Add(5*time.Hour),
		`UPDATE emails SET job_id = (SELECT job_id FROM applications WHERE id = $2) WHERE id = $1`, appID)
	atBoundary := seedRecallEmail(t, q, user, "recall-boundary", since, "")
	// Past the window's far edge. The edge exists so the cap trims the tail rather than
	// the head — without it, a busy mailbox's recent mail crowds out the acknowledgement.
	tooNew := seedRecallEmail(t, q, user, "recall-too-new", applied.AddDate(0, 0, 91), "")

	got := setOf(recallIDs(t, q, user, since, 40))

	for id, what := range map[int64]string{
		unlinked:   "unlinked mail",
		suggested:  "mail carrying an unconfirmed suggestion",
		atBoundary: "mail arriving exactly at the window's opening instant",
	} {
		if !got[id] {
			t.Errorf("%s was not in the net, and it is exactly what the net is for", what)
		}
	}
	for id, what := range map[int64]string{
		linked:           "mail already attached to an application",
		linkedWithoutApp: "mail linked to a job but carrying no application row",
		deleted:          "soft-deleted mail",
		tooOld:           "mail older than the window",
		tooNew:           "mail past the window's far edge",
	} {
		if got[id] {
			t.Errorf("%s reached the net", what)
		}
	}
}

func setOf(ids []int64) map[int64]bool {
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// The columns the model reads have to survive the query, and two of them are exactly the
// ones a plain-text seed would never notice were missing: body_html carries the whole
// HTML-only case the net's design rests on, and ical_uid is the calendar half of the
// feature.
func TestListEmailsForRecall_CarriesTheHTMLBodyAndTheInvitationIdentifier(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	user := seedResponseUser(t, q, "recall-html@example.test", true)
	// No text/plain part at all — how Gem, Ashby and Greenhouse send.
	id := seedRecallEmail(t, q, user, "recall-html-1", at,
		`UPDATE emails SET body_text = '', body_html = '<p>Interview with Derq</p>', ical_uid = 'derq@ashbyhq.com' WHERE id = $1`)

	rows := recallRows(t, q, user, at.AddDate(0, 0, -7), 40)
	if len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("got %d rows, want the one seeded message", len(rows))
	}
	if rows[0].BodyText != "" || rows[0].BodyHtml != "<p>Interview with Derq</p>" {
		t.Errorf("body came back text=%q html=%q — an HTML-only message must reach the caller with its content",
			rows[0].BodyText, rows[0].BodyHtml)
	}
	if rows[0].IcalUid != "derq@ashbyhq.com" {
		t.Errorf("ical_uid came back %q — without it the calendar half of the feature is blind", rows[0].IcalUid)
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

	seeded := make([]int64, 3)
	for i := range seeded {
		seeded[i] = seedRecallEmail(t, q, mine, fmt.Sprintf("recall-mine-%d", i), at.Add(time.Duration(i)*time.Hour), "")
	}
	stranger := seedRecallEmail(t, q, theirs, "recall-theirs-1", at, "")

	if got := setOf(recallIDs(t, q, mine, since, 40)); got[stranger] {
		t.Error("another user's mail reached the net")
	}
	// The cap keeps the OLDEST, and that is the whole point of the sort direction: the
	// acknowledgement that proves an application arrives first, so a cap eating from the
	// other end would drop it and leave the button reporting nothing found.
	got := recallIDs(t, q, mine, since, 2)
	want := []int64{seeded[0], seeded[1]}
	if !slices.Equal(got, want) {
		t.Errorf("limit 2 returned %v, want the two oldest %v", got, want)
	}
}

// The predicate in the statement is the guard, not an optimisation: even if the net, the
// model and the service layer all went wrong at once, a linked message is unreachable.
func TestSuggestJobForEmail_CannotTouchLinkedMail(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "recall-guard@example.test", true)
	ownerJob, ownerApp := seedApplication(t, q, user, "recall-guard-owner", "derq")
	otherJob, _ := seedApplication(t, q, user, "recall-guard-other", "ramp")
	at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	// Both shapes a linked message comes in. The second is the one a single-column guard
	// would miss: auto-linked before the application row existed, and never repaired.
	for name, seed := range map[string]string{
		"linked to an application": `UPDATE emails SET job_id = (SELECT job_id FROM applications WHERE id = $2), application_id = $2 WHERE id = $1`,
		"linked to a job only":     `UPDATE emails SET job_id = (SELECT job_id FROM applications WHERE id = $2) WHERE id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			linked := seedRecallEmail(t, q, user, "recall-guard-"+name, at, seed, ownerApp)

			rows, err := q.SuggestJobForEmail(ctx, SuggestJobForEmailParams{
				ID: linked, UserID: user, SuggestedJobID: otherJob, Confidence: 0.9,
			})
			if err != nil {
				t.Fatalf("suggest: %v", err)
			}
			if rows != 0 {
				t.Fatalf("the write changed %d rows on mail that is already %s, want 0", rows, name)
			}

			var jobID int64
			var suggested *int64
			var confidence *float32
			if err := q.db.QueryRow(ctx,
				`SELECT job_id, suggested_job_id, match_confidence FROM emails WHERE id = $1`, linked).
				Scan(&jobID, &suggested, &confidence); err != nil {
				t.Fatalf("read back: %v", err)
			}
			if jobID != ownerJob {
				t.Errorf("the link moved to job %d, want %d — this path may not relink", jobID, ownerJob)
			}
			if suggested != nil {
				t.Errorf("a suggestion was planted on linked mail (job %d)", *suggested)
			}
			// match_confidence belongs to the LINK. Restating it here would claim a
			// certainty about a link this statement did not make.
			if confidence != nil {
				t.Errorf("the link's confidence was overwritten with %v", *confidence)
			}
		})
	}
}

// A suggestion is a proposal, not a decision. The caller asked about THIS application
// explicitly, so an unconfirmed proposal naming another one gives way.
func TestSuggestJobForEmail_ReplacesAnUnconfirmedSuggestion(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "recall-replace@example.test", true)
	wantedJob, _ := seedApplication(t, q, user, "recall-replace-wanted", "derq")
	staleJob, _ := seedApplication(t, q, user, "recall-replace-stale", "ramp")
	at := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	email := seedRecallEmail(t, q, user, "recall-replace-1", at,
		`UPDATE emails SET suggested_job_id = $2 WHERE id = $1`, staleJob)

	rows, err := q.SuggestJobForEmail(ctx, SuggestJobForEmailParams{
		ID: email, UserID: user, SuggestedJobID: wantedJob, Confidence: 0.82,
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
func TestSuggestJobForEmail_IsScopedToTheCaller(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	mine := seedResponseUser(t, q, "recall-scope-mine@example.test", true)
	theirs := seedResponseUser(t, q, "recall-scope-theirs@example.test", true)
	job, _ := seedApplication(t, q, mine, "recall-scope-1", "derq")
	stranger := seedRecallEmail(t, q, theirs, "recall-scope-stranger", time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC), "")

	rows, err := q.SuggestJobForEmail(context.Background(), SuggestJobForEmailParams{
		ID: stranger, UserID: mine, SuggestedJobID: job, Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if rows != 0 {
		t.Fatalf("the write reached another user's mail (%d rows)", rows)
	}
}
