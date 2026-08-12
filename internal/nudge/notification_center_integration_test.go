//go:build integration

// Integration test for task 2.4 of add-notification-center: a delivered nudge
// records a real user_notifications row against a real Postgres, mapped to
// kind=nudge_job_closed, always carrying a slug. Reuses the job_closed seeding
// helpers from push_integration_test.go (see its package doc for why
// job_closed is the kind exercised directly against Runner.deliver). Run with:
// go test -tags=integration ./internal/nudge/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package nudge

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/notify"
	"github.com/strelov1/freehire/internal/testdb"
)

func TestNotificationCenter_DeliveredNudgeRecordsRowWithSlug(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := seedNudgeIntegrationUser(t, pool, "notif-center-nudge@example.test")
	jobID := seedClosedJob(t, pool, "notif-center-nudge-job")
	seedApplication(t, pool, userID, jobID, "applied")
	seedNotificationSettings(t, pool, userID, []string{notify.ChannelPush})
	seedPendingJobClosedNudge(t, pool, userID, jobID)
	seedPushDevice(t, pool, userID, "ExponentPushToken[notif-center-nudge]")

	router := Router{notify.ChannelPush: NewPushNotifier(queries, &integrationPushTransport{})}
	runner := New(queries, router, DefaultConfig())

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
	if rows[0].Kind != "nudge_job_closed" {
		t.Errorf("kind = %q, want nudge_job_closed", rows[0].Kind)
	}
	if !rows[0].PublicSlug.Valid {
		t.Error("public_slug not set, want it always populated for a nudge")
	}
}
