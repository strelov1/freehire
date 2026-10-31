//go:build integration

package mentorship

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// bookingBarrier holds every CreateBooking until `parties` callers have reached it, so they
// all write together: each has already passed Book's own re-derivation of the slot, and
// only the database can refuse the losers. The same technique internal/engage/buddy uses
// for mocks, where it surfaced a 40P01 deadlock in place of the 23P01 exclusion violation.
type bookingBarrier struct {
	*QueriesRepository
	arrived sync.WaitGroup
}

func (b *bookingBarrier) CreateBooking(ctx context.Context, row BookingRow) (Booking, error) {
	b.arrived.Done()
	b.arrived.Wait()
	return b.QueriesRepository.CreateBooking(ctx, row)
}

func TestConcurrentBookingsOfOneSlotConfirmExactlyOne(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	queries := db.New(pool)
	repo := &bookingBarrier{QueriesRepository: NewQueriesRepository(queries, pool)}
	repo.arrived.Add(2)
	svc := New(repo, Config{})

	mentorUser := seedUser(t, pool, "mentor-race@example.test")
	seekers := []int64{seedUser(t, pool, "s1-race@example.test"), seedUser(t, pool, "s2-race@example.test")}
	mentor, err := queries.CreateMentorProfile(ctx, db.CreateMentorProfileParams{
		UserID: mentorUser, Slug: "race-mentor", DisplayName: "Race M.",
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
	open, err := svc.CachedMentorSlots(ctx, "race-mentor", now, now.AddDate(0, 0, 3), "UTC")
	if err != nil || len(open.Slots) == 0 {
		t.Fatalf("open slots = %v (err %v)", open.Slots, err)
	}
	at := open.Slots[0].Start

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
