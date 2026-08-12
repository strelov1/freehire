//go:build integration

// Integration test for task 5.5 of add-push-notification-channel: a due
// job-closed nudge whose account rule includes `push` is delivered to a real
// registered device (via internal/testdb's throwaway, fully-migrated Postgres),
// and soft-skips cleanly when the account has none.
//
// job_closed is deliberately the kind exercised here: its actionable() re-check
// (internal/nudge/nudge.go) only requires the application to still sit in a
// silence-accruing stage — unlike follow_up/interview_prep it does not depend on
// re-deriving elapsed silence or a live stage_set event. The job's closed_at is
// seeded outside JobClosedWindowDays (the default 7-day MATCH window) so
// Runner.match's own ListJobClosedCandidates scan never picks up this
// application — avoiding its auto-expire side effect (match() drives the stage
// to `expired` for every job-closed candidate it finds, which would flip the
// very stage actionable() re-checks before delivery even runs). The nudge here
// is inserted directly as an already-pending application_nudges row instead,
// so only Runner.deliver (claim → actionable re-check → PushNotifier.Send) is
// exercised, exactly what this task is about.
//
// Run with: go test -tags=integration ./internal/nudge/
// Requires Docker (testcontainers spins up a throwaway Postgres with the
// migrations).
package nudge

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/notify"
	"github.com/strelov1/freehire/internal/testdb"
)

// integrationPushTransport is a DB-free pushnotify.Notifier recording every send.
type integrationPushTransport struct {
	sent []string // tokens sent to
}

func (t *integrationPushTransport) Send(_ context.Context, token, _, _ string, _ map[string]string) error {
	t.sent = append(t.sent, token)
	return nil
}

func seedNudgeIntegrationUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// seedClosedJob inserts a job that closed well outside the default
// JobClosedWindowDays (7 days), so Runner.match's own candidate scan never
// picks it up (see the package doc above for why that matters here).
func seedClosedJob(t *testing.T, pool *pgxpool.Pool, externalID string) int64 {
	t.Helper()
	var id int64
	closedAt := time.Now().Add(-30 * 24 * time.Hour)
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug, closed_at)
		 VALUES ('test', $1, 'http://example.test', 'Go Dev', 'Acme', $1, $2)
		 RETURNING id`, externalID, closedAt).Scan(&id); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return id
}

func seedApplication(t *testing.T, pool *pgxpool.Pool, userID, jobID int64, stage string) {
	t.Helper()
	// applied_at is also seeded outside FollowUpWindowDays (30 days), so the
	// same application never surfaces as a follow-up candidate either.
	appliedAt := time.Now().Add(-40 * 24 * time.Hour)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO applications (user_id, job_id, applied_at, stage) VALUES ($1, $2, $3, $4)`,
		userID, jobID, appliedAt, stage); err != nil {
		t.Fatalf("insert application: %v", err)
	}
}

func seedNotificationSettings(t *testing.T, pool *pgxpool.Pool, userID int64, channels []string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO notification_settings (user_id, enabled, channels) VALUES ($1, true, $2)`,
		userID, channels); err != nil {
		t.Fatalf("insert notification_settings: %v", err)
	}
}

func seedPendingJobClosedNudge(t *testing.T, pool *pgxpool.Pool, userID, jobID int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO application_nudges (user_id, job_id, kind, episode_key, status)
		 VALUES ($1, $2, 'job_closed', now(), 'pending') RETURNING id`,
		userID, jobID).Scan(&id); err != nil {
		t.Fatalf("insert application_nudges: %v", err)
	}
	return id
}

func seedPushDevice(t *testing.T, pool *pgxpool.Pool, userID int64, token string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO user_push_tokens (user_id, token, platform) VALUES ($1, $2, 'ios')`,
		userID, token); err != nil {
		t.Fatalf("insert user_push_tokens: %v", err)
	}
}

func nudgeStatus(t *testing.T, pool *pgxpool.Pool, id int64) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM application_nudges WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("read nudge status: %v", err)
	}
	return status
}

func TestPushNotifier_Integration_DeliversToARegisteredDevice(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := seedNudgeIntegrationUser(t, pool, "push-nudge-delivers@example.test")
	jobID := seedClosedJob(t, pool, "push-nudge-delivers")
	seedApplication(t, pool, userID, jobID, "applied")
	seedNotificationSettings(t, pool, userID, []string{"push"})
	seedPushDevice(t, pool, userID, "ExponentPushToken[delivers]")
	nudgeID := seedPendingJobClosedNudge(t, pool, userID, jobID)

	transport := &integrationPushTransport{}
	router := Router{notify.ChannelPush: NewPushNotifier(queries, transport)}
	runner := New(queries, router, DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Delivered != 1 {
		t.Errorf("stats.Delivered = %d, want 1", stats.Delivered)
	}
	if len(transport.sent) != 1 || transport.sent[0] != "ExponentPushToken[delivers]" {
		t.Errorf("sent = %v, want [ExponentPushToken[delivers]]", transport.sent)
	}
	if status := nudgeStatus(t, pool, nudgeID); status != "delivered" {
		t.Errorf("nudge status = %q, want %q", status, "delivered")
	}
}

func TestPushNotifier_Integration_SoftSkipsWithNoRegisteredDevice(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := seedNudgeIntegrationUser(t, pool, "push-nudge-softskip@example.test")
	jobID := seedClosedJob(t, pool, "push-nudge-softskip")
	seedApplication(t, pool, userID, jobID, "applied")
	seedNotificationSettings(t, pool, userID, []string{"push"})
	// No user_push_tokens row: HasPushDevice reads false, so recipient() should
	// soft-skip the push channel without ever calling the transport.
	nudgeID := seedPendingJobClosedNudge(t, pool, userID, jobID)

	transport := &integrationPushTransport{}
	router := Router{notify.ChannelPush: NewPushNotifier(queries, transport)}
	runner := New(queries, router, DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.SoftSkips != 1 {
		t.Errorf("stats.SoftSkips = %d, want 1", stats.SoftSkips)
	}
	if len(transport.sent) != 0 {
		t.Errorf("sent = %v, want none (no registered device)", transport.sent)
	}
	if status := nudgeStatus(t, pool, nudgeID); status != "pending" {
		t.Errorf("nudge status = %q, want %q (retried next pass, not failed)", status, "pending")
	}
}
