package bookinglink

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/slots"
)

type calendarBusyFunc func(context.Context, int64, time.Time, time.Time) ([]slots.Interval, error)

func (f calendarBusyFunc) Busy(ctx context.Context, ownerID int64, from, to time.Time) ([]slots.Interval, error) {
	return f(ctx, ownerID, from, to)
}

func TestCalendarBusySlotsAreWithheldAndRecheckedAtBooking(t *testing.T) {
	ctx := context.Background()
	svc, repo, token := enabled(t)
	start := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	busy := slots.Interval{Start: start.Add(10 * time.Minute), End: start.Add(20 * time.Minute)}
	svc.calendar = calendarBusyFunc(func(_ context.Context, userID int64, from, to time.Time) ([]slots.Interval, error) {
		if userID != owner || from.After(busy.Start) || to.Before(busy.End) {
			t.Fatalf("calendar query for %d, [%v, %v) does not cover the owner's meeting", userID, from, to)
		}
		return []slots.Interval{busy}, nil
	})
	got, err := svc.Slots(ctx, token, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	hasSlot := func(at time.Time) bool {
		return slices.ContainsFunc(got.Slots, func(slot slots.Interval) bool { return slot.Start.Equal(at) })
	}
	if hasSlot(start) || !hasSlot(start.Add(30*time.Minute)) {
		t.Fatal("calendar overlap must hide the first slot and keep the adjacent slot")
	}

	// The next slot was listed, then a new event appeared in Google.
	busy = slots.Interval{Start: start.Add(35 * time.Minute), End: start.Add(45 * time.Minute)}
	_, err = svc.Book(ctx, BookInput{Token: token, StartsAt: start.Add(30 * time.Minute), Name: "Jane", Email: "jane@example.org"})
	if !errors.Is(err, ErrSlotUnavailable) || len(repo.booked) != 0 {
		t.Fatalf("booking a stale slot: err = %v, bookings = %d", err, len(repo.booked))
	}
}

func TestCalendarReadFailureDoesNotOfferOrBookSlots(t *testing.T) {
	ctx := context.Background()
	svc, repo, token := enabled(t)
	svc.calendar = calendarBusyFunc(func(context.Context, int64, time.Time, time.Time) ([]slots.Interval, error) {
		return nil, errors.New("calendar unreachable")
	})
	if got, err := svc.Slots(ctx, token, "UTC"); !errors.Is(err, ErrCalendarUnavailable) || len(got.Slots) != 0 {
		t.Fatalf("listing while calendar is unavailable: slots = %d, err = %v", len(got.Slots), err)
	}
	start := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	_, err := svc.Book(ctx, BookInput{Token: token, StartsAt: start, Name: "Jane", Email: "jane@example.org"})
	if !errors.Is(err, ErrCalendarUnavailable) || len(repo.booked) != 0 {
		t.Fatalf("booking while calendar is unavailable: err = %v, bookings = %d", err, len(repo.booked))
	}
}
