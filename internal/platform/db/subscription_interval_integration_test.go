//go:build integration

package db

import (
	"context"
	"testing"
)

// The interval frequency's elapsed-interval gate lives in ClaimSubscriptionMatches, so a
// subscription that is not due is never claimed (and released) at all. This pins who is
// claimable: an interval subscription only once its interval has passed since the last
// digest, everything else regardless of the stamp — daily is timed in Go, instant is not
// timed. It also pins the defaults: no settings row reads as an hourly interval.
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
		seed("default-recent@example.test", "", "30 minutes"),
		seed("three-hours@example.test", "digest_frequency = 'interval', digest_interval_hours = 3", "2 hours"),
	}
	due := []int64{
		seed("default-elapsed@example.test", "", "61 minutes"),
		seed("default-never@example.test", "", ""),
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

	t.Run("a new settings row defaults to an hourly interval", func(t *testing.T) {
		uid := insertUser(t, pool, "fresh-row@example.test")
		var freq string
		var hours int16
		if err := pool.QueryRow(ctx,
			`INSERT INTO notification_settings (user_id) VALUES ($1) RETURNING digest_frequency, digest_interval_hours`,
			uid).Scan(&freq, &hours); err != nil {
			t.Fatal(err)
		}
		if freq != "interval" || hours != 1 {
			t.Errorf("defaults = %q/%d, want interval/1", freq, hours)
		}
		if _, err := pool.Exec(ctx, `UPDATE notification_settings SET digest_interval_hours = 24 WHERE user_id = $1`, uid); err == nil {
			t.Error("digest_interval_hours = 24 accepted, want the CHECK to refuse it (a day is 'daily')")
		}
	})
}
