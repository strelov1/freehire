package mentorship

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/availability"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// constraintNoOverlap is the EXCLUDE constraint that stops a mentor's confirmed bookings
// sharing an instant. Its violation is the lost race and is translated to
// ErrSlotUnavailable, not a 500.
const constraintNoOverlap = "mentor_bookings_no_overlap"

// availability is the user's shared calendar behind a mentor profile (see engage/availability).
func (r *QueriesRepository) availability() *availability.Store { return availability.New(r.q) }

// ListAvailability reads every availability row of the mentor's account, in both shapes.
func (r *QueriesRepository) ListAvailability(ctx context.Context, mentorID int64) ([]slots.Rule, error) {
	userID, err := r.q.GetMentorUserID(ctx, mentorID)
	if err != nil {
		return nil, err
	}
	sc, err := r.availability().Load(ctx, userID)
	return sc.Rules, err
}

// AvailabilityZone is the zone the account's saved hours are stated in, empty before any
// settings were saved (the caller then uses the profile's own zone).
func (r *QueriesRepository) AvailabilityZone(ctx context.Context, mentorID int64) (string, error) {
	userID, err := r.q.GetMentorUserID(ctx, mentorID)
	if err != nil {
		return "", err
	}
	sc, err := r.availability().Load(ctx, userID)
	return sc.Zone, err
}

// ReplaceWeeklyAvailability swaps the recurring week in one statement, leaving dated
// overrides alone, so a half-applied edit never makes removed hours bookable.
func (r *QueriesRepository) ReplaceWeeklyAvailability(ctx context.Context, mentorID int64, rules []slots.Rule) error {
	userID, err := r.q.GetMentorUserID(ctx, mentorID)
	if err != nil {
		return err
	}
	if err := r.q.EnsureUserAvailabilityZoneFromMentor(ctx, mentorID); err != nil {
		return err
	}
	return r.availability().ReplaceWeekly(ctx, userID, rules)
}

// AddAvailabilityRule adds one row, which is how a dated override is written.
func (r *QueriesRepository) AddAvailabilityRule(ctx context.Context, mentorID int64, rule slots.Rule) error {
	userID, err := r.q.GetMentorUserID(ctx, mentorID)
	if err != nil {
		return err
	}
	if err := r.q.EnsureUserAvailabilityZoneFromMentor(ctx, mentorID); err != nil {
		return err
	}
	_, err = r.availability().Add(ctx, userID, rule)
	return err
}

// DeleteAvailabilityRule removes one row of the owner's own schedule.
func (r *QueriesRepository) DeleteAvailabilityRule(ctx context.Context, ruleID, mentorID int64) error {
	userID, err := r.q.GetMentorUserID(ctx, mentorID)
	if err != nil {
		return err
	}
	ok, err := r.availability().Delete(ctx, userID, ruleID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrProfileNotFound
	}
	return nil
}

// ListBusy is the mentor's occupied time across every product.
func (r *QueriesRepository) ListBusy(ctx context.Context, mentorID int64, from, to time.Time) ([]slots.Interval, error) {
	booked, busy, err := r.listBusyByKind(ctx, mentorID, from, to)
	if err != nil {
		return nil, err
	}
	return append(booked, busy...), nil
}

// ListBusyByKind is ListBusy split into the sessions this mentor gives and everything else,
// for the mentor's own calendar breakdown.
func (r *QueriesRepository) ListBusyByKind(ctx context.Context, mentorID int64, from, to time.Time) (booked, busy []slots.Interval, err error) {
	return r.listBusyByKind(ctx, mentorID, from, to)
}

func (r *QueriesRepository) listBusyByKind(ctx context.Context, mentorID int64, from, to time.Time) (booked, busy []slots.Interval, err error) {
	userID, err := r.q.GetMentorUserID(ctx, mentorID)
	if err != nil {
		return nil, nil, err
	}
	all, err := r.availability().Busy(ctx, []int64{userID}, from, to)
	if err != nil {
		return nil, nil, err
	}
	booked, busy = []slots.Interval{}, []slots.Interval{}
	for _, b := range all[userID] {
		if b.Source == availability.SourceMentor {
			booked = append(booked, b.Interval)
		} else {
			busy = append(busy, b.Interval)
		}
	}
	return booked, busy, nil
}

// createBookingAttempts bounds the deadlock retry in CreateBooking.
const createBookingAttempts = 3

// CreateBooking writes a confirmed booking, translating the non-overlap constraint into
// the ordinary "no longer available" answer.
func (r *QueriesRepository) CreateBooking(ctx context.Context, row BookingRow) (Booking, error) {
	params := db.CreateMentorBookingParams{
		MentorID:       row.MentorID,
		SeekerUserID:   row.SeekerUserID,
		StartsAt:       pgconv.Instant(row.StartsAt),
		EndsAt:         pgconv.Instant(row.EndsAt),
		JobID:          optionalInt8(row.JobID),
		Note:           row.Note,
		SeekerTimezone: row.SeekerTimezone,
		MeetingUrl:     row.MeetingURL,
	}
	// A single-row insert can lose a true race with 40P01 rather than 23P01: two inserts
	// into the GiST exclusion index each wait on the other's uncommitted entry and Postgres
	// aborts one. The aborted statement wrote nothing, so it is retried and then meets the
	// EXCLUDE plainly.
	var created db.MentorBooking
	var err error
	for range createBookingAttempts {
		if created, err = r.q.CreateMentorBooking(ctx, params); !pgerr.IsDeadlock(err) {
			break
		}
	}
	if err != nil {
		// An EXCLUDE violation is SQLSTATE 23P01, not 23505, so IsUniqueViolation would miss
		// it. Matched by name so another exclusion constraint is never mistaken for this one.
		if name, ok := pgerr.ExclusionViolationConstraint(err); ok && name == constraintNoOverlap {
			return Booking{}, ErrSlotUnavailable
		}
		return Booking{}, err
	}

	// Re-read as the cancellation path does: the INSERT returns only this table's columns,
	// and the confirmation needs both parties' addresses and timezones. The bare row would
	// reach nobody.
	//
	// A failed re-read falls back to the inserted row: the booking is committed, and losing
	// its confirmation beats losing the hour.
	if full, found, err := r.BookingByID(ctx, uuid.UUID(created.ID.Bytes)); err == nil && found {
		return full, nil
	}
	return bookingFromRow(created), nil
}

// BookingByID reads one booking with the mentor context both parties' views need.
// Authorisation is the caller's: this answers for any id.
func (r *QueriesRepository) BookingByID(ctx context.Context, id uuid.UUID) (Booking, bool, error) {
	row, err := r.q.GetMentorBooking(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, false, nil
	}
	if err != nil {
		return Booking{}, false, err
	}
	booking := bookingFromRow(row.MentorBooking)
	booking.MentorUserID = row.MentorUserID
	booking.MentorSlug = row.MentorSlug
	booking.MentorTimezone = row.MentorTimezone
	booking.MentorHeadline = row.Headline
	booking.MentorEmail = row.MentorEmail
	booking.SeekerEmail = row.SeekerEmail
	return booking, true, nil
}

// CancelBooking applies the statement's three guards. A no-row result means one refused,
// and all answer ErrBookingNotFound so a stranger cannot tell "not yours" from "missing".
func (r *QueriesRepository) CancelBooking(ctx context.Context, id uuid.UUID, actorID int64, reason string) (Booking, error) {
	row, err := r.q.CancelMentorBooking(ctx, db.CancelMentorBookingParams{
		ID: pgtype.UUID{Bytes: id, Valid: true}, CancelledBy: actorID, CancelReason: reason,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrBookingNotFound
	}
	if err != nil {
		return Booking{}, err
	}

	// The statement returns only the booking's own columns, while the notice must address
	// both parties in both zones; the re-read supplies them.
	//
	// A failed re-read falls back to the statement's row: it beats failing a cancellation
	// that has already happened.
	if full, found, err := r.BookingByID(ctx, id); err == nil && found {
		return full, nil
	}
	return bookingFromRow(row), nil
}

// ListBookingsBySeeker is one seeker's own sessions.
func (r *QueriesRepository) ListBookingsBySeeker(ctx context.Context, seekerID int64, limit int32) ([]Booking, error) {
	rows, err := r.q.ListBookingsBySeeker(ctx, db.ListBookingsBySeekerParams{
		SeekerUserID: seekerID, RowLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Booking, 0, len(rows))
	for _, row := range rows {
		booking := bookingFromRow(row.MentorBooking)
		booking.MentorSlug = row.MentorSlug
		// The headline says who the session is with.
		booking.MentorHeadline = row.Headline
		out = append(out, booking)
	}
	return out, nil
}

// ListBookingsByMentor is one mentor's own list of who is coming.
func (r *QueriesRepository) ListBookingsByMentor(ctx context.Context, mentorID int64, limit int32) ([]Booking, error) {
	rows, err := r.q.ListBookingsByMentor(ctx, db.ListBookingsByMentorParams{
		MentorID: mentorID, RowLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Booking, 0, len(rows))
	for _, row := range rows {
		booking := bookingFromRow(row.MentorBooking)
		booking.SeekerEmail = row.SeekerEmail
		out = append(out, booking)
	}
	return out, nil
}

// UpsertReview records or replaces the seeker's review. booking_id is the primary key, so
// a second submission updates rather than duplicating.
func (r *QueriesRepository) UpsertReview(ctx context.Context, review Review, seekerID int64) (Review, error) {
	row, err := r.q.UpsertMentorReview(ctx, db.UpsertMentorReviewParams{
		BookingID:    pgtype.UUID{Bytes: review.BookingID, Valid: true},
		MentorID:     review.MentorID,
		SeekerUserID: seekerID,
		Rating:       int16(review.Rating),
		Comment:      review.Comment,
	})
	if err != nil {
		return Review{}, err
	}
	return Review{
		BookingID: uuid.UUID(row.BookingID.Bytes),
		MentorID:  row.MentorID,
		Rating:    int(row.Rating),
		Comment:   row.Comment,
	}, nil
}

// ListBookingsDueForReminder is the reminder worker's page. The query carries the
// predicates (confirmed, not started, not yet reminded at this offset).
func (r *QueriesRepository) ListBookingsDueForReminder(ctx context.Context, offset, floor time.Duration, limit int32) ([]Booking, error) {
	rows, err := r.q.ListBookingsDueForReminder(ctx, db.ListBookingsDueForReminderParams{
		OffsetMinutes: int32(offset / time.Minute),
		FloorMinutes:  int32(floor / time.Minute),
		RowLimit:      limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Booking, 0, len(rows))
	for _, row := range rows {
		booking := bookingFromRow(row.MentorBooking)
		booking.MentorUserID = row.MentorUserID
		booking.MentorSlug = row.MentorSlug
		booking.MentorTimezone = row.MentorTimezone
		booking.MentorHeadline = row.Headline
		booking.MentorEmail = row.MentorEmail
		booking.SeekerEmail = row.SeekerEmail
		// The mentor's current link, not the booking's snapshot: a reminder is read minutes
		// before the session, so a corrected link is the useful one.
		booking.MeetingURL = row.MentorMeetingUrl
		out = append(out, booking)
	}
	return out, nil
}

// ClaimReminder records one reminder as sent and reports whether this caller won it. The
// insert is ON CONFLICT DO NOTHING on a composite primary key, so two concurrent runs
// cannot both get true.
func (r *QueriesRepository) ClaimReminder(ctx context.Context, bookingID uuid.UUID, offset time.Duration) (bool, error) {
	rows, err := r.q.RecordReminderSent(ctx, db.RecordReminderSentParams{
		BookingID:     pgtype.UUID{Bytes: bookingID, Valid: true},
		OffsetMinutes: int32(offset / time.Minute),
	})
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// ReleaseReminderClaim deletes the claim after a failed delivery so the next run may take
// it again. A delete matching nothing is not an error.
func (r *QueriesRepository) ReleaseReminderClaim(ctx context.Context, bookingID uuid.UUID, offset time.Duration) error {
	_, err := r.q.DeleteReminderClaim(ctx, db.DeleteReminderClaimParams{
		BookingID:     pgtype.UUID{Bytes: bookingID, Valid: true},
		OffsetMinutes: int32(offset / time.Minute),
	})
	return err
}

// optionalInt8 turns a zero id into the NULL the column holds for "no vacancy".
func optionalInt8(id int64) pgtype.Int8 {
	if id == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}
