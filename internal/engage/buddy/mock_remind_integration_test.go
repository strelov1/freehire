//go:build integration

package buddy_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestAMockBooked13HoursAheadGetsOnlyTheOneHourReminder(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(13*time.Hour))

	if stats, err := mocks.SendDueMockReminders(ctx, 100); err != nil || stats.Sent != 0 {
		t.Fatalf("13h ahead: %+v, %v; want nothing (the 24-hour window never held it)", stats, err)
	}
	// Twelve and a half hours pass.
	if _, err := pool.Exec(ctx, `UPDATE buddy_mocks SET starts_at = now() + interval '30 minutes',
		ends_at = now() + interval '90 minutes' WHERE id = $1::uuid`, id.String()); err != nil {
		t.Fatal(err)
	}
	stats, err := mocks.SendDueMockReminders(ctx, 100)
	if err != nil || stats.Sent != 1 {
		t.Fatalf("30m ahead: %+v, %v", stats, err)
	}
	if want := id.String() + ":1h0m0s"; len(n.reminded) != 1 || n.reminded[0] != want {
		t.Fatalf("reminded = %v, want [%s]", n.reminded, want)
	}
	if stats, err := mocks.SendDueMockReminders(ctx, 100); err != nil || stats.Sent != 0 {
		t.Fatalf("a rerun sent %+v, %v", stats, err)
	}
}

func TestTheTwentyFourHourReminderGoesOut(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(23*time.Hour+30*time.Minute))

	if stats, err := mocks.SendDueMockReminders(context.Background(), 100); err != nil || stats.Sent != 1 {
		t.Fatalf("stats = %+v, %v", stats, err)
	}
	if want := id.String() + ":24h0m0s"; len(n.reminded) != 1 || n.reminded[0] != want {
		t.Fatalf("reminded = %v, want [%s]", n.reminded, want)
	}
}

func TestAFailedReminderIsReleasedAndRetried(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	seedMock(t, pool, host, guest, time.Now().Add(30*time.Minute))
	n.failReminders = 1

	stats, err := mocks.SendDueMockReminders(ctx, 100)
	if err != nil || stats.Failed != 1 || stats.Sent != 0 {
		t.Fatalf("failing run = %+v, %v", stats, err)
	}
	var claims int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM buddy_mock_reminders`).Scan(&claims); err != nil || claims != 0 {
		t.Fatalf("claims after a failed delivery = %d (err %v), want the claim released", claims, err)
	}
	if stats, err := mocks.SendDueMockReminders(ctx, 100); err != nil || stats.Sent != 1 {
		t.Fatalf("retry = %+v, %v", stats, err)
	}
}

func TestAStartedMockIsNotReminded(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	seedMock(t, pool, host, guest, time.Now().Add(-5*time.Minute))

	if stats, err := mocks.SendDueMockReminders(context.Background(), 100); err != nil || stats.Sent != 0 {
		t.Fatalf("stats = %+v, %v", stats, err)
	}
}

// cancelBeforeClaim cancels the mock between the due read and the claim, the window a
// long batch leaves open.
type cancelBeforeClaim struct {
	*buddy.QueriesRepository
	pool *pgxpool.Pool
}

func (c *cancelBeforeClaim) ClaimMockReminder(ctx context.Context, id uuid.UUID, offset time.Duration) (bool, error) {
	if _, err := c.pool.Exec(ctx, `UPDATE buddy_mocks SET status = 'cancelled', cancelled_at = now(),
		cancelled_by = host_user_id, cancel_reason = 'participant' WHERE id = $1::uuid`, id.String()); err != nil {
		return false, err
	}
	return c.QueriesRepository.ClaimMockReminder(ctx, id, offset)
}

func TestAMockCancelledMidRunIsNotReminded(t *testing.T) {
	pool, svc, _, _ := mockSetup(t)
	n := &recordingNotifier{}
	mocks := buddy.NewMockService(svc, &cancelBeforeClaim{buddy.NewQueriesRepository(db.New(pool)), pool}, n)
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	seedMock(t, pool, host, guest, time.Now().Add(30*time.Minute))

	if stats, err := mocks.SendDueMockReminders(context.Background(), 100); err != nil || stats.Sent != 0 || stats.Failed != 0 || len(n.reminded) != 0 {
		t.Fatalf("stats = %+v, %v, reminded %v; a cancelled mock must not be reminded", stats, err, n.reminded)
	}
}
