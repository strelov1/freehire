//go:build integration

// Integration tests for the inbox's default: mail the classifier judged not to be about an
// application at all is omitted, and the listing says how much it omitted.
//
// The count is the point. A filter that hides silently makes a misclassification impossible
// to find, and the classifier reads attacker-controlled text.
//
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"
)

func seedLabelled(t *testing.T, q *Queries, userID int64, extID, signal string, classified bool) int64 {
	t.Helper()
	var id int64
	var sig any
	if signal != "" {
		sig = signal
	}
	var at any
	if classified {
		at = time.Now()
	}
	if err := q.db.QueryRow(context.Background(),
		`INSERT INTO emails (user_id, source, external_id, subject, body_text, received_at, status_signal, classified_at)
		 VALUES ($1,'gmail',$2,'Subject','body', now(), $3, $4) RETURNING id`,
		userID, extID, sig, at).Scan(&id); err != nil {
		t.Fatalf("seed %s: %v", extID, err)
	}
	return id
}

func listed(t *testing.T, q *Queries, userID int64, includeOther bool) map[int64]bool {
	t.Helper()
	rows, err := q.ListEmails(context.Background(), ListEmailsParams{
		UserID: userID, Lim: 50, IncludeOther: includeOther,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	out := map[int64]bool{}
	for _, r := range rows {
		out[r.ID] = true
	}
	return out
}

// The default hides what the classifier called `other`, keeps everything it judged
// otherwise, and NEVER hides a message nothing has judged — an unclassified message has not
// been found irrelevant, it has not been looked at.
func TestListEmailsHidesOtherByDefault(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	user := seedResponseUser(t, q, "hide-other@example.test", true)

	noise := seedLabelled(t, q, user, "hide-noise", "other", true)
	real := seedLabelled(t, q, user, "hide-real", "acknowledgement", true)
	unjudged := seedLabelled(t, q, user, "hide-unjudged", "", false)

	got := listed(t, q, user, false)
	if got[noise] {
		t.Error("mail labelled `other` was listed by default")
	}
	for id, what := range map[int64]string{real: "classified mail", unjudged: "unclassified mail"} {
		if !got[id] {
			t.Errorf("%s was hidden", what)
		}
	}

	all := listed(t, q, user, true)
	for id, what := range map[int64]string{noise: "`other` mail", real: "classified mail", unjudged: "unclassified mail"} {
		if !all[id] {
			t.Errorf("%s was missing when the caller asked for everything", what)
		}
	}
}

// The number is what makes a misclassification findable, so it is counted under the SAME
// filters as the listing — a hidden count that ignored the active filters would describe a
// different mailbox from the one on screen.
func TestCountEmailsReportsWhatItHid(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	user := seedResponseUser(t, q, "hide-count@example.test", true)

	seedLabelled(t, q, user, "count-noise-1", "other", true)
	seedLabelled(t, q, user, "count-noise-2", "other", true)
	seedLabelled(t, q, user, "count-real", "rejection", true)

	got, err := q.CountEmails(context.Background(), CountEmailsParams{UserID: user})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got.Total != 1 {
		t.Errorf("total %d, want the 1 message the default lists", got.Total)
	}
	if got.Hidden != 2 {
		t.Errorf("hidden %d, want 2 — the number is what makes a misclassification findable", got.Hidden)
	}

	all, err := q.CountEmails(context.Background(), CountEmailsParams{UserID: user, IncludeOther: true})
	if err != nil {
		t.Fatalf("count all: %v", err)
	}
	if all.Total != 3 {
		t.Errorf("total %d when asked for everything, want 3", all.Total)
	}
	if all.Hidden != 0 {
		t.Errorf("hidden %d when nothing is hidden, want 0", all.Hidden)
	}
}

// The hidden count answers "under these filters", not "in this mailbox".
func TestHiddenCountRespectsTheActiveFilters(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	user := seedResponseUser(t, q, "hide-filtered@example.test", true)

	seedLabelled(t, q, user, "filt-noise-gmail", "other", true)
	hosted := seedLabelled(t, q, user, "filt-noise-hosted", "other", true)
	if _, err := q.db.Exec(context.Background(),
		`UPDATE emails SET source = 'hosted' WHERE id = $1`, hosted); err != nil {
		t.Fatalf("move source: %v", err)
	}

	got, err := q.CountEmails(context.Background(), CountEmailsParams{UserID: user, Src: "gmail"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got.Hidden != 1 {
		t.Errorf("hidden %d under a source filter, want 1 — the count must describe the "+
			"mailbox on screen, not the whole one", got.Hidden)
	}
}
