package mentorship

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/application/gmailsync"
)

// The statuses a booking holds. There is no `completed`: completion is not a decision
// anybody makes, it is "confirmed, and the end has passed" — see Booking.Completed. A
// stored status would need a worker to advance it and would be wrong for exactly as long
// as that worker was down.
const (
	BookingConfirmed = "confirmed"
	BookingCancelled = "cancelled"
)

// The sentinels the booking use cases raise, each with the status the handler maps it to.
var (
	// ErrNotAuthenticated → 401. Slots are public; booking one is not.
	ErrNotAuthenticated = errors.New("mentorship: sign in to book a session")
	// ErrCannotBookYourself → 422.
	ErrCannotBookYourself = errors.New("mentorship: you cannot book your own session")
	// ErrSlotUnavailable → 409. THE one answer for every ordinary way a booking fails to
	// land: the hour was taken, the schedule changed, the notice period elapsed, the
	// horizon passed, or another request won the race. They are one event as far as the
	// seeker is concerned — "that time is no longer available" — and splitting them would
	// tell a stranger which.
	ErrSlotUnavailable = errors.New("mentorship: that time is no longer available")
	// ErrBookingNotFound → 404. No booking, or one that is not the caller's. The two are
	// deliberately the same answer.
	ErrBookingNotFound = errors.New("mentorship: booking not found")
	// ErrBookingNotCancellable → 409. Already cancelled, or already begun.
	ErrBookingNotCancellable = errors.New("mentorship: this session can no longer be cancelled")
	// ErrSessionNotCompleted → 409. A review of a session that has not happened.
	ErrSessionNotCompleted = errors.New("mentorship: this session has not taken place")
	// ErrInvalidReview → 422.
	ErrInvalidReview = errors.New("mentorship: invalid review")
)

// Review rating bounds, matching the mentor_reviews_rating_check CHECK.
const (
	minRating = 1
	maxRating = 5
)

// BookingInput is a request to take one of a mentor's offered hours.
type BookingInput struct {
	MentorSlug   string
	SeekerUserID int64
	// StartsAt is the slot's start as the seeker saw it. The END is not taken from the
	// client: it is the mentor's session length, so a crafted request cannot book four
	// hours of somebody who offers one.
	StartsAt       time.Time
	SeekerTimezone string
	Note           string
	// JobID records the vacancy the seeker came from, as context. Zero means none.
	JobID int64
}

// Review is one seeker's rating of one completed session.
type Review struct {
	BookingID uuid.UUID
	MentorID  int64
	Rating    int
	Comment   string
}

// Completed reports whether this session has taken place: confirmed, and its end has
// passed. A cancelled session never completes, however long ago it was.
func (b Booking) Completed(now time.Time) bool {
	return b.Status == BookingConfirmed && !now.Before(b.EndsAt)
}

// Book takes one of a mentor's offered hours.
//
// The slot is RE-DERIVED here from the mentor's live schedule and busy set rather than
// trusted from the request. That is not the race guard — the database's EXCLUDE
// constraint is, and it is the only thing that can be — it is what lets an ordinary
// refusal carry a reason: a page loaded ten minutes ago may be offering an hour the
// mentor has since removed, and re-deriving is how we know.
//
// Every ordinary failure returns the same ErrSlotUnavailable, including the lost race the
// repository translates. To the seeker a stale tab and a race are one event, and telling
// them apart would only tell a stranger which.
func (s *Service) Book(ctx context.Context, in BookingInput) (Booking, error) {
	if in.SeekerUserID == 0 {
		return Booking{}, ErrNotAuthenticated
	}

	mentor, err := s.PublicProfile(ctx, in.MentorSlug)
	if err != nil {
		return Booking{}, err
	}
	if mentor.UserID == in.SeekerUserID {
		return Booking{}, ErrCannotBookYourself
	}

	offerable, err := s.slotIsOfferable(ctx, mentor, in.StartsAt)
	if err != nil {
		return Booking{}, err
	}
	if !offerable {
		return Booking{}, ErrSlotUnavailable
	}

	booking, err := s.repo.CreateBooking(ctx, BookingRow{
		MentorID:       mentor.ID,
		SeekerUserID:   in.SeekerUserID,
		StartsAt:       in.StartsAt,
		EndsAt:         in.StartsAt.Add(mentor.Session.Duration),
		JobID:          in.JobID,
		Note:           in.Note,
		SeekerTimezone: in.SeekerTimezone,
		// A snapshot. A mentor changing their link afterwards must not rewrite the
		// invitation somebody already has in their calendar. Overwritten below when the
		// mentor's calendar mints a real one instead.
		MeetingURL: mentor.MeetingURL,
	})
	if err != nil {
		return Booking{}, err
	}

	if s.calendar != nil {
		s.attachMeetEvent(ctx, mentor, &booking)
	}

	// Best-effort, and after the write: the seeker holds the hour whether or not the
	// message lands, and a confirmation can be resent while a lost hour cannot.
	if s.notifier != nil {
		if err := s.notifier.BookingConfirmed(ctx, booking); err != nil {
			logDeliveryFailure("confirmation", booking, err)
		}
	}
	return booking, nil
}

// attachMeetEvent asks the mentor's calendar for a Meet-carrying event once a booking has
// committed, and reconciles the row to what actually happened. Every path is best-effort:
// the seeker already holds the hour, and a booking with no link — or a stale static one —
// is recoverable, while a lost hour is not.
func (s *Service) attachMeetEvent(ctx context.Context, mentor Profile, booking *Booking) {
	eventID, meetLink, err := s.calendar.CreateMeetEvent(ctx, mentor.UserID, MeetEventInput{
		StartsAt:    booking.StartsAt,
		EndsAt:      booking.EndsAt,
		SeekerEmail: booking.SeekerEmail,
		Summary:     "Mentorship session with " + mentor.DisplayName,
	})
	switch {
	case errors.Is(err, ErrCalendarNotConnected):
		// The row already carries the mentor's static link from CreateBooking above —
		// today's behaviour, unchanged.
		return
	case err != nil:
		logDeliveryFailure("calendar event", *booking, err)
		// This mentor DOES hold a calendar.events grant — CreateMeetEvent would have
		// answered ErrCalendarNotConnected above otherwise — so the static link the row
		// still carries is stale rather than a fallback worth keeping; an explicit empty
		// answer beats surfacing a link nobody chose for this session.
		if setErr := s.setCalendarEvent(ctx, booking, "", ""); setErr != nil {
			logDeliveryFailure("calendar event reset", *booking, setErr)
		}
		if gmailsync.RevokedGrant(err) {
			if markErr := s.repo.MarkCalendarGrantNeedsReconsent(ctx, mentor.UserID); markErr != nil {
				log.Printf("mentorship: marking mentor %d needs_reconsent: %v", mentor.UserID, markErr)
			}
		}
	default:
		if setErr := s.setCalendarEvent(ctx, booking, meetLink, eventID); setErr != nil {
			logDeliveryFailure("calendar event record", *booking, setErr)
		}
	}
}

// setCalendarEvent writes a booking's calendar-event fields and, on success, patches the
// same values onto the in-memory booking — used both when a real event was minted and
// when a failed write clears a now-stale static link, so the caller sees what was
// actually stored either way.
func (s *Service) setCalendarEvent(ctx context.Context, booking *Booking, meetingURL, eventID string) error {
	if err := s.repo.SetBookingCalendarEvent(ctx, booking.ID, meetingURL, eventID); err != nil {
		return err
	}
	booking.MeetingURL = meetingURL
	booking.GoogleEventID = eventID
	return nil
}

// deleteMeetEventBestEffort removes the calendar event a booking minted, if any, ignoring
// (but logging) any failure — used by both Cancel and Withdraw's bulk cancellation, so a
// mentor leaving the marketplace does not leave stray Meet events behind on their own
// calendar for sessions freehire has already told everyone are off.
func (s *Service) deleteMeetEventBestEffort(ctx context.Context, booking Booking) {
	if booking.GoogleEventID == "" || s.calendar == nil {
		return
	}
	if err := s.calendar.DeleteMeetEvent(ctx, booking.MentorUserID, booking.GoogleEventID); err != nil {
		logDeliveryFailure("calendar event deletion", booking, err)
	}
}

// Cancel ends a confirmed session before it starts. Either party may; nobody else can
// learn that the booking exists.
func (s *Service) Cancel(ctx context.Context, bookingID uuid.UUID, actorID int64, reason string) (Booking, error) {
	booking, err := s.repo.CancelBooking(ctx, bookingID, actorID, reason)
	if err != nil {
		return Booking{}, err
	}

	by := CancelledBySeeker
	if actorID == booking.MentorUserID {
		by = CancelledByMentor
	}
	s.notifyCancelled(ctx, []Booking{booking}, by, reason)
	s.deleteMeetEventBestEffort(ctx, booking)

	return booking, nil
}

// Session is one booking, readable ONLY by its two parties.
//
// A random identifier makes bookings unenumerable; this makes a leaked or guessed one
// useless. They are different defences and neither substitutes for the other. A caller
// who is neither party gets the same answer as one naming a booking that does not exist,
// so the endpoint never confirms that somebody else's session is real.
func (s *Service) Session(ctx context.Context, bookingID uuid.UUID, callerID int64) (Booking, error) {
	if callerID == 0 {
		return Booking{}, ErrNotAuthenticated
	}
	booking, found, err := s.repo.BookingByID(ctx, bookingID)
	if err != nil {
		return Booking{}, err
	}
	if !found || (booking.SeekerUserID != callerID && booking.MentorUserID != callerID) {
		return Booking{}, ErrBookingNotFound
	}
	return booking, nil
}

// MySessions is the seeker's own bookings; MentorSessions the mentor's. Each is readable
// only by that party, which the queries enforce by keying on the caller.
func (s *Service) MySessions(ctx context.Context, seekerID int64, limit int32) ([]Booking, error) {
	return s.repo.ListBookingsBySeeker(ctx, seekerID, pageSize(limit))
}

// MentorSessions is the mentor's own list of who is coming.
func (s *Service) MentorSessions(ctx context.Context, userID int64, limit int32) ([]Booking, error) {
	profile, err := s.ownProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListBookingsByMentor(ctx, profile.ID, pageSize(limit))
}

// Review records or replaces the seeker's rating of a completed session. Only that
// seeker may, and only once the session has actually taken place — the alternative is a
// rating written before the meeting it rates.
func (s *Service) Review(ctx context.Context, bookingID uuid.UUID, seekerID int64, rating int, comment string) (Review, error) {
	if rating < minRating || rating > maxRating {
		return Review{}, fmt.Errorf("%w: a rating is %d to %d", ErrInvalidReview, minRating, maxRating)
	}

	booking, found, err := s.repo.BookingByID(ctx, bookingID)
	if err != nil {
		return Review{}, err
	}
	// Not the seeker's booking, or no booking at all: one answer, so neither reveals the
	// other.
	if !found || booking.SeekerUserID != seekerID {
		return Review{}, ErrBookingNotFound
	}
	if !booking.Completed(s.now()) {
		return Review{}, ErrSessionNotCompleted
	}

	return s.repo.UpsertReview(ctx, Review{
		BookingID: booking.ID,
		MentorID:  booking.MentorID,
		Rating:    rating,
		Comment:   comment,
	}, seekerID)
}

// slotIsOfferable re-derives the mentor's slots around the requested start and reports
// whether that exact instant is one of them.
//
// The window is one day either side rather than the whole horizon: the answer only
// depends on the requested day's schedule and the busy time around it, and asking for a
// month of slots to check one hour is work nobody needs. The buffers reach at most a
// session's length beyond a booking, so a day is comfortably enough.
func (s *Service) slotIsOfferable(ctx context.Context, mentor Profile, start time.Time) (bool, error) {
	zone, err := time.LoadLocation(mentor.Timezone)
	if err != nil {
		// A stored zone that no longer resolves is a broken row, not a bad request: the
		// mentor has no schedule at all until it is fixed, and guessing one would offer
		// hours nobody stated.
		return false, fmt.Errorf("%w: mentor %d has timezone %q", ErrNoMentorZone, mentor.ID, mentor.Timezone)
	}

	from := start.AddDate(0, 0, -1)
	to := start.AddDate(0, 0, 1)

	rules, err := s.repo.ListAvailability(ctx, mentor.ID)
	if err != nil {
		return false, err
	}
	busy, err := s.repo.ListBusy(ctx, mentor.ID, from, to)
	if err != nil {
		return false, err
	}

	result, err := Slots(SlotRequest{
		Rules:      rules,
		MentorZone: zone,
		Params:     mentor.Session,
		Busy:       busy,
		From:       from,
		To:         to,
		Now:        s.now(),
		ViewerZone: "UTC",
	})
	if err != nil {
		return false, err
	}

	for _, slot := range result.Slots {
		if slot.Start.Equal(start) {
			return true, nil
		}
	}
	return false, nil
}

// pageSize bounds a list request the same way the directory is bounded.
func pageSize(requested int32) int32 {
	if requested <= 0 || requested > maxDirectoryPage {
		return defaultDirectoryPage
	}
	return requested
}
