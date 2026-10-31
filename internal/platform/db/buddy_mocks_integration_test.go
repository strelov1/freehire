//go:build integration

// Integration tests for the buddy mock-interview schema: the per-person EXCLUDE that keeps
// one member out of two overlapping mocks whatever their role, the single-statement booking
// guard, and the two cancellations that live in SQL — leaving the network and a block.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// mockMembers seeds n accounts and joins each to the buddy network.
func mockMembers(t *testing.T, pool *pgxpool.Pool, q *Queries, n int) []int64 {
	t.Helper()
	ids := make([]int64, n)
	for i := range ids {
		ids[i] = seedAccount(t, pool, fmt.Sprintf("mock-%d@example.test", i))
		if _, err := q.JoinBuddyMember(context.Background(), JoinBuddyMemberParams{
			UserID: ids[i], Handle: fmt.Sprintf("mock-member-%d", i),
		}); err != nil {
			t.Fatalf("join %d: %v", ids[i], err)
		}
	}
	return ids
}

// mockAt is a whole hour two days ahead plus offset: far from now and from any clock edge.
func mockAt(offset time.Duration) time.Time {
	return time.Now().UTC().Truncate(time.Hour).Add(48*time.Hour + offset)
}

func bookMock(ctx context.Context, q *Queries, host, guest int64, start time.Time) (pgtype.UUID, error) {
	return q.CreateBuddyMock(ctx, CreateBuddyMockParams{
		HostUserID:    host,
		GuestUserID:   guest,
		StartsAt:      pgtype.Timestamptz{Time: start, Valid: true},
		EndsAt:        pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
		Topic:         "coding",
		GuestTimezone: "UTC",
	})
}

func mustBookMock(t *testing.T, q *Queries, host, guest int64, start time.Time) pgtype.UUID {
	t.Helper()
	id, err := bookMock(context.Background(), q, host, guest, start)
	if err != nil {
		t.Fatalf("book %d with %d at %v: %v", host, guest, start, err)
	}
	return id
}

// mockState reads a mock's status, why it was cancelled, and how many of its two
// participant rows still hold time.
func mockState(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID) (status, reason string, active int64) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `
		SELECT m.status, coalesce(m.cancel_reason, ''),
		       (SELECT count(*) FROM buddy_mock_participants p WHERE p.mock_id = m.id AND p.active)
		FROM buddy_mocks m WHERE m.id = $1`, id).Scan(&status, &reason, &active); err != nil {
		t.Fatalf("read mock: %v", err)
	}
	return status, reason, active
}

func TestBuddyMockParticipantsCannotOverlapInEitherRole(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 3)
	a, b, c := m[0], m[1], m[2]

	mustBookMock(t, q, a, b, mockAt(0))
	// a hosts at 0; being c's guest at +30m overlaps a's own row although the role differs.
	_, err := bookMock(ctx, q, c, a, mockAt(30*time.Minute))
	if name, ok := pgerr.ExclusionViolationConstraint(err); !ok || name != "buddy_mock_participants_no_overlap" {
		t.Fatalf("overlap as guest: err = %v, want the participants exclusion", err)
	}
	// Half-open bounds, as the slot engine's: back to back is not an overlap.
	mustBookMock(t, q, c, a, mockAt(time.Hour))

	// The refused insert was one statement and left no orphan mock row behind.
	var n int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM buddy_mocks`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("mock rows = %d (err %v), want 2", n, err)
	}
}

func TestCancelBuddyMockFreesBothParticipants(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 3)
	a, b, c := m[0], m[1], m[2]

	id := mustBookMock(t, q, a, b, mockAt(0))
	got, err := q.CancelBuddyMock(ctx, CancelBuddyMockParams{ID: id, UserID: b})
	if err != nil || got != id {
		t.Fatalf("cancel = %v, %v", got, err)
	}
	if status, reason, active := mockState(t, pool, id); status != "cancelled" || reason != "participant" || active != 0 {
		t.Fatalf("after cancel: %s/%s/%d active", status, reason, active)
	}
	// Both people's time is free again, in either role.
	mustBookMock(t, q, b, a, mockAt(0))

	other := mustBookMock(t, q, a, b, mockAt(4*time.Hour))
	if _, err := q.CancelBuddyMock(ctx, CancelBuddyMockParams{ID: other, UserID: c}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a stranger's cancel: err = %v, want no rows", err)
	}
	if _, err := q.CancelBuddyMock(ctx, CancelBuddyMockParams{ID: id, UserID: a}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cancelling twice: err = %v, want no rows", err)
	}
}

func TestBlockCancelsThePairsFutureMocks(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 3)
	a, b, c := m[0], m[1], m[2]

	pair := mustBookMock(t, q, a, b, mockAt(0))
	unrelated := mustBookMock(t, q, a, c, mockAt(2*time.Hour))
	// A mock already under way is history rather than a plan, and the block leaves it alone.
	var started pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO buddy_mocks (host_user_id, guest_user_id, starts_at, ends_at, topic, guest_timezone)
		VALUES ($1, $2, now() - interval '10 minutes', now() + interval '50 minutes', 'coding', 'UTC')
		RETURNING id`, b, a).Scan(&started); err != nil {
		t.Fatalf("seed started mock: %v", err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, b, a); err != nil {
		t.Fatalf("block: %v", err)
	}

	if status, reason, active := mockState(t, pool, pair); status != "cancelled" || reason != "blocked" || active != 0 {
		t.Fatalf("pair mock after block: %s/%s/%d active", status, reason, active)
	}
	var by int64
	if err := pool.QueryRow(ctx, `SELECT cancelled_by FROM buddy_mocks WHERE id = $1`, pair).Scan(&by); err != nil || by != b {
		t.Fatalf("cancelled_by = %d (err %v), want the blocker %d", by, err, b)
	}
	if status, _, _ := mockState(t, pool, unrelated); status != "confirmed" {
		t.Fatalf("a mock with someone else was cancelled: %s", status)
	}
	if status, _, _ := mockState(t, pool, started); status != "confirmed" {
		t.Fatalf("a started mock was cancelled: %s", status)
	}
}

func TestLeaveBuddyMemberCancelsTheirFutureMocks(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 3)
	a, b, c := m[0], m[1], m[2]

	asHost := mustBookMock(t, q, a, b, mockAt(0))
	asGuest := mustBookMock(t, q, c, a, mockAt(2*time.Hour))
	unrelated := mustBookMock(t, q, b, c, mockAt(4*time.Hour))

	if _, err := q.LeaveBuddyMember(ctx, a); err != nil {
		t.Fatalf("leave: %v", err)
	}
	for name, id := range map[string]pgtype.UUID{"as host": asHost, "as guest": asGuest} {
		if status, reason, active := mockState(t, pool, id); status != "cancelled" || reason != "left" || active != 0 {
			t.Fatalf("%s after leave: %s/%s/%d active", name, status, reason, active)
		}
	}
	if status, _, _ := mockState(t, pool, unrelated); status != "confirmed" {
		t.Fatalf("someone else's mock was cancelled by the leave: %s", status)
	}
}

func TestCreateBuddyMockRefusesAPairThatCannotMeet(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 3)
	a, b, c := m[0], m[1], m[2]

	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, b, a); err != nil {
		t.Fatalf("block: %v", err)
	}
	if _, err := bookMock(ctx, q, a, b, mockAt(0)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("booking across a block: err = %v, want no rows", err)
	}
	if _, err := q.LeaveBuddyMember(ctx, c); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if _, err := bookMock(ctx, q, a, c, mockAt(0)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("booking a member who left: err = %v, want no rows", err)
	}
}

func TestClaimBuddyMockCancelNoticesOnce(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 2)
	a, b := m[0], m[1]

	id := mustBookMock(t, q, a, b, mockAt(0))
	if _, err := q.CancelBuddyMock(ctx, CancelBuddyMockParams{ID: id, UserID: a}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	first, err := q.ClaimBuddyMockCancelNotices(ctx, a)
	if err != nil || len(first) != 1 || first[0] != id {
		t.Fatalf("first claim = %v, %v", first, err)
	}
	if again, err := q.ClaimBuddyMockCancelNotices(ctx, a); err != nil || len(again) != 0 {
		t.Fatalf("second claim = %v, %v, want nothing", again, err)
	}
}
