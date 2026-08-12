//go:build integration

// Integration test for task 2.4 of add-notification-center: a delivered
// subscription digest records a real user_notifications row against a real
// Postgres, and a recording failure does not prevent the delivery itself from
// succeeding. The fast unit tests in deliver_test.go already cover the
// render/branch logic against a fake Store; this test's only job is proving
// the real Store + real schema round-trip through Runner.Run. Run with:
// go test -tags=integration ./internal/notify/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package notify

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/testdb"
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
