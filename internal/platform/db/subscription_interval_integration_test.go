//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

// The interval frequency's elapsed-interval gate lives in ClaimSubscriptionMatches, so a
// subscription that is not due is never claimed (and released) at all. This pins who is
// claimable: an interval subscription only once its interval has passed since the last
// digest, everything else regardless of the stamp — daily is timed in Go, instant is not
// timed. It also pins the defaults: no settings row reads as daily at 09:00 (migration 0246),
// which the claim does not gate — the daily hour is decided in Go.
func TestClaimSubscriptionMatches_IntervalGate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateSubs(t, pool)
	job := insertJob(t, pool, "interval-job")

	// seed makes one subscription with one pending match, optional account settings, and
	// a last-digest stamp lastSent ago (SQL interval text; "" leaves it NULL).
	seed := func(email, settingsSQL, lastSent string) int64 {
		uid := insertUser(t, pool, email)
		if settingsSQL != "" {
			if _, err := pool.Exec(ctx, `INSERT INTO notification_settings (user_id, enabled, channels) VALUES ($1, true, '{email}')`, uid); err != nil {
				t.Fatalf("seed settings: %v", err)
			}
			if _, err := pool.Exec(ctx, `UPDATE notification_settings SET `+settingsSQL+` WHERE user_id = $1`, uid); err != nil {
				t.Fatalf("seed settings %q: %v", settingsSQL, err)
			}
		}
		ssID := insertSavedSearch(t, pool, uid, "s", "q=go")
		sub, err := q.CreateSubscription(ctx, CreateSubscriptionParams{Channel: "email", SavedSearchID: ssID, UserID: uid})
		if err != nil {
			t.Fatalf("seed subscription: %v", err)
		}
		if lastSent != "" {
			if _, err := pool.Exec(ctx, `UPDATE subscriptions SET last_digest_sent_at = now() - $2::interval WHERE id = $1`, sub.ID, lastSent); err != nil {
				t.Fatalf("seed last sent: %v", err)
			}
		}
		if _, err := recordMatch(ctx, q, sub.ID, job); err != nil {
			t.Fatalf("seed match: %v", err)
		}
		return sub.ID
	}

	notDue := []int64{
		seed("hourly-recent@example.test", "digest_frequency = 'interval'", "30 minutes"),
		seed("three-hours@example.test", "digest_frequency = 'interval', digest_interval_hours = 3", "2 hours"),
	}
	due := []int64{
		seed("hourly-elapsed@example.test", "digest_frequency = 'interval'", "61 minutes"),
		seed("no-row-never@example.test", "", ""),
		seed("no-row-recent@example.test", "", "30 minutes"),
		seed("three-hours-elapsed@example.test", "digest_frequency = 'interval', digest_interval_hours = 3", "3 hours 1 minute"),
		seed("instant@example.test", "digest_frequency = 'instant'", "1 minute"),
		seed("daily@example.test", "digest_frequency = 'daily', digest_time = '09:00'", "1 minute"),
	}

	claimed, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, PerSubscription: 100, BatchSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]bool{}
	for _, c := range claimed {
		got[c.SubscriptionID] = true
	}
	for _, id := range due {
		if !got[id] {
			t.Errorf("subscription %d not claimed, want claimed (due)", id)
		}
	}
	for _, id := range notDue {
		if got[id] {
			t.Errorf("subscription %d claimed, want skipped (interval not elapsed)", id)
		}
	}

	t.Run("a new settings row defaults to daily at 09:00", func(t *testing.T) {
		uid := insertUser(t, pool, "fresh-row@example.test")
		var freq, at string
		var hours int16
		if err := pool.QueryRow(ctx,
			`INSERT INTO notification_settings (user_id) VALUES ($1) RETURNING digest_frequency, digest_time::text, digest_interval_hours`,
			uid).Scan(&freq, &at, &hours); err != nil {
			t.Fatal(err)
		}
		if freq != "daily" || at != "09:00:00" || hours != 1 {
			t.Errorf("defaults = %q at %s / %d, want daily at 09:00:00 / 1", freq, at, hours)
		}
		if _, err := pool.Exec(ctx, `UPDATE notification_settings SET digest_interval_hours = 24 WHERE user_id = $1`, uid); err == nil {
			t.Error("digest_interval_hours = 24 accepted, want the CHECK to refuse it (a day is 'daily')")
		}
	})
}

// The interval is the person's on a channel, not each subscription's: the notifier
// sends one message per (user, channel) and stamps every subscription it carried, so
// a person's newest stamp on the channel decides. A brand-new subscription (no stamp)
// therefore waits for the person's next hourly mail instead of firing on its own, a
// second channel keeps its own clock, and a webhook keeps one per subscription.
func TestClaimSubscriptionMatches_IntervalGateIsPerPersonAndChannel(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateSubs(t, pool)
	job := insertJob(t, pool, "per-person-job")

	sub := func(uid int64, channel, lastSent string) int64 {
		ssID := insertSavedSearch(t, pool, uid, channel+lastSent, "q=go")
		s, err := q.CreateSubscription(ctx, CreateSubscriptionParams{Channel: channel, SavedSearchID: ssID, UserID: uid})
		if err != nil {
			t.Fatalf("seed subscription: %v", err)
		}
		if lastSent != "" {
			if _, err := pool.Exec(ctx, `UPDATE subscriptions SET last_digest_sent_at = now() - $2::interval WHERE id = $1`, s.ID, lastSent); err != nil {
				t.Fatalf("seed last sent: %v", err)
			}
		}
		if _, err := recordMatch(ctx, q, s.ID, job); err != nil {
			t.Fatalf("seed match: %v", err)
		}
		return s.ID
	}

	recent := insertUser(t, pool, "recent@example.test")
	elapsed := insertUser(t, pool, "elapsed@example.test")
	// Both on the hourly interval: the no-row default is daily (migration 0246), which
	// the claim does not gate.
	for _, uid := range []int64{recent, elapsed} {
		if _, err := pool.Exec(ctx, `INSERT INTO notification_settings (user_id, digest_frequency) VALUES ($1, 'interval')`, uid); err != nil {
			t.Fatalf("seed settings: %v", err)
		}
	}
	notDue := []int64{
		sub(recent, "email", "10 minutes"),
		sub(recent, "email", ""),            // new: waits for the person's next mail
		sub(recent, "email", "5 hours"),     // stale own stamp, but the person was mailed 10 minutes ago
		sub(recent, "webhook", "5 minutes"), // a webhook keeps its own clock
	}
	due := []int64{
		sub(recent, "telegram", ""),         // another channel: its own clock
		sub(recent, "webhook", "2 hours"),   // its own stamp has elapsed
		sub(elapsed, "email", "61 minutes"), // the person's newest stamp has elapsed
		sub(elapsed, "email", ""),           // and joins that mail
	}

	claimed, err := q.ClaimSubscriptionMatches(ctx, ClaimSubscriptionMatchesParams{LeaseSeconds: 3600, PerSubscription: 100, BatchSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]ClaimSubscriptionMatchesRow{}
	for _, c := range claimed {
		got[c.SubscriptionID] = c
	}
	for _, id := range due {
		if _, ok := got[id]; !ok {
			t.Errorf("subscription %d not claimed, want claimed", id)
		}
	}
	for _, id := range notDue {
		if _, ok := got[id]; ok {
			t.Errorf("subscription %d claimed, want held for the person's interval", id)
		}
	}
	if c := got[due[2]]; c.UserID != elapsed || c.Channel != "email" {
		t.Errorf("claim row = user %d channel %q, want user %d email (the notifier groups on them)", c.UserID, c.Channel, elapsed)
	}
	// The never-sent subscription carries the PERSON's stamp, which is what times a
	// `daily` message in Go.
	if c := got[due[3]]; !c.PersonLastSentAt.Valid || time.Since(c.PersonLastSentAt.Time) < time.Hour {
		t.Errorf("new subscription's person stamp = %+v, want the sibling's ~61 minutes ago", c.PersonLastSentAt)
	}
}
