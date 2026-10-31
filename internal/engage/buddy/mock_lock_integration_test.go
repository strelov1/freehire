//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/platform/db"
)

// raceRounds is how many fresh pairs each race test books against a rival write. One
// overlap is a coin toss; a run of them is what makes the bad interleaving show up.
const raceRounds = 8

// raceAgainst books a fresh pair once per round while rival(host, guest) runs at the same
// moment: the booking's CreateMock and the rival meet at a barrier, so the booking's
// write and the rival's write are in Postgres together. It returns, per round, the pair
// and the booking's error.
func raceAgainst(t *testing.T, rival func(ctx context.Context, pool *pgxpool.Pool, svc *buddy.Service, host, guest int64) error) (*pgxpool.Pool, [][2]int64) {
	t.Helper()
	pool, svc, _, _ := mockSetup(t)
	ctx := context.Background()
	pairs := make([][2]int64, 0, raceRounds)
	for round := range raceRounds {
		repo := &insertBarrier{QueriesRepository: buddy.NewQueriesRepository(db.New(pool), pool)}
		repo.arrived.Add(2)
		mocks := buddy.NewMockService(svc, repo, &recordingNotifier{})
		host, hostHandle := joined(t, pool, svc, fmt.Sprintf("host%d@example.test", round))
		guest, _ := joined(t, pool, svc, fmt.Sprintf("guest%d@example.test", round))
		openAllWeek(t, mocks, host, "UTC")
		at := firstSlot(t, mocks, guest, hostHandle)

		var bookErr, rivalErr error
		var done sync.WaitGroup
		done.Add(2)
		go func() {
			defer done.Done()
			_, bookErr = book(mocks, guest, hostHandle, at, buddy.TopicCoding)
		}()
		go func() {
			defer done.Done()
			repo.arrived.Done()
			repo.arrived.Wait()
			rivalErr = rival(ctx, pool, svc, host, guest)
		}()
		done.Wait()
		if bookErr != nil && !errors.Is(bookErr, buddy.ErrSlotUnavailable) {
			t.Fatalf("round %d: booking: %v", round, bookErr)
		}
		if rivalErr != nil {
			t.Fatalf("round %d: rival: %v", round, rivalErr)
		}
		pairs = append(pairs, [2]int64{host, guest})
	}
	return pool, pairs
}

// confirmedMocks counts the pair's confirmed mocks, which after a block or a leave must be
// none: the booking either lost (nothing written) or was cancelled by the rival.
func confirmedMocks(t *testing.T, pool *pgxpool.Pool, host, guest int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM buddy_mocks
		WHERE host_user_id = $1 AND guest_user_id = $2 AND status = 'confirmed'`, host, guest).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestABookingRacingABlockNeverStaysConfirmed(t *testing.T) {
	pool, pairs := raceAgainst(t, func(ctx context.Context, pool *pgxpool.Pool, _ *buddy.Service, host, guest int64) error {
		_, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, host, guest)
		return err
	})
	for i, p := range pairs {
		if n := confirmedMocks(t, pool, p[0], p[1]); n != 0 {
			t.Errorf("round %d: %d confirmed mock(s) between a blocked pair", i, n)
		}
	}
}

func TestABookingRacingALeaveNeverStaysConfirmed(t *testing.T) {
	pool, pairs := raceAgainst(t, func(ctx context.Context, _ *pgxpool.Pool, svc *buddy.Service, host, _ int64) error {
		return svc.Leave(ctx, host)
	})
	for i, p := range pairs {
		if n := confirmedMocks(t, pool, p[0], p[1]); n != 0 {
			t.Errorf("round %d: %d confirmed mock(s) with a host who left", i, n)
		}
	}
}

// waitForLockWait reports whether some backend is waiting on a lock while running a
// statement that contains fragment, polling until done closes or a deadline passes.
func waitForLockWait(t *testing.T, pool *pgxpool.Pool, fragment string, done <-chan struct{}) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			return false
		default:
		}
		var n int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock' AND strpos(query, $1) > 0`,
			fragment).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// The deterministic half of the block race: the block is written but not yet committed
// when the booking runs. The booking must wait for it and then see it.
func TestABookingWaitsForAnUncommittedBlock(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	at := firstSlot(t, mocks, guest, hostHandle)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, host, guest); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var bookErr error
	go func() {
		defer close(done)
		_, bookErr = book(mocks, guest, hostHandle, at, buddy.TopicCoding)
	}()
	waited := waitForLockWait(t, pool, "FOR SHARE", done)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	<-done
	if !waited {
		t.Error("the booking did not wait for the uncommitted block")
	}
	if !errors.Is(bookErr, buddy.ErrSlotUnavailable) {
		t.Errorf("booking err = %v, want ErrSlotUnavailable", bookErr)
	}
	if n := confirmedMocks(t, pool, host, guest); n != 0 {
		t.Errorf("%d confirmed mock(s) between a blocked pair", n)
	}
}

// blockBypassingTrigger writes a block the way the lost race used to leave it: standing,
// with the pair's mock still confirmed.
func blockBypassingTrigger(t *testing.T, pool *pgxpool.Pool, blocker, blocked int64) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`ALTER TABLE user_blocks DISABLE TRIGGER buddy_mocks_cancel_on_block`,
		fmt.Sprintf(`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES (%d, %d)`, blocker, blocked),
		`ALTER TABLE user_blocks ENABLE TRIGGER buddy_mocks_cancel_on_block`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
}

// leftBypassingCancel marks a member left without cancelling their mocks.
func leftBypassingCancel(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE buddy_members SET left_at = now() WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
}

// Whatever state a write left behind, the link only leaves the database for a confirmed
// mock between two active, unblocked members, and no reminder is sent for anything else.
func TestTheMeetingLinkIsWithheldFromAPairThatCannotMeet(t *testing.T) {
	for _, tc := range []struct {
		name string
		cut  func(t *testing.T, pool *pgxpool.Pool, host, guest int64)
	}{
		{"host blocked guest", func(t *testing.T, pool *pgxpool.Pool, host, guest int64) { blockBypassingTrigger(t, pool, host, guest) }},
		{"guest blocked host", func(t *testing.T, pool *pgxpool.Pool, host, guest int64) { blockBypassingTrigger(t, pool, guest, host) }},
		{"host left", func(t *testing.T, pool *pgxpool.Pool, host, _ int64) { leftBypassingCancel(t, pool, host) }},
		{"guest left", func(t *testing.T, pool *pgxpool.Pool, _, guest int64) { leftBypassingCancel(t, pool, guest) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc, mocks, n := mockSetup(t)
			ctx := context.Background()
			host, _ := joined(t, pool, svc, "host@example.test")
			guest, _ := joined(t, pool, svc, "guest@example.test")
			openAllWeek(t, mocks, host, "UTC")
			id := seedMock(t, pool, host, guest, time.Now().Add(30*time.Minute))
			tc.cut(t, pool, host, guest)

			if status, _ := storedMock(t, pool, id); status != buddy.MockConfirmed {
				t.Fatalf("setup: mock is %s, want it left confirmed", status)
			}
			for _, viewer := range []int64{host, guest} {
				list, err := mocks.List(ctx, viewer)
				if errors.Is(err, buddy.ErrNotMember) {
					continue // a member who left has no list to read
				}
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range append(list.Upcoming, list.Past...) {
					if e.MeetingURL != "" {
						t.Errorf("viewer %d: list carries the link %q", viewer, e.MeetingURL)
					}
				}
			}
			repo := buddy.NewQueriesRepository(db.New(pool), pool)
			m, found, err := repo.MockByID(ctx, id)
			if err != nil || !found {
				t.Fatalf("by id: found %v, err %v", found, err)
			}
			if m.MeetingURL != "" {
				t.Errorf("by id carries the link %q", m.MeetingURL)
			}
			claimed, err := repo.ClaimMockReminder(ctx, id, time.Hour)
			if err != nil || claimed {
				t.Errorf("reminder claim = %v (err %v), want refused", claimed, err)
			}
			if stats, err := mocks.SendDueMockReminders(ctx, 100); err != nil || stats.Sent != 0 || len(n.reminded) != 0 {
				t.Errorf("reminders = %+v (err %v), reminded %v; want none", stats, err, n.reminded)
			}
		})
	}
}

// The same reads still carry the link for a pair that can meet, so the tests above are
// not passing on a link that is never there.
func TestTheMeetingLinkReachesAPairThatCanMeet(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	id := seedMock(t, pool, host, guest, time.Now().Add(30*time.Minute))

	list, err := mocks.List(ctx, guest)
	if err != nil || len(list.Upcoming) != 1 || list.Upcoming[0].MeetingURL != hostLink(host) {
		t.Fatalf("list = %+v (err %v)", list, err)
	}
	repo := buddy.NewQueriesRepository(db.New(pool), pool)
	if m, _, err := repo.MockByID(ctx, id); err != nil || m.MeetingURL != hostLink(host) {
		t.Fatalf("by id link = %q (err %v)", m.MeetingURL, err)
	}
	if claimed, err := repo.ClaimMockReminder(ctx, id, time.Hour); err != nil || !claimed {
		t.Fatalf("reminder claim = %v (err %v), want claimed", claimed, err)
	}
}
