package buddy

import (
	"context"
	"fmt"
	"time"

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
