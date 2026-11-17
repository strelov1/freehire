//go:build integration

// Free saved-search alerts are daily whatever the stored preference says; a live Pro or
// Ultra entitlement (any source: pro_until/ultra_until) lifts that. The rule is applied
// where the schedule is READ (the candidate list, the delivery row, the claim), so a stored
// instant/interval preference, or an entitlement that expired, never buys speed.
// Run with: go test -tags=integration ./internal/engage/notify/
package notify

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// clearPlanSQL drops every source of entitlement (pro_until/ultra_until are generated from them).
const clearPlanSQL = `UPDATE users SET pro_until_stripe = NULL, pro_until_revenuecat = NULL, pro_until_granted = NULL,
	ultra_until_stripe = NULL, ultra_until_revenuecat = NULL, ultra_until_granted = NULL WHERE id = $1`

func TestFreeAlertsAreDailyAndPaidKeepTheirFasterPreference(t *testing.T) {
	// 06:00 UTC: before the 09:00 default digest time, outside any quiet hours.
	before9 := time.Date(2026, 8, 14, 6, 0, 0, 0, time.UTC)
	after9 := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name          string
		entitlement   string // SQL assignments for users
		frequency     string
		lastSentAgo   string // SQL interval; "" never sent
		now           time.Time
		wantDelivered bool
	}{
		{"free instant waits for the daily time", "", "instant", "", before9, false},
		{"free interval waits for the daily time", "", "interval", "", before9, false},
		{"free instant is delivered at the daily time", "", "instant", "", after9, true},
		{"free instant gets one digest a day", "", "instant", "1 hour", after9, false},
		{"expired pro reverts to daily", "pro_until_stripe = now() - interval '1 minute'", "instant", "", before9, false},
		{"expired ultra reverts to daily", "ultra_until_granted = now() - interval '1 minute'", "interval", "", before9, false},
		{"stripe pro keeps instant", "pro_until_stripe = now() + interval '1 day'", "instant", "", before9, true},
		{"store (revenuecat) pro keeps instant", "pro_until_revenuecat = now() + interval '1 day'", "instant", "", before9, true},
		{"granted pro keeps instant", "pro_until_granted = now() + interval '1 day'", "instant", "", before9, true},
		{"ultra keeps interval", "ultra_until_stripe = now() + interval '1 day'", "interval", "", before9, true},
		{"paid instant is not held to one digest a day", "pro_until_granted = now() + interval '1 day'", "instant", "1 hour", after9, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := testdb.Pool(t)
			queries := db.New(pool)
			ctx := context.Background()
			user := insertNotifyIntegrationUser(t, pool, "free-daily@example.test")
			insertNotifyPushToken(t, pool, user, "ExponentPushToken[fd]")
			if _, err := pool.Exec(ctx, `UPDATE notification_settings SET digest_frequency = $2, digest_time = '09:00' WHERE user_id = $1`, user, tc.frequency); err != nil {
				t.Fatalf("settings: %v", err)
			}
			// The helper may grant a plan by default; the case decides the entitlement.
			if _, err := pool.Exec(ctx, clearPlanSQL, user); err != nil {
				t.Fatalf("reset plan: %v", err)
			}
			if tc.entitlement != "" {
				if _, err := pool.Exec(ctx, `UPDATE users SET `+tc.entitlement+` WHERE id = $1`, user); err != nil {
					t.Fatalf("entitlement: %v", err)
				}
			}
			search := insertNotifySavedSearch(t, pool, user, "Go", "q=go")
			sub := insertNotifyPushSubscription(t, pool, user, search)
			if tc.lastSentAgo != "" {
				if _, err := pool.Exec(ctx, `UPDATE subscriptions SET last_digest_sent_at = $2::timestamptz - $3::interval WHERE id = $1`, sub, tc.now, tc.lastSentAgo); err != nil {
					t.Fatalf("stamp: %v", err)
				}
			}
			job := insertNotifyJob(t, pool, "fd-ext", "Engineer", "fd-slug")
			insertNotifyPendingMatch(t, pool, sub, job)

			transport := &fakePushIntegrationTransport{}
			runner := New(queries, &fakeSearcher{}, Router{ChannelPush: NewPushNotifier(queries, transport)}, DefaultConfig())
			runner.now = func() time.Time { return tc.now }
			if _, err := runner.Run(ctx); err != nil {
				t.Fatalf("Run: %v", err)
			}
			notified, _ := matchState(t, pool, sub, job)
			if notified != tc.wantDelivered || (len(transport.sent) == 1) != tc.wantDelivered {
				t.Fatalf("notified=%v sent=%v, want delivered=%v", notified, transport.sent, tc.wantDelivered)
			}
		})
	}
}

// The search side reads the same effective schedule: a Free account's query is not
// searched on the faster cadence it no longer gets, and the webhook channel (a program
// reading one saved search per payload, not a person) keeps the stored one.
func TestEffectiveFrequencyOnEveryRead(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	free := insertNotifyIntegrationUser(t, pool, "eff-free@example.test")
	paid := insertNotifyIntegrationUser(t, pool, "eff-paid@example.test")
	for _, u := range []int64{free, paid} {
		if _, err := pool.Exec(ctx, `UPDATE notification_settings SET digest_frequency = 'instant', digest_time = '07:30' WHERE user_id = $1`, u); err != nil {
			t.Fatalf("settings: %v", err)
		}
	}
	for _, u := range []int64{free, paid} {
		if _, err := pool.Exec(ctx, clearPlanSQL, u); err != nil {
			t.Fatalf("clear plan: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET ultra_until_granted = now() + interval '1 day' WHERE id = $1`, paid); err != nil {
		t.Fatalf("paid plan: %v", err)
	}
	want := map[int64]string{}
	for _, c := range []struct {
		user    int64
		channel string
		freq    string
	}{{free, ChannelPush, FrequencyDaily}, {free, ChannelWebhook, FrequencyInstant}, {paid, ChannelPush, FrequencyInstant}} {
		search := insertNotifySavedSearch(t, pool, c.user, "Go "+c.channel, "q=go")
		sub := insertNotifySubscription(t, pool, c.user, search, c.channel)
		insertNotifyPendingMatch(t, pool, sub, insertNotifyJob(t, pool, "eff-"+c.channel+"-"+c.freq, "Engineer", "eff-"+c.channel+"-"+c.freq))
		want[sub] = c.freq
	}

	active, err := queries.ListActiveSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSubscriptions: %v", err)
	}
	for _, r := range active {
		if w, ok := want[r.ID]; ok && r.DigestFrequency != w {
			t.Errorf("ListActiveSubscriptions %d = %q, want %q", r.ID, r.DigestFrequency, w)
		}
	}
	candidates, err := queries.ListDeliveryCandidates(ctx)
	if err != nil {
		t.Fatalf("ListDeliveryCandidates: %v", err)
	}
	for _, r := range candidates {
		if r.DigestFrequency != want[r.ID] {
			t.Errorf("ListDeliveryCandidates %d = %q, want %q", r.ID, r.DigestFrequency, want[r.ID])
		}
		one, err := queries.GetSubscriptionForDelivery(ctx, r.ID)
		if err != nil || one.DigestFrequency != want[r.ID] {
			t.Errorf("GetSubscriptionForDelivery %d = %q (%v), want %q", r.ID, one.DigestFrequency, err, want[r.ID])
		}
		if r.DigestTime.Microseconds != 7*3600e6+30*60e6 {
			t.Errorf("subscription %d lost the stored local time: %+v", r.ID, r.DigestTime)
		}
	}
}
