package buddy

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/engage/slots"
)

// MockNotifier delivers what a mock produces. Every message names the other participant
// by buddy handle only and renders the time in its recipient's own zone.
type MockNotifier interface {
	// MockBooked tells both participants.
	MockBooked(ctx context.Context, m Mock) error
	// MockCancelled tells the participant who did not cause it; actorID is who did.
	MockCancelled(ctx context.Context, m Mock, actorID int64) error
	// MockReminder tells both participants the mock starts in `before`.
	MockReminder(ctx context.Context, m Mock, before time.Duration) error
}

// MockRepository is the storage the MockService needs.
type MockRepository interface {
	// MockSettings is a member's saved settings; ok is false when they never saved any.
	MockSettings(ctx context.Context, userID int64) (s MockSettings, ok bool, err error)
	SaveMockSettings(ctx context.Context, userID int64, s MockSettings) (MockSettings, error)
	// MockAvailability is every rule of a member, weekly and dated, each with its row id.
	MockAvailability(ctx context.Context, userID int64) ([]slots.Rule, error)
	// ReplaceMockWeekly swaps the weekly rules in one statement, leaving overrides alone.
	ReplaceMockWeekly(ctx context.Context, userID int64, rules []slots.Rule) error
	// AddMockRule stores one rule and returns it labelled with its row id.
	AddMockRule(ctx context.Context, userID int64, rule slots.Rule) (slots.Rule, error)
	// DeleteMockRule reports false when the rule is not userID's.
	DeleteMockRule(ctx context.Context, userID, ruleID int64) (bool, error)
	// MockBusy is the member's occupied time in a window: every active participant row of
	// theirs, as host or as guest.
	MockBusy(ctx context.Context, userID int64, from, to time.Time) ([]slots.Interval, error)
	// FutureGuestMocks counts the member's confirmed, not yet started mocks as guest.
	FutureGuestMocks(ctx context.Context, guestID int64) (int, error)
	// CreateMock writes the mock and both participant rows in one statement. It returns
	// ErrSlotUnavailable when either person is already in an overlapping mock, or when the
	// pair can no longer meet (one left, or a block landed since the service's read).
	CreateMock(ctx context.Context, in NewMock) (Mock, error)
	// MockByID reads one mock with both handles, the host's current zone and link and
	// both addresses. Authorisation is the caller's.
	MockByID(ctx context.Context, id uuid.UUID) (Mock, bool, error)
	// CancelMock cancels a confirmed, not yet started mock userID takes part in, freeing
	// both participants' time in the same statement; false when any of that does not hold.
	CancelMock(ctx context.Context, id uuid.UUID, userID int64) (bool, error)
	// ClaimCancelNotices claims every recent, not yet announced cancellation actorID caused
	// — by cancelling, leaving or blocking — and returns those mocks.
	ClaimCancelNotices(ctx context.Context, actorID int64) ([]Mock, error)
	// MockEntries is userID's mocks, newest first, with their own review and the review
	// about them only once revealed. CanCancel/CanReview are left for the service.
	MockEntries(ctx context.Context, userID int64, limit int32) ([]MockEntry, error)
	// CreateMockReview stores authorID's review of the other participant; false when they
	// already reviewed it or the mock is outside the review rules.
	CreateMockReview(ctx context.Context, id uuid.UUID, authorID int64, rating int, comment string) (bool, error)
	// MockSummary is subjectID's completed count and revealed-review aggregate.
	MockSummary(ctx context.Context, subjectID int64, recent int32) (MockSummary, error)
}

// MockService is the mock-interview use cases between buddy members: settings,
// availability, the slot listing, booking, cancellation, review and reminders. Membership,
// handles and blocks are the buddy Service's; this asks it rather than keeping a copy.
type MockService struct {
	members  *Service
	repo     MockRepository
	notifier MockNotifier
	now      func() time.Time
}

// NewMockService builds a MockService. A nil notifier sends nothing and claims nothing.
func NewMockService(members *Service, repo MockRepository, notifier MockNotifier) *MockService {
	return &MockService{members: members, repo: repo, notifier: notifier, now: time.Now}
}

// Settings is the caller's own settings; ok is false before the first save.
func (s *MockService) Settings(ctx context.Context, userID int64) (MockSettings, bool, error) {
	if err := s.members.requireActive(ctx, userID); err != nil {
		return MockSettings{}, false, err
	}
	return s.repo.MockSettings(ctx, userID)
}

// SaveSettings validates and stores the caller's settings whole.
func (s *MockService) SaveSettings(ctx context.Context, userID int64, in MockSettings) (MockSettings, error) {
	if err := s.members.requireActive(ctx, userID); err != nil {
		return MockSettings{}, err
	}
	valid, err := validateMockSettings(in)
	if err != nil {
		return MockSettings{}, err
	}
	return s.repo.SaveMockSettings(ctx, userID, valid)
}

// Availability is the caller's whole schedule, both shapes together.
func (s *MockService) Availability(ctx context.Context, userID int64) ([]slots.Rule, error) {
	if err := s.members.requireActive(ctx, userID); err != nil {
		return nil, err
	}
	return s.repo.MockAvailability(ctx, userID)
}

// ReplaceWeekly swaps the caller's recurring week; a dated rule is not part of a week.
func (s *MockService) ReplaceWeekly(ctx context.Context, userID int64, rules []slots.Rule) error {
	for _, r := range rules {
		if r.IsDated() {
			return fmt.Errorf("%w: the weekly schedule takes weekday rules", slots.ErrInvalidSpan)
		}
	}
	if err := s.members.requireActive(ctx, userID); err != nil {
		return err
	}
	return s.repo.ReplaceMockWeekly(ctx, userID, rules)
}

// AddOverride adds one dated rule: a narrowed day, an extra day, or a closure.
func (s *MockService) AddOverride(ctx context.Context, userID int64, rule slots.Rule) (slots.Rule, error) {
	if !rule.IsDated() {
		return slots.Rule{}, fmt.Errorf("%w: an override names a date", slots.ErrInvalidSpan)
	}
	if err := s.members.requireActive(ctx, userID); err != nil {
		return slots.Rule{}, err
	}
	return s.repo.AddMockRule(ctx, userID, rule)
}

// DeleteRule removes one row of the caller's own schedule; anyone else's is ErrNotFound.
func (s *MockService) DeleteRule(ctx context.Context, userID, ruleID int64) error {
	if err := s.members.requireActive(ctx, userID); err != nil {
		return err
	}
	ok, err := s.repo.DeleteMockRule(ctx, userID, ruleID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// MockSlots is what one member may book of another: the open slots in the viewer's zone
// and the host's topics. The meeting link is deliberately absent — it reaches a guest only
// through a booked mock.
type MockSlots struct {
	Slots  []slots.Interval
	Zone   string
	Topics []string
}

// OpenSlots lists hostHandle's bookable slots for an active viewer, in viewerZone (an
// unknown name answers in UTC and says so in Zone). A host with no settings or paused
// offers nothing; an unknown, departed or blocked host, and the viewer's own handle, are
// ErrNotFound.
func (s *MockService) OpenSlots(ctx context.Context, viewerID int64, hostHandle string, from, to time.Time, viewerZone string) (MockSlots, error) {
	host, err := s.members.visibleMember(ctx, viewerID, hostHandle)
	if err != nil {
		return MockSlots{}, err
	}
	settings, ok, err := s.repo.MockSettings(ctx, host.UserID)
	if err != nil {
		return MockSlots{}, err
	}
	if !ok || settings.Paused {
		// No schedule: the engine over no rules still resolves the viewer's zone, so the
		// empty answer says which zone it would have been in.
		res, err := mockSlots(nil, time.UTC, nil, from, to, s.now(), viewerZone)
		return MockSlots{Slots: res.Slots, Zone: res.Zone, Topics: []string{}}, err
	}
	res, err := s.hostSlots(ctx, host.UserID, settings, from, to, viewerZone)
	if err != nil {
		return MockSlots{}, err
	}
	return MockSlots{Slots: res.Slots, Zone: res.Zone, Topics: settings.Topics}, nil
}

// hostSlots runs the engine over a host's schedule and busy time. Busy is read a day wider
// than the window on both sides, as mentorship's slotsFor does.
func (s *MockService) hostSlots(ctx context.Context, hostID int64, settings MockSettings, from, to time.Time, viewerZone string) (slots.SlotResult, error) {
	zone, ok := ianaZone(settings.Timezone)
	if !ok {
		return slots.SlotResult{}, fmt.Errorf("%w: member %d has timezone %q", slots.ErrNoOwnerZone, hostID, settings.Timezone)
	}
	rules, err := s.repo.MockAvailability(ctx, hostID)
	if err != nil {
		return slots.SlotResult{}, err
	}
	busy, err := s.repo.MockBusy(ctx, hostID, from.AddDate(0, 0, -1), to.AddDate(0, 0, 1))
	if err != nil {
		return slots.SlotResult{}, err
	}
	return mockSlots(rules, zone, busy, from, to, s.now(), viewerZone)
}

// BookInput is a request to take one of another member's slots.
type BookInput struct {
	HostHandle  string
	GuestUserID int64
	// StartsAt is the slot's start as the guest saw it. The end is MockDuration later and
	// is never taken from the client.
	StartsAt time.Time
	Topic    string
	// GuestZone is the zone the guest booked in; their notices are rendered in it. An
	// unusable name is stored as UTC, which the notices then state.
	GuestZone string
}

// Book takes one of another member's slots. The slot is re-derived from the host's live
// schedule and busy time rather than trusted from the request; the EXCLUDE on
// buddy_mock_participants, not this re-check, is what wins a race, and losing one is the
// same ErrSlotUnavailable a stale page gets. The booked notice is best-effort.
func (s *MockService) Book(ctx context.Context, in BookInput) (Mock, error) {
	host, err := s.members.visibleMember(ctx, in.GuestUserID, in.HostHandle)
	if err != nil {
		return Mock{}, err
	}
	settings, ok, err := s.repo.MockSettings(ctx, host.UserID)
	if err != nil {
		return Mock{}, err
	}
	if !ok || settings.Paused {
		return Mock{}, ErrSlotUnavailable
	}
	if !slices.Contains(settings.Topics, in.Topic) {
		return Mock{}, ErrInvalidTopic
	}
	held, err := s.repo.FutureGuestMocks(ctx, in.GuestUserID)
	if err != nil {
		return Mock{}, err
	}
	// A count followed by an insert: concurrent bookings by one guest can slip one past the
	// bound, as with the invitation cap. The bound is a brake, the EXCLUDE is what holds.
	if held >= MaxGuestMocks {
		return Mock{}, ErrGuestMockLimit
	}
	offerable, err := s.offerable(ctx, host.UserID, settings, in.StartsAt)
	if err != nil {
		return Mock{}, err
	}
	if !offerable {
		return Mock{}, ErrSlotUnavailable
	}
	guestZone := "UTC"
	if _, ok := ianaZone(in.GuestZone); ok {
		guestZone = in.GuestZone
	}
	mock, err := s.repo.CreateMock(ctx, NewMock{
		HostUserID: host.UserID, GuestUserID: in.GuestUserID,
		StartsAt: in.StartsAt, EndsAt: in.StartsAt.Add(MockDuration),
		Topic: in.Topic, GuestTimezone: guestZone,
	})
	if err != nil {
		return Mock{}, err
	}
	if s.notifier != nil {
		if err := s.notifier.MockBooked(ctx, mock); err != nil {
			log.Printf("buddy: booked notice for mock %s: %v", mock.ID, err)
		}
	}
	return mock, nil
}

// offerable re-derives the host's slots a day either side of start and reports whether
// that exact instant is one of them.
func (s *MockService) offerable(ctx context.Context, hostID int64, settings MockSettings, start time.Time) (bool, error) {
	res, err := s.hostSlots(ctx, hostID, settings, start.AddDate(0, 0, -1), start.AddDate(0, 0, 1), "UTC")
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(res.Slots, func(iv slots.Interval) bool { return iv.Start.Equal(start) }), nil
}

// Cancel cancels one of the caller's mocks before it starts and tells the other side.
// Someone else's mock reads as ErrMockNotFound; a started or cancelled one is
// ErrMockNotCancellable.
func (s *MockService) Cancel(ctx context.Context, userID int64, id uuid.UUID) error {
	m, ok, err := s.repo.MockByID(ctx, id)
	if err != nil {
		return err
	}
	if !ok || (userID != m.HostUserID && userID != m.GuestUserID) {
		return ErrMockNotFound
	}
	if m.Status != MockConfirmed || !s.now().Before(m.StartsAt) {
		return ErrMockNotCancellable
	}
	done, err := s.repo.CancelMock(ctx, id, userID)
	if err != nil {
		return err
	}
	if !done {
		// The start or another cancellation landed between the read and the write.
		return ErrMockNotCancellable
	}
	s.AnnounceCancellations(ctx, userID)
	return nil
}

// AnnounceCancellations tells the other participant of every mock actorID's own act
// cancelled and nobody has announced yet: a participant's Cancel, the leave
// (LeaveBuddyMember cancels in its own statement) or a block (a trigger on user_blocks
// cancels inside the block's statement). The api layer calls it after a leave and after a
// block. Best-effort: a failure is logged, never returned, because the cancellation it
// reports has already happened.
func (s *MockService) AnnounceCancellations(ctx context.Context, actorID int64) {
	if s.notifier == nil {
		return
	}
	cancelled, err := s.repo.ClaimCancelNotices(ctx, actorID)
	if err != nil {
		log.Printf("buddy: claiming cancellation notices of user %d: %v", actorID, err)
	}
	for _, m := range cancelled {
		if err := s.notifier.MockCancelled(ctx, m, actorID); err != nil {
			log.Printf("buddy: cancellation notice for mock %s: %v", m.ID, err)
		}
	}
}

// List is the caller's own mocks.
func (s *MockService) List(ctx context.Context, userID int64) (MockList, error) {
	if err := s.members.requireActive(ctx, userID); err != nil {
		return MockList{}, err
	}
	entries, err := s.repo.MockEntries(ctx, userID, mockListLimit)
	if err != nil {
		return MockList{}, err
	}
	now := s.now()
	list := MockList{Upcoming: []MockEntry{}, Past: []MockEntry{}}
	for _, e := range entries {
		e.settle(now)
		if e.Status == MockConfirmed && now.Before(e.EndsAt) {
			list.Upcoming = append(list.Upcoming, e)
		} else {
			list.Past = append(list.Past, e)
		}
	}
	// The read is newest first; what is coming reads soonest first.
	slices.Reverse(list.Upcoming)
	return list, nil
}

// Review rates the other participant of one of the caller's confirmed mocks, once, between
// its end and ReviewWindow after it. The rating is hidden from its subject and their page
// until they review too or the window closes — computed by every read, never stored.
func (s *MockService) Review(ctx context.Context, userID int64, id uuid.UUID, rating int, comment string) error {
	comment = strings.TrimSpace(comment)
	if rating < 1 || rating > 5 || utf8.RuneCountInString(comment) > MaxReviewCommentRunes {
		return fmt.Errorf("%w: a rating from 1 to 5 and at most %d characters", ErrInvalidReview, MaxReviewCommentRunes)
	}
	if err := s.members.requireActive(ctx, userID); err != nil {
		return err
	}
	m, ok, err := s.repo.MockByID(ctx, id)
	if err != nil {
		return err
	}
	if !ok || (userID != m.HostUserID && userID != m.GuestUserID) {
		return ErrMockNotFound
	}
	now := s.now()
	switch {
	case m.Status != MockConfirmed:
		return fmt.Errorf("%w: the mock interview was cancelled", ErrInvalidReview)
	case now.Before(m.EndsAt):
		return fmt.Errorf("%w: the mock interview has not ended", ErrInvalidReview)
	case !now.Before(m.EndsAt.Add(ReviewWindow)):
		return fmt.Errorf("%w: the review window has closed", ErrInvalidReview)
	}
	created, err := s.repo.CreateMockReview(ctx, id, userID, rating, comment)
	if err != nil {
		return err
	}
	if !created {
		return ErrAlreadyReviewed
	}
	return nil
}

// MemberSummary is what handle's member page shows another active member: completed mocks
// and their revealed reviews. Who reviewed is not part of it.
func (s *MockService) MemberSummary(ctx context.Context, viewerID int64, handle string) (MockSummary, error) {
	target, err := s.members.visibleMember(ctx, viewerID, handle)
	if err != nil {
		return MockSummary{}, err
	}
	return s.repo.MockSummary(ctx, target.UserID, recentReviews)
}
