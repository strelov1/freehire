//go:build integration

// Integration test for task 2.4 of add-notification-center: a delivered
// reminder records a real user_notifications row against a real Postgres,
// always carrying a slug (a reminder always concerns one job). Run with:
// go test -tags=integration ./internal/reminder/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package reminder

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/notify"
	"github.com/strelov1/freehire/internal/testdb"
)

func TestNotificationCenter_DeliveredReminderRecordsRowWithSlug(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertReminderIntegrationUser(t, pool, "notif-center-reminder@example.test")
	jobID := insertReminderIntegrationJob(t, pool, "notif-center-reminder-job")
	seedSavedJobReminder(t, pool, userID, jobID)
	insertPushToken(t, pool, userID, "ExponentPushToken[notif-center-reminder]")

	router := Router{notify.ChannelPush: NewPushNotifier(queries, &fakePushTransport{})}
	runner := NewRunner(queries, router, DefaultConfig())

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
	if rows[0].Kind != "reminder" {
		t.Errorf("kind = %q, want reminder", rows[0].Kind)
	}
	if !rows[0].PublicSlug.Valid {
		t.Error("public_slug not set, want it always populated for a reminder")
	}
}
