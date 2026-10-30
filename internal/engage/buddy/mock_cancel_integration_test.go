//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/slots"
)

// storedMock reads a mock's status and cancel reason straight from the table.
func storedMock(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) (status, reason string) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT status, coalesce(cancel_reason, '') FROM buddy_mocks WHERE id = $1::uuid`, id.String(),
	).Scan(&status, &reason); err != nil {
		t.Fatalf("read mock: %v", err)
	}
	return status, reason
}

func TestGuestCancelsFreesTheSlotAndTellsTheHost(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	at := firstSlot(t, mocks, guest, hostHandle)
	m, err := book(mocks, guest, hostHandle, at, buddy.TopicCoding)
	if err != nil {
		t.Fatal(err)
	}

	if err := mocks.Cancel(ctx, guest, m.ID); err != nil {
		t.Fatal(err)
	}
	if status, reason := storedMock(t, pool, m.ID); status != buddy.MockCancelled || reason != buddy.CancelByParticipant {
		t.Fatalf("stored = %s/%s", status, reason)
	}
	if len(n.cancelled) != 1 || n.cancelled[0].ID != m.ID || n.cancelled[0].CancelReason != buddy.CancelByParticipant {
		t.Fatalf("cancel notices = %+v", n.cancelled)
	}
	now := time.Now()
	reopened, err := mocks.OpenSlots(ctx, guest, hostHandle, now, now.AddDate(0, 0, 3), "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(reopened.Slots, func(s slots.Interval) bool { return s.Start.Equal(at) }) {
		t.Fatalf("the %v slot is not offered again", at)
	}
	// Announcing again finds nothing: each notice is claimed once.
	mocks.AnnounceCancellations(ctx, guest)
	if len(n.cancelled) != 1 {
		t.Fatalf("cancel notices after a second announce = %d", len(n.cancelled))
	}
	// Cancelling twice is refused rather than announced twice.
	if err := mocks.Cancel(ctx, guest, m.ID); !errors.Is(err, buddy.ErrMockNotCancellable) {
		t.Fatalf("second cancel: err = %v, want ErrMockNotCancellable", err)
	}
}

func TestAStartedMockCannotBeCancelled(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(-10*time.Minute))

	if err := mocks.Cancel(context.Background(), guest, id); !errors.Is(err, buddy.ErrMockNotCancellable) {
		t.Fatalf("err = %v, want ErrMockNotCancellable", err)
	}
}

func TestAStrangerCannotCancel(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	stranger, _ := joined(t, pool, svc, "stranger@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(48*time.Hour))

	if err := mocks.Cancel(context.Background(), stranger, id); !errors.Is(err, buddy.ErrMockNotFound) {
		t.Fatalf("err = %v, want ErrMockNotFound", err)
	}
	if err := mocks.Cancel(context.Background(), guest, uuid.New()); !errors.Is(err, buddy.ErrMockNotFound) {
		t.Fatalf("unknown id: err = %v, want ErrMockNotFound", err)
	}
}

func TestLeavingCancelsFutureMocksAndTellsTheOtherSide(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	m, err := book(mocks, guest, hostHandle, firstSlot(t, mocks, guest, hostHandle), buddy.TopicCoding)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Leave(ctx, host); err != nil {
		t.Fatal(err)
	}
	mocks.AnnounceCancellations(ctx, host)

	if status, reason := storedMock(t, pool, m.ID); status != buddy.MockCancelled || reason != buddy.CancelByLeave {
		t.Fatalf("stored = %s/%s", status, reason)
	}
	if len(n.cancelled) != 1 || n.cancelled[0].ID != m.ID || n.cancelled[0].CancelReason != buddy.CancelByLeave {
		t.Fatalf("cancel notices = %+v", n.cancelled)
	}
}

func TestBlockingCancelsThePairsFutureMocks(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	m, err := book(mocks, guest, hostHandle, firstSlot(t, mocks, guest, hostHandle), buddy.TopicCoding)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.Invite(ctx, guest, hostHandle, "Hi, looking forward to the mock")
	if err != nil {
		t.Fatal(err)
	}

	if err := chat(pool).Block(ctx, host, conv.ID); err != nil {
		t.Fatal(err)
	}
	mocks.AnnounceCancellations(ctx, host)

	if status, reason := storedMock(t, pool, m.ID); status != buddy.MockCancelled || reason != buddy.CancelByBlock {
		t.Fatalf("stored = %s/%s", status, reason)
	}
	if len(n.cancelled) != 1 || n.cancelled[0].CancelReason != buddy.CancelByBlock {
		t.Fatalf("cancel notices = %+v", n.cancelled)
	}
}
