//go:build integration

// Integration test for the push channel's end-to-end path against a real
// Postgres: a due, pending reminder whose rule includes "push" is delivered when
// the user has a registered device, and soft-skips (stays pending) when they
// don't — the same live has_push_device check TestRecipient_Push exercises with
// a fake, proven here against the real GetReminderForDelivery query and a real
// Runner. Run with:
// go test -tags=integration ./internal/reminder/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package reminder

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/notify"
	"github.com/strelov1/freehire/internal/testdb"
)

func insertReminderIntegrationUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func insertReminderIntegrationJob(t *testing.T, pool *pgxpool.Pool, externalID string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('test', $1, 'http://example.test', 'Go Developer', 'Acme', 'job-' || $1)
		 RETURNING id`, externalID).Scan(&id); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return id
}

// seedSavedJobReminder marks the job saved-but-unapplied for the user (so
// GetReminderForDelivery's still_actionable check passes) and inserts a
// job_reminders row that is already due, on the push channel.
func seedSavedJobReminder(t *testing.T, pool *pgxpool.Pool, userID, jobID int64) int64 {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_jobs (user_id, job_id, saved_at) VALUES ($1, $2, now())`,
		userID, jobID); err != nil {
		t.Fatalf("save job: %v", err)
	}
	var id int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO job_reminders (user_id, job_id, fire_at, channels)
		 VALUES ($1, $2, now() - interval '1 hour', $3)
		 RETURNING id`,
		userID, jobID, []string{notify.ChannelPush}).Scan(&id); err != nil {
		t.Fatalf("insert reminder: %v", err)
	}
	return id
}

func insertPushToken(t *testing.T, pool *pgxpool.Pool, userID int64, token string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO user_push_tokens (user_id, token, platform) VALUES ($1, $2, 'ios')`,
		userID, token); err != nil {
		t.Fatalf("insert push token: %v", err)
	}
}

func reminderStatus(t *testing.T, pool *pgxpool.Pool, id int64) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM job_reminders WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("load reminder status: %v", err)
	}
	return status
}

func TestRun_DeliversPushReminderToARegisteredDevice(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertReminderIntegrationUser(t, pool, "push-reminder-has-device@example.test")
	jobID := insertReminderIntegrationJob(t, pool, "push-has-device")
	reminderID := seedSavedJobReminder(t, pool, userID, jobID)
	insertPushToken(t, pool, userID, "ExponentPushToken[has-device]")

	transport := &fakePushTransport{}
	router := Router{notify.ChannelPush: NewPushNotifier(queries, transport)}
	runner := NewRunner(queries, router, DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Delivered != 1 {
		t.Errorf("stats.Delivered = %d, want 1", stats.Delivered)
	}
	if len(transport.calls) != 1 || transport.calls[0].token != "ExponentPushToken[has-device]" {
		t.Errorf("transport calls = %v, want one call to the registered device", transport.calls)
	}
	if got := reminderStatus(t, pool, reminderID); got != "delivered" {
		t.Errorf("reminder status = %q, want delivered", got)
	}
}

func TestRun_SoftSkipsPushReminderWithNoRegisteredDevice(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertReminderIntegrationUser(t, pool, "push-reminder-no-device@example.test")
	jobID := insertReminderIntegrationJob(t, pool, "push-no-device")
	reminderID := seedSavedJobReminder(t, pool, userID, jobID)
	// No user_push_tokens row for this user.

	transport := &fakePushTransport{}
	router := Router{notify.ChannelPush: NewPushNotifier(queries, transport)}
	runner := NewRunner(queries, router, DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.SoftSkips != 1 {
		t.Errorf("stats.SoftSkips = %d, want 1", stats.SoftSkips)
	}
	if len(transport.calls) != 0 {
		t.Errorf("transport calls = %v, want none — no registered device", transport.calls)
	}
	if got := reminderStatus(t, pool, reminderID); got != "pending" {
		t.Errorf("reminder status = %q, want pending (soft-skip, not dead-lettered)", got)
	}
}
