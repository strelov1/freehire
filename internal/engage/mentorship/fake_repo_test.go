package mentorship

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// fakeRepo is an in-memory Repository. It exists so the service's rules can be tested
// without Postgres — the rules that live in SQL (the non-overlap constraint, the
// cancellation guards, the publication predicate) are tested against a real database in
// internal/platform/db, which is the only place they can be.
type fakeRepo struct {
	profiles map[int64]Profile // by mentor id
	byUser   map[int64]int64   // user id → mentor id
	nextID   int64

	approvedReferralOffers map[referralKey]bool
	futureBookings         map[int64][]Booking

	availability  map[int64][]Rule
	bookings      map[uuid.UUID]Booking
	reviews       map[uuid.UUID]Review
	remindersSent map[reminderKey]bool
	claimed       int

	// claimAlwaysLost stands in for a concurrent run winning every claim.
	claimAlwaysLost bool
	listDueErr      error
	releaseErr      error

	// createErr forces CreateProfile to fail, standing in for a constraint violation the
	// adapter has already translated into a domain error.
	createErr error
	// createBookingErr stands in for the EXCLUDE constraint firing between the engine
	// offering a slot and the insert reaching the database — the lost race.
	createBookingErr error

	withdrawn             bool
	cancelledBeforeDelete int

	// calendarGrants stands in for gmail_connections, keyed by the mentor's user id.
	calendarGrants          map[int64]fakeCalendarGrant
	needsReconsent          map[int64]bool
	setBookingCalendarEvent map[uuid.UUID][2]string // bookingID -> [meetingURL, eventID]
	setBookingCalendarEvErr error
}

type fakeCalendarGrant struct {
	refreshTokenEnc string
	scopes          []string
}

type referralKey struct {
	userID  int64
	company string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		profiles:                map[int64]Profile{},
		byUser:                  map[int64]int64{},
		approvedReferralOffers:  map[referralKey]bool{},
		futureBookings:          map[int64][]Booking{},
		availability:            map[int64][]Rule{},
		bookings:                map[uuid.UUID]Booking{},
		reviews:                 map[uuid.UUID]Review{},
		remindersSent:           map[reminderKey]bool{},
		calendarGrants:          map[int64]fakeCalendarGrant{},
		needsReconsent:          map[int64]bool{},
		setBookingCalendarEvent: map[uuid.UUID][2]string{},
	}
}

func (r *fakeRepo) CreateProfile(_ context.Context, in ProfileInput) (Profile, error) {
	if r.createErr != nil {
		return Profile{}, r.createErr
	}
	if _, taken := r.byUser[in.UserID]; taken {
		return Profile{}, ErrAlreadyAMentor
	}
	r.nextID++
	p := Profile{
		ID:          r.nextID,
		UserID:      in.UserID,
		CompanySlug: in.CompanySlug,
		Slug:        in.Slug,
		DisplayName: in.DisplayName,
		Headline:    in.Headline,
		Bio:         in.Bio,
		Topics:      in.Topics,
		Languages:   in.Languages,
		Timezone:    in.Timezone,
		Session:     in.Session,
		MeetingURL:  in.MeetingURL,
		ShowPhoto:   in.ShowPhoto,
		Status:      StatusPending,
	}
	r.profiles[p.ID] = p
	r.byUser[in.UserID] = p.ID
	return p, nil
}

func (r *fakeRepo) ProfileByUser(_ context.Context, userID int64) (Profile, bool, error) {
	id, ok := r.byUser[userID]
	if !ok {
		return Profile{}, false, nil
	}
	return r.profiles[id], true, nil
}

func (r *fakeRepo) ProfileByID(_ context.Context, id int64) (Profile, bool, error) {
	p, ok := r.profiles[id]
	return p, ok, nil
}

func (r *fakeRepo) PublishedProfileBySlug(_ context.Context, slug string) (Profile, bool, error) {
	for _, p := range r.profiles {
		if p.Slug == slug && p.Status == StatusApproved && !p.Paused {
			return p, true, nil
		}
	}
	return Profile{}, false, nil
}

func (r *fakeRepo) UpdateProfile(_ context.Context, in ProfileInput) (Profile, error) {
	id, ok := r.byUser[in.UserID]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	p := r.profiles[id]
	// Mirrors the SQL: the slug and the company are not in the UPDATE at all.
	p.DisplayName = in.DisplayName
	p.Headline = in.Headline
	p.Bio = in.Bio
	p.Topics = in.Topics
	p.Languages = in.Languages
	p.Timezone = in.Timezone
	p.Session = in.Session
	p.MeetingURL = in.MeetingURL
	p.ShowPhoto = in.ShowPhoto
	r.profiles[id] = p
	return p, nil
}

func (r *fakeRepo) SetPaused(_ context.Context, userID int64, paused bool) (Profile, error) {
	id, ok := r.byUser[userID]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	p := r.profiles[id]
	p.Paused = paused
	r.profiles[id] = p
	return p, nil
}

func (r *fakeRepo) DecideProfile(_ context.Context, id, moderatorID int64, status string) (Profile, error) {
	p, ok := r.profiles[id]
	if !ok || p.Status != StatusPending {
		return Profile{}, ErrProfileNotPending
	}
	p.Status = status
	p.DecidedBy = moderatorID
	r.profiles[id] = p
	return p, nil
}

func (r *fakeRepo) ListPendingProfiles(_ context.Context) ([]PendingProfile, error) {
	var out []PendingProfile
	for _, p := range r.profiles {
		if p.Status != StatusPending {
			continue
		}
		out = append(out, PendingProfile{
			Profile:                  p,
			HasApprovedReferralOffer: r.approvedReferralOffers[referralKey{p.UserID, p.CompanySlug}],
		})
	}
	return out, nil
}

func (r *fakeRepo) ListPublishedProfiles(_ context.Context, f DirectoryFilter) ([]Profile, error) {
	var out []Profile
	for _, p := range r.profiles {
		if p.Status != StatusApproved || p.Paused {
			continue
		}
		if f.CompanySlug != "" && p.CompanySlug != f.CompanySlug {
			continue
		}
		if f.Topic != "" && !contains(p.Topics, f.Topic) {
			continue
		}
		if f.Language != "" && !contains(p.Languages, f.Language) {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *fakeRepo) CancelFutureBookings(_ context.Context, mentorID, cancelledBy int64, reason string) ([]Booking, error) {
	bookings := r.futureBookings[mentorID]
	r.cancelledBeforeDelete += len(bookings)
	delete(r.futureBookings, mentorID)
	return bookings, nil
}

// WithdrawProfile marks rather than deletes, exactly as the SQL does — the bookings and
// reviews map below survive it, which is the behaviour under test.
func (r *fakeRepo) WithdrawProfile(_ context.Context, userID int64) error {
	id, ok := r.byUser[userID]
	if !ok {
		return ErrProfileNotFound
	}
	p := r.profiles[id]
	if p.Status == StatusWithdrawn {
		return ErrProfileNotFound
	}
	p.Status = StatusWithdrawn
	p.Paused = true
	r.profiles[id] = p
	r.withdrawn = true
	return nil
}

func (r *fakeRepo) ListAvailability(_ context.Context, mentorID int64) ([]Rule, error) {
	return r.availability[mentorID], nil
}

func (r *fakeRepo) ReplaceWeeklyAvailability(_ context.Context, mentorID int64, rules []Rule) error {
	var kept []Rule
	for _, rule := range r.availability[mentorID] {
		if rule.IsDated() {
			kept = append(kept, rule)
		}
	}
	r.availability[mentorID] = append(kept, rules...)
	return nil
}

func (r *fakeRepo) AddAvailabilityRule(_ context.Context, mentorID int64, rule Rule) error {
	r.availability[mentorID] = append(r.availability[mentorID], rule)
	return nil
}

func (r *fakeRepo) DeleteAvailabilityRule(_ context.Context, _, _ int64) error { return nil }

// ListBusy mirrors what the query does: this mentor's CONFIRMED bookings overlapping the
// window, on half-open bounds, with no buffers applied.
func (r *fakeRepo) ListBusy(_ context.Context, mentorID int64, from, to time.Time) ([]Interval, error) {
	var out []Interval
	for _, b := range r.bookings {
		if b.MentorID != mentorID || b.Status != BookingConfirmed {
			continue
		}
		if b.StartsAt.Before(to) && b.EndsAt.After(from) {
			out = append(out, Interval{Start: b.StartsAt, End: b.EndsAt})
		}
	}
	return out, nil
}

func (r *fakeRepo) CreateBooking(_ context.Context, row BookingRow) (Booking, error) {
	if r.createBookingErr != nil {
		return Booking{}, r.createBookingErr
	}
	// Stands in for the EXCLUDE constraint: a confirmed booking of this mentor cannot
	// overlap another.
	candidate := Interval{Start: row.StartsAt, End: row.EndsAt}
	for _, b := range r.bookings {
		if b.MentorID != row.MentorID || b.Status != BookingConfirmed {
			continue
		}
		if candidate.Overlaps(Interval{Start: b.StartsAt, End: b.EndsAt}) {
			return Booking{}, ErrSlotUnavailable
		}
	}

	// The mentor's own columns are filled here because the real adapter fills them too —
	// by RE-READING after the insert, since the INSERT returns mentor_bookings' columns
	// only. That difference once cost every booking its confirmation and no unit test
	// noticed, because this fake filled them and the adapter did not. What holds the two
	// together now is TestAWrittenBookingCarriesBothPartiesContactDetails, against a real
	// Postgres; this fake describes the contract, it cannot prove it.
	mentor := r.profiles[row.MentorID]
	booking := Booking{
		ID:             uuid.New(),
		MentorID:       row.MentorID,
		MentorUserID:   mentor.UserID,
		MentorSlug:     mentor.Slug,
		MentorHeadline: mentor.Headline,
		MentorEmail:    "mentor@fake.test",
		SeekerEmail:    "seeker@fake.test",
		SeekerUserID:   row.SeekerUserID,
		StartsAt:       row.StartsAt,
		EndsAt:         row.EndsAt,
		Status:         BookingConfirmed,
		JobID:          row.JobID,
		Note:           row.Note,
		SeekerTimezone: row.SeekerTimezone,
		MentorTimezone: mentor.Timezone,
		MeetingURL:     row.MeetingURL,
	}
	r.bookings[booking.ID] = booking
	return booking, nil
}

func (r *fakeRepo) BookingByID(_ context.Context, id uuid.UUID) (Booking, bool, error) {
	b, ok := r.bookings[id]
	return b, ok, nil
}

// CancelBooking mirrors the statement's three guards, including that a stranger gets the
// same answer as a booking that does not exist.
func (r *fakeRepo) CancelBooking(_ context.Context, id uuid.UUID, actorID int64, reason string) (Booking, error) {
	b, ok := r.bookings[id]
	if !ok {
		return Booking{}, ErrBookingNotFound
	}
	if actorID != b.SeekerUserID && actorID != b.MentorUserID {
		return Booking{}, ErrBookingNotFound
	}
	if b.Status != BookingConfirmed {
		return Booking{}, ErrBookingNotCancellable
	}
	b.Status = BookingCancelled
	_ = reason
	r.bookings[id] = b
	return b, nil
}

func (r *fakeRepo) ListBookingsBySeeker(_ context.Context, seekerID int64, _ int32) ([]Booking, error) {
	var out []Booking
	for _, b := range r.bookings {
		if b.SeekerUserID == seekerID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (r *fakeRepo) ListBookingsByMentor(_ context.Context, mentorID int64, _ int32) ([]Booking, error) {
	var out []Booking
	for _, b := range r.bookings {
		if b.MentorID == mentorID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (r *fakeRepo) UpsertReview(_ context.Context, review Review, _ int64) (Review, error) {
	r.reviews[review.BookingID] = review
	return review, nil
}

// ListBookingsDueForReminder mirrors the query's three predicates: confirmed, not yet
// started, and within the offset. The "not already reminded" one is the claim's job.
func (r *fakeRepo) ListBookingsDueForReminder(_ context.Context, offset, floor time.Duration, _ int32) ([]Booking, error) {
	if r.listDueErr != nil {
		return nil, r.listDueErr
	}
	var out []Booking
	for _, b := range r.bookings {
		if b.Status != BookingConfirmed || !b.StartsAt.After(reminderNow) {
			continue
		}
		if b.StartsAt.After(reminderNow.Add(offset)) {
			continue
		}
		// The floor, mirroring the query: this offset's window starts where the next one
		// down ends.
		if !b.StartsAt.After(reminderNow.Add(floor)) {
			continue
		}
		if r.remindersSent[reminderKey{b.ID, offset}] {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *fakeRepo) ClaimReminder(_ context.Context, bookingID uuid.UUID, offset time.Duration) (bool, error) {
	if r.claimAlwaysLost {
		return false, nil
	}
	key := reminderKey{bookingID, offset}
	if r.remindersSent[key] {
		return false, nil
	}
	r.remindersSent[key] = true
	r.claimed++
	return true, nil
}

func (r *fakeRepo) ReleaseReminderClaim(_ context.Context, bookingID uuid.UUID, offset time.Duration) error {
	if r.releaseErr != nil {
		return r.releaseErr
	}
	delete(r.remindersSent, reminderKey{bookingID, offset})
	r.claimed--
	return nil
}

func (r *fakeRepo) GetMentorCalendarGrant(_ context.Context, userID int64) (string, []string, bool, error) {
	g, ok := r.calendarGrants[userID]
	if !ok {
		return "", nil, false, nil
	}
	return g.refreshTokenEnc, g.scopes, true, nil
}

func (r *fakeRepo) SetBookingCalendarEvent(_ context.Context, bookingID uuid.UUID, meetingURL, eventID string) error {
	if r.setBookingCalendarEvErr != nil {
		return r.setBookingCalendarEvErr
	}
	r.setBookingCalendarEvent[bookingID] = [2]string{meetingURL, eventID}
	b := r.bookings[bookingID]
	b.MeetingURL = meetingURL
	b.GoogleEventID = eventID
	r.bookings[bookingID] = b
	return nil
}

func (r *fakeRepo) MarkCalendarGrantNeedsReconsent(_ context.Context, userID int64) error {
	r.needsReconsent[userID] = true
	return nil
}

type reminderKey struct {
	booking uuid.UUID
	offset  time.Duration
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// fakeNotifier records what would have been delivered, and can be made to fail.
type fakeNotifier struct {
	err         error
	confirmed   []Booking
	cancelled   []Booking
	cancelledBy []CancelledBy
	reminders   []time.Duration
}

func (n *fakeNotifier) BookingCancelled(_ context.Context, b Booking, by CancelledBy, _ string) error {
	n.cancelled = append(n.cancelled, b)
	n.cancelledBy = append(n.cancelledBy, by)
	return n.err
}

func (n *fakeNotifier) BookingConfirmed(_ context.Context, b Booking) error {
	n.confirmed = append(n.confirmed, b)
	return n.err
}

func (n *fakeNotifier) BookingReminder(_ context.Context, _ Booking, before time.Duration) error {
	if n.err != nil {
		return n.err
	}
	n.reminders = append(n.reminders, before)
	return nil
}

// fakeCalendarLinker stands in for CalendarLinker, so Book()/Cancel() are tested without
// Google. deletedEventIDs records what Cancel() asked it to remove.
type fakeCalendarLinker struct {
	err            error
	eventID        string
	meetLink       string
	deletedEventID string
	deleteErr      error
	calls          int
}

func (l *fakeCalendarLinker) CreateMeetEvent(_ context.Context, _ int64, _ MeetEventInput) (string, string, error) {
	l.calls++
	if l.err != nil {
		return "", "", l.err
	}
	return l.eventID, l.meetLink, nil
}

func (l *fakeCalendarLinker) DeleteMeetEvent(_ context.Context, _ int64, eventID string) error {
	l.deletedEventID = eventID
	return l.deleteErr
}
