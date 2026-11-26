package mentorship

import (
	"context"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/engage/slots"
)

// There is deliberately no uncached MentorSlots beside CachedMentorSlots: a second public
// read would be a second place to forget that a paused mentor answers as nonexistent.

// slotsFor assembles the engine's inputs and runs it.
func (s *Service) slotsFor(ctx context.Context, mentor Profile, from, to time.Time, viewerZone string) (slots.SlotResult, error) {
	zone, err := s.zoneOf(ctx, mentor)
	if err != nil {
		return slots.SlotResult{}, err
	}

	rules, err := s.repo.ListAvailability(ctx, mentor.ID)
	if err != nil {
		return slots.SlotResult{}, err
	}
	// The busy read is widened a day either side: a booking starting before the window can
	// still block an hour inside it once buffers apply.
	busy, err := s.repo.ListBusy(ctx, mentor.ID, from.AddDate(0, 0, -1), to.AddDate(0, 0, 1))
	if err != nil {
		return slots.SlotResult{}, err
	}

	return slots.Slots(slots.SlotRequest{
		Rules:      rules,
		OwnerZone:  zone,
		Params:     mentor.Session,
		Busy:       busy,
		From:       from,
		To:         to,
		Now:        s.now(),
		ViewerZone: viewerZone,
	})
}

// MyAvailability is the owner's whole schedule.
func (s *Service) MyAvailability(ctx context.Context, userID int64) ([]slots.Rule, error) {
	profile, err := s.ownProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListAvailability(ctx, profile.ID)
}

// ReplaceWeeklySchedule swaps the recurring week, leaving dated overrides alone.
func (s *Service) ReplaceWeeklySchedule(ctx context.Context, userID int64, rules []slots.Rule) error {
	profile, err := s.ownProfile(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.ReplaceWeeklyAvailability(ctx, profile.ID, rules)
}

// AddOverride adds one dated rule.
func (s *Service) AddOverride(ctx context.Context, userID int64, rule slots.Rule) error {
	if !rule.IsDated() {
		return fmt.Errorf("%w: an override names a date", slots.ErrInvalidSpan)
	}
	profile, err := s.ownProfile(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.AddAvailabilityRule(ctx, profile.ID, rule)
}

// MyCalendar is the owner's own resolved calendar for one month: their availability,
// bookings and synced-calendar busy time, partitioned into booked/busy/free/closed the
// same way slotsFor computes what a seeker may book.
func (s *Service) MyCalendar(ctx context.Context, userID int64, year int, month time.Month) (slots.CalendarResult, error) {
	profile, err := s.ownProfile(ctx, userID)
	if err != nil {
		return slots.CalendarResult{}, err
	}
	zone, err := s.zoneOf(ctx, profile)
	if err != nil {
		return slots.CalendarResult{}, err
	}

	from := time.Date(year, month, 1, 0, 0, 0, 0, zone)
	to := from.AddDate(0, 1, 0)

	rules, err := s.repo.ListAvailability(ctx, profile.ID)
	if err != nil {
		return slots.CalendarResult{}, err
	}
	// Widened a day either side, as in slotsFor.
	booked, busy, err := s.repo.ListBusyByKind(ctx, profile.ID, from.AddDate(0, 0, -1), to.AddDate(0, 0, 1))
	if err != nil {
		return slots.CalendarResult{}, err
	}

	return slots.Calendar(slots.CalendarRequest{
		Rules:     rules,
		OwnerZone: zone,
		Params:    profile.Session,
		Booked:    booked,
		Busy:      busy,
		From:      from,
		To:        to,
		Now:       s.now(),
	})
}

// DeleteAvailabilityRule removes one row of the caller's own schedule.
func (s *Service) DeleteAvailabilityRule(ctx context.Context, userID, ruleID int64) error {
	profile, err := s.ownProfile(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.DeleteAvailabilityRule(ctx, ruleID, profile.ID)
}

// ownProfile is the caller's own mentor profile, or ErrProfileNotFound. Every cabinet
// operation starts here so "no profile" and "no such account" stay one answer.
func (s *Service) ownProfile(ctx context.Context, userID int64) (Profile, error) {
	profile, found, err := s.repo.ProfileByUser(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	if !found {
		return Profile{}, ErrProfileNotFound
	}
	return profile, nil
}

// zoneOf is the zone the mentor's hours are stated in: the account's shared one once saved,
// else the profile's own.
func (s *Service) zoneOf(ctx context.Context, mentor Profile) (*time.Location, error) {
	name, err := s.repo.AvailabilityZone(ctx, mentor.ID)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = mentor.Timezone
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		// A stored zone that no longer resolves is a broken row, not a bad request; guessing
		// one would offer hours nobody stated.
		return nil, fmt.Errorf("%w: mentor %d has timezone %q", slots.ErrNoOwnerZone, mentor.ID, name)
	}
	return zone, nil
}
