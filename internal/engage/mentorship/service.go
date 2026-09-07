package mentorship

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
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
	CompanyHasPublishedProfile(ctx context.Context, companySlug string) (bool, error)
	CancelFutureBookings(ctx context.Context, mentorID, cancelledBy int64, reason string) ([]Booking, error)
	DeleteProfile(ctx context.Context, id, userID int64) error
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
	now      func() time.Time
}

// New builds a Service. A nil Now falls back to time.Now; a nil Notifier disables
// delivery without disabling the feature.
func New(repo Repository, cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, notifier: cfg.Notifier, now: now}
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
			log.Printf("mentorship: notifying seeker %d of cancelled booking %s: %v",
				b.SeekerUserID, b.ID, err)
		}
	}
}
