package mentorship

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/platform/cache"
)

// Directory page bounds. The default is what an unbounded request gets; the maximum is
// what a request asking for more gets, silently — a caller asking for ten thousand
// mentors is a crawler, and this endpoint is public on a host where most traffic is.
const (
	defaultDirectoryPage = 24
	maxDirectoryPage     = 100
)

// The reasons this package cancels a booking on somebody's behalf. They reach the seeker,
// so they are sentences rather than codes.
const reasonMentorWithdrew = "the mentor is no longer available on freehire"

// CancelledBy names which side ended a session. It exists as a type because the
// notification and the audit both need to say WHO, and a bare boolean at two call sites
// is how that ends up backwards at one of them.
type CancelledBy string

const (
	CancelledByMentor CancelledBy = "mentor"
	CancelledBySeeker CancelledBy = "seeker"
)

// Booking is one booked session, as this package talks about one.
//
// The id is a random UUID rather than a counter, and for the reason referral's ids are
// (migration 0046): a booking is read by two different accounts, so a countable id would
// make any single authorisation slip enumerable.
type Booking struct {
	ID             uuid.UUID
	MentorID       int64
	MentorUserID   int64
	MentorSlug     string
	MentorHeadline string
	// MentorEmail and SeekerEmail are how a notification reaches each party. They are on
	// the booking rather than fetched per message because every message goes to BOTH,
	// and looking one up per send is a query per notification.
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
}

// Repository is the persistence contract, in domain types. The adapter is responsible for
// translating what the database refuses into the sentinels above: the (user_id) unique
// violation into ErrAlreadyAMentor, the slug unique violation into ErrSlugTaken, the
// company foreign key into ErrCompanyNotFound, and a no-row status-guarded update into
// ErrProfileNotPending — the same division of labour internal/engage/referral uses.
//
// Nothing here takes a "published?" flag: the publication predicate lives in the SQL, so
// the directory, the public read and the vacancy-page check cannot drift apart.
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
	// WithdrawProfile marks a profile withdrawn. It must NOT delete the row — bookings
	// and reviews cascade off it, and the past is history both parties keep. It is
	// idempotent: withdrawing an already-withdrawn profile succeeds.
	WithdrawProfile(ctx context.Context, userID int64) error
	// ReactivateProfile moves a withdrawn profile back to pending and clears the pause
	// switch. The caller (Service.Reactivate) has already confirmed the profile is
	// withdrawn, so a zero-row result here means only a genuine race.
	ReactivateProfile(ctx context.Context, userID int64) (Profile, error)

	ListAvailability(ctx context.Context, mentorID int64) ([]Rule, error)
	ReplaceWeeklyAvailability(ctx context.Context, mentorID int64, rules []Rule) error
	AddAvailabilityRule(ctx context.Context, mentorID int64, rule Rule) error
	DeleteAvailabilityRule(ctx context.Context, ruleID, mentorID int64) error

	// ListBusy is the mentor's occupied time in a window: their confirmed bookings and,
	// once that sync exists, their calendar's free/busy intervals. Buffers are NOT applied
	// here — the slot engine widens these, because the buffers belong to the mentor and
	// may change between two reads of the same booking.
	ListBusy(ctx context.Context, mentorID int64, from, to time.Time) ([]Interval, error)

	// CreateBooking writes a confirmed booking. The adapter translates the EXCLUDE
	// constraint's violation into ErrSlotUnavailable — the same sentinel a stale page
	// gets, because to a seeker a lost race and a stale tab are one event.
	CreateBooking(ctx context.Context, row BookingRow) (Booking, error)
	BookingByID(ctx context.Context, id uuid.UUID) (Booking, bool, error)
	// CancelBooking carries its three guards in the statement: confirmed, not yet begun,
	// and the actor is one of the two parties. A no-row result is ErrBookingNotFound, so a
	// stranger cannot learn that the booking exists.
	CancelBooking(ctx context.Context, id uuid.UUID, actorID int64, reason string) (Booking, error)
	ListBookingsBySeeker(ctx context.Context, seekerID int64, limit int32) ([]Booking, error)
	ListBookingsByMentor(ctx context.Context, mentorID int64, limit int32) ([]Booking, error)

	UpsertReview(ctx context.Context, review Review, seekerID int64) (Review, error)

	// ListBookingsDueForReminder is the confirmed sessions starting between `floor` and
	// `offset` from now that have not been reminded at this offset. Sessions already begun
	// are excluded by the query: a reminder arriving after its session is worse than none.
	// The floor is what stops the 24-hour reminder firing for a session three hours away.
	ListBookingsDueForReminder(ctx context.Context, offset, floor time.Duration, limit int32) ([]Booking, error)
	// ClaimReminder records one reminder as sent and reports whether THIS caller won it.
	// It is the claim, not the record — a caller that sends before checking sends twice.
	ClaimReminder(ctx context.Context, bookingID uuid.UUID, offset time.Duration) (bool, error)
	// ReleaseReminderClaim gives a claim back after a failed delivery, so the next run
	// retries rather than skipping the reminder forever.
	ReleaseReminderClaim(ctx context.Context, bookingID uuid.UUID, offset time.Duration) error
}

// BookingRow is what the service asks the repository to write. It is separate from
// Booking because the service supplies neither the id nor the status: the first is the
// database's random UUID and the second is always `confirmed` at creation.
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

// Notifier delivers what a booking's two parties must be told. Every method is
// best-effort: a delivery failure is logged and never undoes what has already committed,
// because a session that is booked but unannounced is recoverable and one that is
// half-booked is not.
//
// These messages are TRANSACTIONAL and deliberately outside the account-level
// notification rule — see the notification-settings delta. A user who silenced saved-job
// reminders still turns up to the session they booked, or a mentor holds an hour for
// nobody.
type Notifier interface {
	BookingConfirmed(ctx context.Context, b Booking) error
	BookingCancelled(ctx context.Context, b Booking, by CancelledBy, reason string) error
	BookingReminder(ctx context.Context, b Booking, before time.Duration) error
}

// Config tunes a Service.
type Config struct {
	// Cache stores computed slot windows. Nil disables caching without disabling the
	// endpoint — a read never fails because a cache is unavailable.
	Cache cache.Cache
	// Notifier delivers confirmations, cancellations and reminders. Nil is a deployment
	// with no channel configured — the feature works and nobody is told, which is how it
	// ships before the mail path is wired and how it is rolled back.
	Notifier Notifier
	// Now is the clock, injectable for tests; nil → time.Now.
	Now func() time.Time
}

// Service implements the mentorship use cases over a Repository.
type Service struct {
	repo     Repository
	notifier Notifier
	cache    cache.Cache
	now      func() time.Time
}

// New builds a Service. A nil Now falls back to time.Now; a nil Notifier disables
// delivery without disabling the feature.
func New(repo Repository, cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, notifier: cfg.Notifier, cache: cfg.Cache, now: now}
}

// notifyCancelled tells each seeker their session is off, best-effort. Failures are
// logged per booking and never returned: the cancellations have already committed, so
// there is nothing a caller could usefully do with the error except fail an operation
// that already succeeded.
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

// logDeliveryFailure records a message that did not go out. Every caller of it has
// already committed something, so this is the whole response: the alternative is failing
// an operation that succeeded because a mail server was briefly down.
func logDeliveryFailure(kind string, b Booking, err error) {
	log.Printf("mentorship: sending the %s for booking %s (mentor %d, seeker %d): %v",
		kind, b.ID, b.MentorID, b.SeekerUserID, err)
}
