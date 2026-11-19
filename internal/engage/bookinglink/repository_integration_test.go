//go:build integration

package bookinglink_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	aiplan "github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/engage/bookinglink"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type paid struct{}

func (paid) RequirePaid(context.Context, int64) (aiplan.Tier, error) { return "", nil }

// nextWeekday10 is 10:00 Berlin on a weekday 2-6 days out: past the notice, inside the
// horizon, and never a weekend.
func nextWeekday10(t *testing.T) time.Time {
	t.Helper()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	d := time.Now().In(berlin).AddDate(0, 0, 2)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, 1)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 10, 0, 0, 0, berlin).UTC()
}

// A double book loses to the EXCLUDE as ErrSlotUnavailable, even when both requests race
// past the re-derivation; the winner's interview and ledger event are on the application.
func TestBookingTwiceIsRefusedAndTheWinnerShowsOnTheApplication(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	var user, app int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('bl@example.test') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO applications (user_id, company_slug, role_title, applied_at) VALUES ($1, 'acme', 'Engineer', now()) RETURNING id`,
		user).Scan(&app); err != nil {
		t.Fatal(err)
	}

	svc := bookinglink.New(bookinglink.NewQueriesRepository(db.New(pool), pool), paid{}, []byte("k"))
	tok, err := svc.Enable(ctx, user, app, "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enable(ctx, user+1, app, "Europe/Berlin"); !errors.Is(err, bookinglink.ErrNotFound) {
		t.Fatalf("someone else's application: %v", err)
	}

	at := nextWeekday10(t)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = svc.Book(ctx, bookinglink.BookInput{Token: tok, StartsAt: at, Name: "R" + strconv.Itoa(i), Email: "r@acme.com"})
		}()
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, bookinglink.ErrSlotUnavailable):
			t.Fatalf("a losing booking must be ErrSlotUnavailable, got %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d bookings won the same slot, want 1", won)
	}

	var source, status string
	var startsAt time.Time
	if err := pool.QueryRow(ctx,
		`SELECT source, status, starts_at FROM application_interviews WHERE application_id = $1`, app,
	).Scan(&source, &status, &startsAt); err != nil {
		t.Fatalf("interview row: %v", err)
	}
	if source != bookinglink.InterviewSource || status != "confirmed" || !startsAt.Equal(at) {
		t.Fatalf("interview = %s/%s at %v", source, status, startsAt)
	}
	var events int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM application_events WHERE application_id = $1 AND kind = 'interview_scheduled'`, app,
	).Scan(&events); err != nil || events != 1 {
		t.Fatalf("ledger events = %d, %v; want 1", events, err)
	}

	// A confirmed meeting freehire already shows on another application (no end: held 30
	// minutes) is busy time too.
	calendar := at.Add(2 * time.Hour)
	if _, err := pool.Exec(ctx, `INSERT INTO application_interviews
		(user_id, application_id, ical_uid, starts_at, status, source)
		VALUES ($1, $2, 'cal-1', $3, 'confirmed', 'calendar_google')`, user, app, calendar); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Book(ctx, bookinglink.BookInput{Token: tok, StartsAt: calendar, Name: "R", Email: "r@acme.com"}); !errors.Is(err, bookinglink.ErrSlotUnavailable) {
		t.Fatalf("booking over a calendar interview: %v", err)
	}

	res, err := svc.Slots(ctx, tok, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	offered := map[time.Time]bool{}
	for _, s := range res.Slots {
		offered[s.Start.UTC()] = true
	}
	if offered[at] || offered[calendar] {
		t.Fatal("a busy slot is still offered")
	}
	if !offered[calendar.Add(30*time.Minute)] {
		t.Fatal("the slot after a 30-minute open-ended interview should be free")
	}
}
