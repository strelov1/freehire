package mentorship

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// constraintNoOverlap is the EXCLUDE constraint that guarantees a mentor's confirmed
// bookings cannot share an instant. Its violation is the LOST RACE, and it is translated
// into the same ErrSlotUnavailable a stale page gets: to a seeker the two events are
// identical, and letting a constraint violation reach the handler would answer an
// ordinary outcome with a 500.
const constraintNoOverlap = "mentor_bookings_no_overlap"

// ListAvailability reads every availability row for a mentor, in both shapes. A row the
// domain cannot represent — one carrying both a weekday and a date, which the CHECK
// forbids — is SKIPPED rather than failing the read: a single corrupt row must not make a
// mentor unbookable, and the schema is what stops one existing in the first place.
func (r *QueriesRepository) ListAvailability(ctx context.Context, mentorID int64) ([]Rule, error) {
	rows, err := r.q.ListMentorAvailability(ctx, mentorID)
	if err != nil {
		return nil, err
	}
	out := make([]Rule, 0, len(rows))
	for _, row := range rows {
		rule, ok := ruleFromRow(row)
		if !ok {
			continue
		}
		out = append(out, rule)
	}
	return out, nil
}

// ReplaceWeeklyAvailability swaps the recurring week in one transaction, leaving dated
// overrides alone. A transaction because the delete and the inserts are one edit: half of
// it applied is a mentor with an emptier week than they asked for, and the window between
// the two is a window in which they are bookable at hours they just removed.
func (r *QueriesRepository) ReplaceWeeklyAvailability(ctx context.Context, mentorID int64, rules []Rule) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.q.WithTx(tx)
	if _, err := q.DeleteMentorWeeklyAvailability(ctx, mentorID); err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.IsDated() {
			continue
		}
		if _, err := q.CreateMentorAvailabilityRule(ctx, availabilityParams(mentorID, rule)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// AddAvailabilityRule adds one row, which is how a dated override is written.
func (r *QueriesRepository) AddAvailabilityRule(ctx context.Context, mentorID int64, rule Rule) error {
	_, err := r.q.CreateMentorAvailabilityRule(ctx, availabilityParams(mentorID, rule))
	return err
}

// DeleteAvailabilityRule removes one row of the owner's own schedule.
func (r *QueriesRepository) DeleteAvailabilityRule(ctx context.Context, ruleID, mentorID int64) error {
	rows, err := r.q.DeleteMentorAvailabilityRule(ctx, db.DeleteMentorAvailabilityRuleParams{
		ID: ruleID, MentorID: mentorID,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrProfileNotFound
	}
	return nil
}

// ListBusy is both halves of the mentor's occupied time: their confirmed bookings and the
// intervals read from their calendar. The second is empty until that sync ships, and it
// is unioned here rather than in SQL so the seam stays one query per source.
func (r *QueriesRepository) ListBusy(ctx context.Context, mentorID int64, from, to time.Time) ([]Interval, error) {
	bookings, err := r.q.ListMentorBusyBookings(ctx, db.ListMentorBusyBookingsParams{
		MentorID:    mentorID,
		WindowStart: pgtype.Timestamptz{Time: from, Valid: true},
		WindowEnd:   pgtype.Timestamptz{Time: to, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	calendar, err := r.q.ListMentorBusyIntervals(ctx, db.ListMentorBusyIntervalsParams{
		MentorID:    mentorID,
		WindowStart: pgtype.Timestamptz{Time: from, Valid: true},
		WindowEnd:   pgtype.Timestamptz{Time: to, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	out := make([]Interval, 0, len(bookings)+len(calendar))
	for _, row := range bookings {
		out = append(out, Interval{Start: row.StartsAt.Time, End: row.EndsAt.Time})
	}
	for _, row := range calendar {
		out = append(out, Interval{Start: row.StartsAt.Time, End: row.EndsAt.Time})
	}
	return out, nil
}

// CreateBooking writes a confirmed booking, translating the non-overlap constraint into
// the ordinary "no longer available" answer.
func (r *QueriesRepository) CreateBooking(ctx context.Context, row BookingRow) (Booking, error) {
	created, err := r.q.CreateMentorBooking(ctx, db.CreateMentorBookingParams{
		MentorID:       row.MentorID,
		SeekerUserID:   row.SeekerUserID,
		StartsAt:       pgtype.Timestamptz{Time: row.StartsAt, Valid: true},
		EndsAt:         pgtype.Timestamptz{Time: row.EndsAt, Valid: true},
		JobID:          optionalInt8(row.JobID),
		Note:           row.Note,
		SeekerTimezone: row.SeekerTimezone,
		MeetingUrl:     row.MeetingURL,
	})
	if err != nil {
		// An EXCLUDE violation carries SQLSTATE 23P01, NOT a unique violation's 23505 —
		// reaching for IsUniqueViolation here would miss it and turn "somebody took that
		// hour" into a 500. Matched by name so a second exclusion constraint on this
		// table could never be answered with this one's message.
		if name, ok := pgerr.ExclusionViolationConstraint(err); ok && name == constraintNoOverlap {
			return Booking{}, ErrSlotUnavailable
		}
		return Booking{}, err
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

// CancelBooking applies the statement's three guards. A no-row result means one of them
// refused, and all three answer ErrBookingNotFound: a stranger must not be able to tell
// "not yours" from "does not exist", and a seeker learning their own session has already
// started is told by the page, not by a distinct error here.
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

	// The statement returns the booking's own columns and nothing else, while the
	// cancellation notice has to address BOTH parties in BOTH zones. The second read is
	// what fetches them — one query on a path that has just written, not a per-message
	// lookup.
	booking := bookingFromRow(row)
	if full, found, err := r.BookingByID(ctx, id); err == nil && found {
		booking.MentorUserID = full.MentorUserID
		booking.MentorSlug = full.MentorSlug
		booking.MentorTimezone = full.MentorTimezone
		booking.MentorHeadline = full.MentorHeadline
		booking.MentorEmail = full.MentorEmail
		booking.SeekerEmail = full.SeekerEmail
	}
	return booking, nil
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

// ListBookingsDueForReminder is the reminder worker's page. The query carries the three
// predicates that matter — confirmed, not yet started, not already reminded at this offset
// — so nothing here re-derives them.
func (r *QueriesRepository) ListBookingsDueForReminder(ctx context.Context, offset time.Duration, limit int32) ([]Booking, error) {
	rows, err := r.q.ListBookingsDueForReminder(ctx, db.ListBookingsDueForReminderParams{
		OffsetMinutes: int32(offset / time.Minute),
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
		// The mentor's CURRENT link, not the booking's snapshot. A reminder is read minutes
		// before the session, so a corrected link is the useful one — the opposite trade
		// from the invitation, which must not be rewritten after somebody has filed it.
		booking.MeetingURL = row.MentorMeetingUrl
		out = append(out, booking)
	}
	return out, nil
}

// ClaimReminder records one reminder as sent and reports whether THIS caller won it. The
// insert is ON CONFLICT DO NOTHING against a composite primary key, so two concurrent runs
// cannot both come back true — which is the entire idempotency guarantee.
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

// availabilityParams turns a domain rule into the row's two shapes: a weekly rule sets
// weekday and leaves on_date NULL, a dated one the reverse. The CHECK rejects anything
// else, and the domain type cannot build it.
func availabilityParams(mentorID int64, rule Rule) db.CreateMentorAvailabilityRuleParams {
	params := db.CreateMentorAvailabilityRuleParams{
		MentorID:  mentorID,
		StartTime: timeOfDayToPg(rule.Start()),
		EndTime:   timeOfDayToPg(rule.End()),
	}
	if rule.IsDated() {
		year, month, day := rule.Date().Parts()
		params.OnDate = pgtype.Date{
			Time:  time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
			Valid: true,
		}
		return params
	}
	params.Weekday = pgtype.Int2{Int16: int16(rule.Weekday()), Valid: true}
	return params
}

// ruleFromRow rebuilds a domain rule, reporting false for a row that is neither shape.
func ruleFromRow(row db.MentorAvailability) (Rule, bool) {
	start, startOK := timeOfDayFromPg(row.StartTime)
	end, endOK := timeOfDayFromPg(row.EndTime)
	if !startOK || !endOK {
		return Rule{}, false
	}

	if row.OnDate.Valid {
		date, err := NewDate(row.OnDate.Time.Year(), row.OnDate.Time.Month(), row.OnDate.Time.Day())
		if err != nil {
			return Rule{}, false
		}
		rule, err := NewDatedRule(date, start, end)
		return rule, err == nil
	}
	if !row.Weekday.Valid {
		return Rule{}, false
	}
	rule, err := NewWeeklyRule(time.Weekday(row.Weekday.Int16), start, end)
	return rule, err == nil
}

// timeOfDayToPg renders a wall-clock time as the microseconds-since-midnight pgtype.Time
// carries. 24:00 is a valid time in Postgres and is how a rule reaches the end of its day.
func timeOfDayToPg(t TimeOfDay) pgtype.Time {
	return pgtype.Time{Microseconds: int64(t.Minutes()) * 60 * 1_000_000, Valid: true}
}

// timeOfDayFromPg reads one back, rejecting a stored value finer than a minute rather
// than rounding it — a rounded hour is a mentor's stated hours quietly moved.
func timeOfDayFromPg(t pgtype.Time) (TimeOfDay, bool) {
	if !t.Valid {
		return TimeOfDay{}, false
	}
	const microsPerMinute = 60 * 1_000_000
	if t.Microseconds%microsPerMinute != 0 {
		return TimeOfDay{}, false
	}
	minutes := int(t.Microseconds / microsPerMinute)
	got, err := NewTimeOfDay(minutes/60, minutes%60)
	return got, err == nil
}

// optionalInt8 turns a zero id into the NULL the column holds for "no vacancy".
func optionalInt8(id int64) pgtype.Int8 {
	if id == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}
