//go:build integration

// Integration test for task 2.4 of add-notification-center: a delivered
// subscription digest records a real user_notifications row against a real
// Postgres, and a recording failure does not prevent the delivery itself from
// succeeding. The fast unit tests in deliver_test.go already cover the
// render/branch logic against a fake Store; this test's only job is proving
// the real Store + real schema round-trip through Runner.Run. Run with:
// go test -tags=integration ./internal/engage/notify/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package notify

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestNotificationCenter_SingleJobDigestRecordsOneRowWithSlug(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "notif-center-single@example.test")
	savedSearchID := insertNotifySavedSearch(t, pool, userID, "Backend Engineer", "seniority=senior")
	subID := insertNotifyPushSubscription(t, pool, userID, savedSearchID)
	jobID := insertNotifyJob(t, pool, "notif-center-job-1", "Backend Engineer", "acme-backend-engineer")
	insertNotifyPendingMatch(t, pool, subID, jobID)
	insertNotifyPushToken(t, pool, userID, "ExponentPushToken[notif-center-1]")

	router := Router{ChannelPush: NewPushNotifier(queries, &fakePushIntegrationTransport{})}
	runner := New(queries, &fakeSearcher{}, router, DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Delivered != 1 {
		t.Fatalf("stats.Delivered = %d, want 1", stats.Delivered)
	}

	rows, err := queries.ListUserNotifications(ctx, db.ListUserNotificationsParams{UserID: userID, Lim: 10, Off: 0})
	if err != nil {
		t.Fatalf("ListUserNotifications: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("notifications = %d, want 1", len(rows))
	}
	if rows[0].Kind != "subscription_digest" {
		t.Errorf("kind = %q, want subscription_digest", rows[0].Kind)
	}
	if !rows[0].PublicSlug.Valid || rows[0].PublicSlug.String != "acme-backend-engineer" {
		t.Errorf("public_slug = %+v, want a valid slug of acme-backend-engineer", rows[0].PublicSlug)
	}
	if rows[0].ReadAt.Valid {
		t.Error("read_at set on a freshly recorded notification, want unread")
	}
}

func TestNotificationCenter_MultiJobDigestRecordsRowWithoutSlug(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "notif-center-multi@example.test")
	savedSearchID := insertNotifySavedSearch(t, pool, userID, "Backend Engineer", "seniority=senior")
	subID := insertNotifyPushSubscription(t, pool, userID, savedSearchID)
	job1 := insertNotifyJob(t, pool, "notif-center-job-2", "Backend Engineer", "acme-backend-engineer-2")
	job2 := insertNotifyJob(t, pool, "notif-center-job-3", "Backend Engineer II", "acme-backend-engineer-3")
	insertNotifyPendingMatch(t, pool, subID, job1)
	insertNotifyPendingMatch(t, pool, subID, job2)
	insertNotifyPushToken(t, pool, userID, "ExponentPushToken[notif-center-2]")

	router := Router{ChannelPush: NewPushNotifier(queries, &fakePushIntegrationTransport{})}
	runner := New(queries, &fakeSearcher{}, router, DefaultConfig())

	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	rows, err := queries.ListUserNotifications(ctx, db.ListUserNotificationsParams{UserID: userID, Lim: 10, Off: 0})
	if err != nil {
		t.Fatalf("ListUserNotifications: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("notifications = %d, want 1 (one digest = one row, even with 2 matched jobs)", len(rows))
	}
	if rows[0].PublicSlug.Valid {
		t.Errorf("public_slug = %+v, want invalid/absent for a multi-job digest", rows[0].PublicSlug)
	}
}

// One saved search subscribed on three channels is still ONE job-match event.
// The delivery loop runs once per `subscriptions` row and that table is keyed
// (saved_search_id, channel), so before the dedup key every enabled channel left
// its own indistinguishable `My profile — 1 new job` row in the history that
// migration 0090 designed to hold one row per event "independent of which
// channel(s) carried it" (freehire#3020).
func TestNotificationCenter_ThreeChannelsRecordOneRow(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "notif-center-channels@example.test")
	savedSearchID := insertNotifySavedSearch(t, pool, userID, "My profile", "seniority=senior")
	pushSub := insertNotifySubscription(t, pool, userID, savedSearchID, ChannelPush)
	emailSub := insertNotifySubscription(t, pool, userID, savedSearchID, ChannelEmail)
	telegramSub := insertNotifySubscription(t, pool, userID, savedSearchID, ChannelTelegram)
	jobID := insertNotifyJob(t, pool, "notif-center-job-4", "Backend Engineer", "acme-backend-engineer-4")
	insertNotifyPendingMatch(t, pool, pushSub, jobID)
	insertNotifyPendingMatch(t, pool, emailSub, jobID)
	insertNotifyPendingMatch(t, pool, telegramSub, jobID)
	insertNotifyPushToken(t, pool, userID, "ExponentPushToken[notif-center-4]")
	insertNotifyTelegramLink(t, pool, userID, 424242)

	channels := &fakeNotifier{}
	router := Router{
		ChannelPush:     NewPushNotifier(queries, &fakePushIntegrationTransport{}),
		ChannelEmail:    channels,
		ChannelTelegram: channels,
	}
	runner := New(queries, &fakeSearcher{}, router, DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// All three messages still go out — the person asked for three channels.
	if stats.Delivered != 3 {
		t.Fatalf("stats.Delivered = %d, want 3 (one message per enabled channel)", stats.Delivered)
	}

	rows, err := queries.ListUserNotifications(ctx, db.ListUserNotificationsParams{UserID: userID, Lim: 10, Off: 0})
	if err != nil {
		t.Fatalf("ListUserNotifications: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("notifications = %d, want 1 (one event, delivered over three channels)", len(rows))
	}
	if rows[0].Title != "My profile" {
		t.Errorf("title = %q, want the saved search name", rows[0].Title)
	}
	if !rows[0].PublicSlug.Valid || rows[0].PublicSlug.String != "acme-backend-engineer-4" {
		t.Errorf("public_slug = %+v, want the single matched job's slug", rows[0].PublicSlug)
	}
}
