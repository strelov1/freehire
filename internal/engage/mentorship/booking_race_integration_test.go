//go:build integration

package mentorship

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// bookingBarrier holds every CreateBooking until `parties` callers have reached it, so they
// all write together: each has already passed Book's own re-derivation of the slot, and
// only the database can refuse the losers. The same technique internal/engage/buddy uses
// for mocks, where it surfaced a 40P01 deadlock in place of the 23P01 exclusion violation.
// The wait gives up with the context, so a caller that never reaches the insert fails the
// test instead of hanging it.
type bookingBarrier struct {
	*QueriesRepository
	mu      sync.Mutex
	waiting int
	parties int
	ready   chan struct{}
}

func newBookingBarrier(repo *QueriesRepository, parties int) *bookingBarrier {
	return &bookingBarrier{QueriesRepository: repo, parties: parties, ready: make(chan struct{})}
}

func (b *bookingBarrier) CreateBooking(ctx context.Context, row BookingRow) (Booking, error) {
	b.mu.Lock()
	if b.waiting++; b.waiting == b.parties {
		close(b.ready)
	}
	b.mu.Unlock()
	select {
	case <-b.ready:
	case <-ctx.Done():
		return Booking{}, ctx.Err()
	}
	return b.QueriesRepository.CreateBooking(ctx, row)
}

// seedBookableMentor creates an approved mentor open around the clock and returns a slot
// far enough past the notice edge that it cannot stop being offerable mid-test.
func seedBookableMentor(t *testing.T, pool *pgxpool.Pool, svc *Service, repo Repository, slug string) time.Time {
	t.Helper()
	ctx := context.Background()
	mentorUser := seedUser(t, pool, slug+"@example.test")
	mentor, err := db.New(pool).CreateMentorProfile(ctx, db.CreateMentorProfileParams{
		UserID: mentorUser, Slug: slug, DisplayName: "Race M.",
		Topics: []string{"career"}, Languages: []string{"en"},
		Timezone: "UTC", SessionDurationMin: 60, MinNoticeMin: 60, HorizonDays: 14,
		MeetingUrl: "https://meet.example.test/race", CompanySlug: pgtype.Text{},
	})
	if err != nil {
		t.Fatalf("CreateMentorProfile: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE mentors SET status = 'approved' WHERE id = $1`, mentor.ID); err != nil {
		t.Fatalf("approve mentor: %v", err)
	}
	dayStart, _ := slots.NewTimeOfDay(0, 0)
	dayEnd, _ := slots.NewTimeOfDay(24, 0)
	var week []slots.Rule
	for d := time.Sunday; d <= time.Saturday; d++ {
		r, err := slots.NewWeeklyRule(d, dayStart, dayEnd)
		if err != nil {
			t.Fatal(err)
		}
		week = append(week, r)
	}
	if err := repo.ReplaceWeeklyAvailability(ctx, mentor.ID, week); err != nil {
		t.Fatalf("ReplaceWeeklyAvailability: %v", err)
	}
	now := time.Now()
	open, err := svc.CachedMentorSlots(ctx, slug, now, now.AddDate(0, 0, 3), "UTC")
	if err != nil {
		t.Fatalf("open slots: %v", err)
	}
	for _, slot := range open.Slots {
		if slot.Start.After(now.Add(3 * time.Hour)) {
			return slot.Start
		}
	}
	t.Fatalf("no open slot beyond the notice edge in %v", open.Slots)
	return time.Time{}
}

func TestConcurrentBookingsOfOneSlotConfirmExactlyOne(t *testing.T) {
	pool := testdb.Pool(t)
	repo := newBookingBarrier(NewQueriesRepository(db.New(pool), pool), 2)
	svc := New(repo, Config{})
	at := seedBookableMentor(t, pool, svc, repo, "race-mentor")
	seekers := []int64{seedUser(t, pool, "s1-race@example.test"), seedUser(t, pool, "s2-race@example.test")}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Both goroutines park on start and are released together; the barrier then makes the
	// two inserts run at the same moment, so the EXCLUDE, not a read, decides the winner.
	errs := make([]error, 2)
	start := make(chan struct{})
	var done sync.WaitGroup
	for i, seeker := range seekers {
		done.Add(1)
		go func() {
			defer done.Done()
			<-start
			_, errs[i] = svc.Book(ctx, BookingInput{
				MentorSlug: "race-mentor", SeekerUserID: seeker, StartsAt: at, SeekerTimezone: "UTC",
			})
		}()
	}
	close(start)
	done.Wait()

	won, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case errors.Is(err, ErrSlotUnavailable):
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if won != 1 || refused != 1 {
		t.Fatalf("won %d, refused %d; want one of each", won, refused)
	}
	var confirmed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM mentor_bookings WHERE status = 'confirmed'`).Scan(&confirmed); err != nil || confirmed != 1 {
		t.Fatalf("confirmed bookings = %d (err %v), want 1", confirmed, err)
	}
}

// deadlockOnce answers the booking INSERT with 40P01 for its first `left` calls and passes
// everything else to the real pool, so CreateBooking's retry is exercised deterministically
// rather than only when a real race happens to deadlock.
type deadlockOnce struct {
	db.DBTX
	left    int
	inserts int
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

func (d *deadlockOnce) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "INSERT INTO mentor_bookings") {
		d.inserts++
		if d.left > 0 {
			d.left--
			return errRow{&pgconn.PgError{Code: "40P01", Message: "deadlock detected"}}
		}
	}
	return d.DBTX.QueryRow(ctx, sql, args...)
}

func TestCreateBookingRetriesADeadlock(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	base := NewQueriesRepository(db.New(pool), pool)
	at := seedBookableMentor(t, pool, New(base, Config{}), base, "retry-mentor")
	seeker := seedUser(t, pool, "seeker-retry@example.test")

	for _, tc := range []struct {
		name      string
		deadlocks int
		wantErr   bool
	}{
		{"one deadlock is retried", 1, false},
		{"the retry is bounded", createBookingAttempts, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, `DELETE FROM mentor_bookings`); err != nil {
				t.Fatal(err)
			}
			conn := &deadlockOnce{DBTX: pool, left: tc.deadlocks}
			svc := New(NewQueriesRepository(db.New(conn), pool), Config{})
			_, err := svc.Book(ctx, BookingInput{MentorSlug: "retry-mentor", SeekerUserID: seeker, StartsAt: at, SeekerTimezone: "UTC"})
			if tc.wantErr {
				if !pgerr.IsDeadlock(err) || conn.inserts != createBookingAttempts {
					t.Fatalf("err = %v after %d inserts, want a deadlock after %d", err, conn.inserts, createBookingAttempts)
				}
				return
			}
			if err != nil || conn.inserts != 2 {
				t.Fatalf("err = %v after %d inserts, want success on the second", err, conn.inserts)
			}
		})
	}
}
