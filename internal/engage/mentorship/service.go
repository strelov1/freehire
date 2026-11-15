package mentorship

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/cache"
)

// Directory page bounds. A request for more than the maximum is clamped silently: someone
// asking for ten thousand mentors is a crawler on a public endpoint.
const (
	defaultDirectoryPage = 24
	maxDirectoryPage     = 100
)

// The reasons this package cancels a booking on somebody's behalf. They reach the seeker,
// so they are sentences rather than codes.
const reasonMentorWithdrew = "the mentor is no longer available on freehire"

// CancelledBy names which side ended a session, for the notification and the audit.
type CancelledBy string

const (
	CancelledByMentor CancelledBy = "mentor"
	CancelledBySeeker CancelledBy = "seeker"
)

// Booking is one booked session, as this package talks about one.
//
// The id is a random UUID, as in referral (migration 0046): two accounts read a booking, so
// a countable id would make any authorisation slip enumerable.
type Booking struct {
	ID             uuid.UUID
	MentorID       int64
	MentorUserID   int64
	MentorSlug     string
	MentorHeadline string
	// MentorEmail and SeekerEmail are on the booking because every message goes to both; a lookup
	// per send would be a query per notification.
	MentorEmail    string
	SeekerUserID   int64
	SeekerEmail    string
	StartsAt       time.Time
	EndsAt         time.Time
	Status         string
	JobID          int64
	Note           string
	SeekerTimezone string
	MentorTimezone string
	MeetingURL     string
	// GoogleEventID is the calendar event MeetingURL was minted from. Empty for bookings on a
	// mentor's static link, so Cancel knows whether there is an event to delete.
	GoogleEventID string
}

// Repository is the persistence contract, in domain types. The adapter translates database
// refusals into the sentinels above: (user_id) unique violation to ErrAlreadyAMentor, slug to
// ErrSlugTaken, company foreign key to ErrCompanyNotFound, no-row status-guarded update to
// ErrProfileNotPending.
//
// Nothing here takes a "published?" flag: the publication predicate lives in the SQL, so the
// directory, the public read and the vacancy-page check cannot drift apart.
type Repository interface {
	CreateProfile(ctx context.Context, in ProfileInput) (Profile, error)
	ProfileByUser(ctx context.Context, userID int64) (Profile, bool, error)
	ProfileByID(ctx context.Context, id int64) (Profile, bool, error)
	PublishedProfileBySlug(ctx context.Context, slug string) (Profile, bool, error)
	UpdateProfile(ctx context.Context, in ProfileInput) (Profile, error)
	SetPaused(ctx context.Context, userID int64, paused bool) (Profile, error)
	DecideProfile(ctx context.Context, id, moderatorID int64, status string) (Profile, error)
	ListPendingProfiles(ctx context.Context) ([]PendingProfile, error)
	ListPublishedProfiles(ctx context.Context, f DirectoryFilter) ([]Profile, error)
	CancelFutureBookings(ctx context.Context, mentorID, cancelledBy int64, reason string) ([]Booking, error)
	// WithdrawProfile marks a profile withdrawn. It must NOT delete the row: bookings and reviews
	// cascade off it. Idempotent.
	WithdrawProfile(ctx context.Context, userID int64) error
	// ReactivateProfile moves a withdrawn profile back to pending and clears the pause switch.
	// Service.Reactivate has already checked the status, so zero rows means a genuine race.
	ReactivateProfile(ctx context.Context, userID int64) (Profile, error)

	ListAvailability(ctx context.Context, mentorID int64) ([]slots.Rule, error)
	ReplaceWeeklyAvailability(ctx context.Context, mentorID int64, rules []slots.Rule) error
	AddAvailabilityRule(ctx context.Context, mentorID int64, rule slots.Rule) error
	DeleteAvailabilityRule(ctx context.Context, ruleID, mentorID int64) error

	// ListBusy is the mentor's occupied time in a window: confirmed bookings plus synced calendar
	// busy intervals. Buffers are NOT applied here; the slot engine widens them, since they may
	// change between reads.
	ListBusy(ctx context.Context, mentorID int64, from, to time.Time) ([]slots.Interval, error)
	// ListBusyByKind is ListBusy's two sources kept apart, for the mentor's own calendar
	// breakdown.
	ListBusyByKind(ctx context.Context, mentorID int64, from, to time.Time) (booked, busy []slots.Interval, err error)

	// CreateBooking writes a confirmed booking. The adapter translates the EXCLUDE constraint
	// violation into ErrSlotUnavailable, the same sentinel a stale page gets.
	CreateBooking(ctx context.Context, row BookingRow) (Booking, error)
	BookingByID(ctx context.Context, id uuid.UUID) (Booking, bool, error)
	// CancelBooking carries its three guards in the statement: confirmed, not yet begun, and the
	// actor is one of the two parties. No row is ErrBookingNotFound, so a stranger cannot learn the
	// booking exists.
	CancelBooking(ctx context.Context, id uuid.UUID, actorID int64, reason string) (Booking, error)
	ListBookingsBySeeker(ctx context.Context, seekerID int64, limit int32) ([]Booking, error)
	ListBookingsByMentor(ctx context.Context, mentorID int64, limit int32) ([]Booking, error)

	UpsertReview(ctx context.Context, review Review, seekerID int64) (Review, error)

	// ListBookingsDueForReminder is the confirmed sessions starting between `floor` and `offset`
	// from now that have not been reminded at this offset. Sessions already begun are excluded: a
	// late reminder is worse than none. The floor stops the 24-hour reminder firing for a session
	// three hours away.
	ListBookingsDueForReminder(ctx context.Context, offset, floor time.Duration, limit int32) ([]Booking, error)
	// ClaimReminder records one reminder as sent and reports whether THIS caller won it. Claim
	// before sending, or it sends twice.
	ClaimReminder(ctx context.Context, bookingID uuid.UUID, offset time.Duration) (bool, error)
	// ReleaseReminderClaim gives a claim back after a failed delivery, so the next run
	// retries rather than skipping the reminder forever.
	ReleaseReminderClaim(ctx context.Context, bookingID uuid.UUID, offset time.Duration) error

	// GetMentorCalendarGrant reads what a mentor's Google grant holds, for the calendar write
	// CreateMeetEvent is about to attempt. found is false for no grant or any status other than
	// 'connected' (needs_reconsent included).
	GetMentorCalendarGrant(ctx context.Context, userID int64) (refreshTokenEnc string, scopes []string, found bool, err error)
	// SetBookingCalendarEvent records the Meet-carrying event a booking's calendar write
	// created, replacing the mentor's static snapshot with the link Google minted.
	SetBookingCalendarEvent(ctx context.Context, bookingID uuid.UUID, meetingURL, eventID string) error
	// MarkCalendarGrantNeedsReconsent flags a revocation-shaped calendar-write failure on the
	// status column gmailsync already uses: one grant, one health flag.
	MarkCalendarGrantNeedsReconsent(ctx context.Context, userID int64) error
}

// BookingRow is what the service asks the repository to write. The id and status are not
// supplied: the id is the database's random UUID and the status is always `confirmed`.
type BookingRow struct {
	MentorID       int64
	SeekerUserID   int64
	StartsAt       time.Time
	EndsAt         time.Time
	JobID          int64
	Note           string
	SeekerTimezone string
	MeetingURL     string
}

// Notifier delivers what a booking's two parties must be told. Every method is best-effort: a
// failure is logged and never undoes what committed, since a booked but unannounced session is
// recoverable and a half-booked one is not.
//
// These messages are TRANSACTIONAL and deliberately outside the account-level notification
// rule: a user who silenced saved-job reminders still gets the session they booked.
type Notifier interface {
	BookingConfirmed(ctx context.Context, b Booking) error
	BookingCancelled(ctx context.Context, b Booking, by CancelledBy, reason string) error
	BookingReminder(ctx context.Context, b Booking, before time.Duration) error
}

// Config tunes a Service.
type Config struct {
	// Cache stores computed slot windows. Nil disables caching; a read never fails because a
	// cache is unavailable.
	Cache cache.Cache
	// Notifier delivers confirmations, cancellations and reminders. Nil means no channel is
	// configured: the feature works and nobody is told.
	Notifier Notifier
	// Now is the clock, injectable for tests; nil means time.Now.
	Now func() time.Time
	// CalendarLinker mints the Meet-carrying calendar event a connected mentor's booking uses
	// instead of their static link. Nil disables it: every mentor behaves as not connected.
	CalendarLinker CalendarLinker
}

// Service implements the mentorship use cases over a Repository.
type Service struct {
	repo     Repository
	notifier Notifier
	cache    cache.Cache
	now      func() time.Time
	calendar CalendarLinker
}

// New builds a Service. A nil Now falls back to time.Now; a nil Notifier disables
// delivery without disabling the feature.
func New(repo Repository, cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, notifier: cfg.Notifier, cache: cfg.Cache, now: now, calendar: cfg.CalendarLinker}
}

// notifyCancelled tells each seeker their session is off, best-effort. Failures are logged per
// booking and never returned: the cancellations already committed.
func (s *Service) notifyCancelled(ctx context.Context, bookings []Booking, by CancelledBy, reason string) {
	if s.notifier == nil {
		return
	}
	for _, b := range bookings {
		if err := s.notifier.BookingCancelled(ctx, b, by, reason); err != nil {
			logDeliveryFailure("cancellation", b, err)
		}
	}
}

// logDeliveryFailure records a message that did not go out. Callers have already committed,
// so failing the operation over a briefly down mail server would be wrong.
func logDeliveryFailure(kind string, b Booking, err error) {
	log.Printf("mentorship: sending the %s for booking %s (mentor %d, seeker %d): %v",
		kind, b.ID, b.MentorID, b.SeekerUserID, err)
}
