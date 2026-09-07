package mentorship

import (
	"context"
	"time"
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

	// createErr forces CreateProfile to fail, standing in for a constraint violation the
	// adapter has already translated into a domain error.
	createErr error

	deleted               bool
	cancelledBeforeDelete int
}

type referralKey struct {
	userID  int64
	company string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		profiles:               map[int64]Profile{},
		byUser:                 map[int64]int64{},
		approvedReferralOffers: map[referralKey]bool{},
		futureBookings:         map[int64][]Booking{},
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
		Headline:    in.Headline,
		Bio:         in.Bio,
		Topics:      in.Topics,
		Languages:   in.Languages,
		Timezone:    in.Timezone,
		Session:     in.Session,
		MeetingURL:  in.MeetingURL,
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
	p.Headline = in.Headline
	p.Bio = in.Bio
	p.Topics = in.Topics
	p.Languages = in.Languages
	p.Timezone = in.Timezone
	p.Session = in.Session
	p.MeetingURL = in.MeetingURL
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

func (r *fakeRepo) CompanyHasPublishedProfile(_ context.Context, company string) (bool, error) {
	for _, p := range r.profiles {
		if p.CompanySlug == company && p.Status == StatusApproved && !p.Paused {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeRepo) CancelFutureBookings(_ context.Context, mentorID, cancelledBy int64, reason string) ([]Booking, error) {
	bookings := r.futureBookings[mentorID]
	r.cancelledBeforeDelete += len(bookings)
	delete(r.futureBookings, mentorID)
	return bookings, nil
}

func (r *fakeRepo) DeleteProfile(_ context.Context, id, userID int64) error {
	p, ok := r.profiles[id]
	if !ok || p.UserID != userID {
		return ErrProfileNotFound
	}
	delete(r.profiles, id)
	delete(r.byUser, userID)
	r.deleted = true
	return nil
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
	err       error
	cancelled []Booking
}

func (n *fakeNotifier) BookingCancelled(_ context.Context, b Booking, _ CancelledBy, _ string) error {
	n.cancelled = append(n.cancelled, b)
	return n.err
}

func (n *fakeNotifier) BookingConfirmed(_ context.Context, _ Booking) error { return n.err }

func (n *fakeNotifier) BookingReminder(_ context.Context, _ Booking, _ time.Duration) error {
	return n.err
}
