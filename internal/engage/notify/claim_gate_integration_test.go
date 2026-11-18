//go:build integration

// Integration tests for the pre-claim delivery gate: a subscription that cannot be written
// to now is never leased, which only a real Postgres can show (the row's xmin does not move
// when no UPDATE touched it). Run with:
// go test -tags=integration ./internal/engage/notify/
package notify

import (
	"context"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// candidateIDs is every subscription ListDeliveryCandidates names, for a test that wants the
// claim without the worker's gate in front of it.
func candidateIDs(t *testing.T, queries *db.Queries) []int64 {
	t.Helper()
	rows, err := queries.ListDeliveryCandidates(context.Background())
	if err != nil {
		t.Fatalf("ListDeliveryCandidates: %v", err)
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// matchXmin is the id of the transaction that last wrote the match row: it moves on ANY
// UPDATE, including a claim followed by a release that restores claimed_at to NULL.
func matchXmin(t *testing.T, pool *pgxpool.Pool, subscriptionID, jobID int64) string {
	t.Helper()
	var x string
	if err := pool.QueryRow(context.Background(),
		`SELECT xmin::text FROM subscription_matches WHERE subscription_id = $1 AND job_id = $2`,
		subscriptionID, jobID).Scan(&x); err != nil {
		t.Fatalf("read xmin: %v", err)
	}
	return x
}

// Three subscriptions with one pending match each: one in its user's quiet hours, one whose
// user has no push device, one deliverable. After a pass only the deliverable one has been
// written to; the other two rows are byte-for-byte untouched and stay pending.
func TestRunNeverLeasesAHeldSubscription(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	quietUser := insertNotifyIntegrationUser(t, pool, "gate-quiet@example.test")
	noDeviceUser := insertNotifyIntegrationUser(t, pool, "gate-nodevice@example.test")
	dueUser := insertNotifyIntegrationUser(t, pool, "gate-due@example.test")

	// instant, with quiet hours covering every minute of the day but one.
	if _, err := pool.Exec(ctx, `INSERT INTO notification_settings (user_id, digest_frequency, quiet_hours_start, quiet_hours_end)
		VALUES ($1, 'instant', '00:00', '23:59')
		ON CONFLICT (user_id) DO UPDATE SET digest_frequency = 'instant', quiet_hours_start = '00:00', quiet_hours_end = '23:59'`, quietUser); err != nil {
		t.Fatalf("settings: %v", err)
	}
	for _, u := range []int64{noDeviceUser, dueUser} {
		if _, err := pool.Exec(ctx, `INSERT INTO notification_settings (user_id, digest_frequency) VALUES ($1, 'instant')
			ON CONFLICT (user_id) DO UPDATE SET digest_frequency = 'instant'`, u); err != nil {
			t.Fatalf("settings: %v", err)
		}
	}
	insertNotifyPushToken(t, pool, quietUser, "ExponentPushToken[quiet]")
	insertNotifyPushToken(t, pool, dueUser, "ExponentPushToken[due]")

	type pending struct{ sub, job int64 }
	var quiet, noDevice, due pending
	for i, p := range []*pending{&quiet, &noDevice, &due} {
		user := []int64{quietUser, noDeviceUser, dueUser}[i]
		search := insertNotifySavedSearch(t, pool, user, "Go", "q=go")
		p.sub = insertNotifyPushSubscription(t, pool, user, search)
		p.job = insertNotifyJob(t, pool, sequentialExternalID("gate", i), "Engineer", sequentialSlug("gate", i))
		insertNotifyPendingMatch(t, pool, p.sub, p.job)
	}
	before := map[pending]string{quiet: matchXmin(t, pool, quiet.sub, quiet.job), noDevice: matchXmin(t, pool, noDevice.sub, noDevice.job)}

	transport := &fakePushIntegrationTransport{}
	runner := New(queries, &fakeSearcher{}, Router{ChannelPush: NewPushNotifier(queries, transport)}, DefaultConfig())
	// 12:00 UTC is inside 00:00-23:59.
	runner.now = func() time.Time { return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) }

	stats, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Delivered != 1 || stats.Deferred != 1 || stats.SoftSkips != 1 {
		t.Errorf("delivered=%d deferred=%d softSkips=%d, want 1/1/1", stats.Delivered, stats.Deferred, stats.SoftSkips)
	}
	for name, p := range map[string]pending{"quiet hours": quiet, "no device": noDevice} {
		if got := matchXmin(t, pool, p.sub, p.job); got != before[p] {
			t.Errorf("%s: match row was written (xmin %s -> %s); a held subscription must not be leased", name, before[p], got)
		}
		if notified, claimed := matchState(t, pool, p.sub, p.job); notified || claimed {
			t.Errorf("%s: notified=%v claimed=%v, want pending and unleased", name, notified, claimed)
		}
	}
	if notified, _ := matchState(t, pool, due.sub, due.job); !notified {
		t.Error("the deliverable subscription was not delivered")
	}
	if len(transport.sent) != 1 || transport.sent[0] != "ExponentPushToken[due]" {
		t.Errorf("sent = %v, want only the due user's device", transport.sent)
	}
}

// The gate decides from the candidate row and deliverMessage rechecks from
// GetSubscriptionForDelivery: they must describe a subscription identically, the person's last
// send included.
func TestCandidateRowMatchesTheDeliveryRow(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	user := insertNotifyIntegrationUser(t, pool, "gate-parity@example.test")
	insertNotifyPushToken(t, pool, user, "ExponentPushToken[parity]")
	var subs []int64
	for i := 0; i < 2; i++ {
		search := insertNotifySavedSearch(t, pool, user, "Go "+strconv.Itoa(i), "q=go")
		sub := insertNotifyPushSubscription(t, pool, user, search)
		insertNotifyPendingMatch(t, pool, sub, insertNotifyJob(t, pool, sequentialExternalID("parity", i), "Engineer", sequentialSlug("parity", i)))
		subs = append(subs, sub)
	}
	// Only the first alert was mailed: the person's last send, read from either row, is its stamp.
	if _, err := pool.Exec(ctx, `UPDATE subscriptions SET last_digest_sent_at = now() - interval '1 hour' WHERE id = $1`, subs[0]); err != nil {
		t.Fatalf("stamp: %v", err)
	}

	candidates, err := queries.ListDeliveryCandidates(ctx)
	if err != nil || len(candidates) != 2 {
		t.Fatalf("candidates = %d, err %v, want 2", len(candidates), err)
	}
	for _, c := range candidates {
		got, err := queries.GetSubscriptionForDelivery(ctx, c.ID)
		if err != nil {
			t.Fatalf("GetSubscriptionForDelivery: %v", err)
		}
		if !reflect.DeepEqual(db.GetSubscriptionForDeliveryRow(c), got) {
			t.Errorf("subscription %d: candidate row %+v != delivery row %+v", c.ID, c, got)
		}
		if !c.PersonLastSentAt.Valid {
			t.Errorf("subscription %d: PersonLastSentAt not set, want the sibling alert's stamp", c.ID)
		}
	}
}
