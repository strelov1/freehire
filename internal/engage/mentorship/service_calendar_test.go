package mentorship

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// calendarMonthService mirrors bookingService's fixed clock, so "now" sits safely before
// the Tuesday-evening availability bookableMentor sets up.
func calendarMonthService(repo *fakeRepo) *Service {
	return New(repo, Config{
		Now: func() time.Time {
			return time.Date(2026, time.September, 7, 9, 0, 0, 0, time.UTC)
		},
	})
}

func TestMyCalendarRefusesWhenCallerHasNoProfile(t *testing.T) {
	repo := newFakeRepo()
	svc := calendarMonthService(repo)

	_, err := svc.MyCalendar(context.Background(), 999, 2026, time.September)
	if !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("error = %v, want ErrProfileNotFound", err)
	}
}

func TestMyCalendarReflectsAConfirmedBookingAsBooked(t *testing.T) {
	repo := newFakeRepo()
	profile := bookableMentor(t, repo)
	svc := calendarMonthService(repo)

	booking := tuesdayAt(t, 18)
	bookingID := uuid.New()
	repo.bookings[bookingID] = Booking{
		ID: bookingID, MentorID: profile.ID, Status: BookingConfirmed,
		StartsAt: booking, EndsAt: booking.Add(time.Hour),
	}

	result, err := svc.MyCalendar(context.Background(), profile.UserID, 2026, time.September)
	if err != nil {
		t.Fatalf("MyCalendar: %v", err)
	}

	found := false
	for _, iv := range result.Intervals {
		if iv.Status == StatusBooked && iv.Start.Equal(booking) {
			found = true
		}
	}
	if !found {
		t.Errorf("no booked interval starting at %v in %+v", booking, result.Intervals)
	}
}

func TestMyCalendarReflectsASyncedIntervalAsBusy(t *testing.T) {
	repo := newFakeRepo()
	profile := bookableMentor(t, repo)
	svc := calendarMonthService(repo)

	synced := tuesdayAt(t, 19)
	repo.busyIntervals[profile.ID] = []Interval{{Start: synced, End: synced.Add(30 * time.Minute)}}

	result, err := svc.MyCalendar(context.Background(), profile.UserID, 2026, time.September)
	if err != nil {
		t.Fatalf("MyCalendar: %v", err)
	}

	found := false
	for _, iv := range result.Intervals {
		if iv.Status == StatusBusy && iv.Start.Equal(synced) {
			found = true
		}
	}
	if !found {
		t.Errorf("no busy interval starting at %v in %+v", synced, result.Intervals)
	}
}

// MyCalendar takes no mentor identifier at all — it always resolves the caller's OWN
// profile via ownProfile(userID) — so a second mentor's booking has no way to reach the
// first mentor's breakdown. This is a construction guarantee rather than a checked
// branch, and this test is the regression guard for it.
func TestMyCalendarNeverShowsAnotherMentorsBooking(t *testing.T) {
	repo := newFakeRepo()
	mine := bookableMentor(t, repo)

	otherInput := validInput()
	otherInput.UserID = 999
	otherInput.Slug = "other-mentor"
	other, err := repo.CreateProfile(context.Background(), otherInput)
	if err != nil {
		t.Fatalf("CreateProfile (other mentor): %v", err)
	}
	if _, err := repo.DecideProfile(context.Background(), other.ID, 1, StatusApproved); err != nil {
		t.Fatalf("DecideProfile (other mentor): %v", err)
	}

	otherBooking := tuesdayAt(t, 18)
	otherBookingID := uuid.New()
	repo.bookings[otherBookingID] = Booking{
		ID: otherBookingID, MentorID: other.ID, Status: BookingConfirmed,
		StartsAt: otherBooking, EndsAt: otherBooking.Add(time.Hour),
	}

	svc := calendarMonthService(repo)
	result, err := svc.MyCalendar(context.Background(), mine.UserID, 2026, time.September)
	if err != nil {
		t.Fatalf("MyCalendar: %v", err)
	}

	for _, iv := range result.Intervals {
		if iv.Status == StatusBooked {
			t.Errorf("own calendar shows a booked interval %+v, but the only booking belongs to a different mentor",
				iv)
		}
	}
}
