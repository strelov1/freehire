//go:build integration

// Integration test for the push delivery channel (task 3.5 of
// add-push-notification-channel): a push subscription backed by a real
// Postgres store delivers when the user has a registered device, and softly
// skips (matches stay pending, no attempt counted) when they don't. The fast
// unit tests in notify_test.go and push_test.go already cover recipient()'s
// branches and PushNotifier's render logic against fakes; this test's only job
// is proving the real Store + real schema round-trip through Runner.Run the
// same way. Run with:
// go test -tags=integration ./internal/notify/
// Requires Docker (testcontainers spins up a throwaway Postgres with the
// migrations).
package notify

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/testdb"
)

// fakePushIntegrationTransport is a pushnotify.Notifier stub that always
// succeeds, so the test exercises PushNotifier + the real Store without
// depending on Expo's actual API.
type fakePushIntegrationTransport struct {
	sent []string // tokens sent to
}

func (f *fakePushIntegrationTransport) Send(_ context.Context, token, _, _ string, _ map[string]string) error {
	f.sent = append(f.sent, token)
	return nil
}

func insertNotifyIntegrationUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func insertNotifySavedSearch(t *testing.T, pool *pgxpool.Pool, userID int64, name, query string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO saved_searches (user_id, name, query) VALUES ($1, $2, $3) RETURNING id`,
		userID, name, query).Scan(&id); err != nil {
		t.Fatalf("insert saved search: %v", err)
	}
	return id
}

func insertNotifyPushSubscription(t *testing.T, pool *pgxpool.Pool, userID, savedSearchID int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO subscriptions (user_id, saved_search_id, channel, active) VALUES ($1, $2, 'push', true) RETURNING id`,
		userID, savedSearchID).Scan(&id); err != nil {
		t.Fatalf("insert subscription: %v", err)
	}
	return id
}

func insertNotifyJob(t *testing.T, pool *pgxpool.Pool, externalID, title, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug) VALUES ('test', $1, 'https://example.test/job', $2, $3) RETURNING id`,
		externalID, title, slug).Scan(&id); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return id
}

func insertNotifyPendingMatch(t *testing.T, pool *pgxpool.Pool, subscriptionID, jobID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO subscription_matches (subscription_id, job_id) VALUES ($1, $2)`,
		subscriptionID, jobID); err != nil {
		t.Fatalf("insert subscription match: %v", err)
	}
}

func insertNotifyPushToken(t *testing.T, pool *pgxpool.Pool, userID int64, token string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO user_push_tokens (user_id, token, platform) VALUES ($1, $2, 'ios')`,
		userID, token); err != nil {
		t.Fatalf("insert push token: %v", err)
	}
}

func matchState(t *testing.T, pool *pgxpool.Pool, subscriptionID, jobID int64) (notified, claimed bool) {
	t.Helper()
	var notifiedAt, claimedAt any
	if err := pool.QueryRow(context.Background(),
		`SELECT notified_at, claimed_at FROM subscription_matches WHERE subscription_id = $1 AND job_id = $2`,
		subscriptionID, jobID).Scan(&notifiedAt, &claimedAt); err != nil {
		t.Fatalf("load match state: %v", err)
	}
	return notifiedAt != nil, claimedAt != nil
}

func TestPushDelivery_UserWithRegisteredDeviceIsDelivered(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "push-delivered@example.test")
	savedSearchID := insertNotifySavedSearch(t, pool, userID, "Backend Engineer", "seniority=senior")
	subID := insertNotifyPushSubscription(t, pool, userID, savedSearchID)
	jobID := insertNotifyJob(t, pool, "job-delivered-1", "Backend Engineer", "acme-backend-engineer")
	insertNotifyPendingMatch(t, pool, subID, jobID)
	insertNotifyPushToken(t, pool, userID, "ExponentPushToken[has-device]")

	transport := &fakePushIntegrationTransport{}
	router := Router{ChannelPush: NewPushNotifier(queries, transport)}
	runner := New(queries, &fakeSearcher{}, router, DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Delivered != 1 {
		t.Errorf("stats.Delivered = %d, want 1", stats.Delivered)
	}
	if stats.SoftSkips != 0 {
		t.Errorf("stats.SoftSkips = %d, want 0", stats.SoftSkips)
	}
	if len(transport.sent) != 1 || transport.sent[0] != "ExponentPushToken[has-device]" {
		t.Errorf("transport.sent = %v, want the registered token", transport.sent)
	}

	notified, _ := matchState(t, pool, subID, jobID)
	if !notified {
		t.Error("match not marked notified after a successful delivery")
	}
}

func TestPushDelivery_UserWithNoDeviceSoftSkips(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "push-no-device@example.test")
	savedSearchID := insertNotifySavedSearch(t, pool, userID, "Backend Engineer", "seniority=senior")
	subID := insertNotifyPushSubscription(t, pool, userID, savedSearchID)
	jobID := insertNotifyJob(t, pool, "job-no-device-1", "Backend Engineer", "acme-backend-engineer-2")
	insertNotifyPendingMatch(t, pool, subID, jobID)
	// No user_push_tokens row: HasPushDevice is false, so the delivery must
	// soft-skip rather than dead-letter.

	transport := &fakePushIntegrationTransport{}
	router := Router{ChannelPush: NewPushNotifier(queries, transport)}
	runner := New(queries, &fakeSearcher{}, router, DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.SoftSkips != 1 {
		t.Errorf("stats.SoftSkips = %d, want 1", stats.SoftSkips)
	}
	if stats.Delivered != 0 {
		t.Errorf("stats.Delivered = %d, want 0", stats.Delivered)
	}
	if len(transport.sent) != 0 {
		t.Errorf("transport.sent = %v, want none — no device to send to", transport.sent)
	}

	notified, claimed := matchState(t, pool, subID, jobID)
	if notified {
		t.Error("match marked notified despite a soft-skip")
	}
	if claimed {
		t.Error("match still claimed after soft-skip; want the claim released so the next pass retries")
	}
}
