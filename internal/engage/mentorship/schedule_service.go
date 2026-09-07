package mentorship

import (
	"context"
	"fmt"
	"time"
)

// MentorSlots is the public slot read: a published mentor's offerable hours in a window,
// expressed in the viewer's zone.
//
// It refuses an unpublished mentor the same way every other public read does — as though
// they do not exist — so a paused mentor's calendar cannot be walked by anybody who
// remembers their address.
func (s *Service) MentorSlots(ctx context.Context, slug string, from, to time.Time, viewerZone string) (SlotResult, error) {
	mentor, err := s.PublicProfile(ctx, slug)
	if err != nil {
		return SlotResult{}, err
	}
	return s.slotsFor(ctx, mentor, from, to, viewerZone)
}

// slotsFor assembles the engine's inputs and runs it.
func (s *Service) slotsFor(ctx context.Context, mentor Profile, from, to time.Time, viewerZone string) (SlotResult, error) {
	zone, err := time.LoadLocation(mentor.Timezone)
	if err != nil {
		return SlotResult{}, fmt.Errorf("%w: mentor %d has timezone %q", ErrNoMentorZone, mentor.ID, mentor.Timezone)
	}

	rules, err := s.repo.ListAvailability(ctx, mentor.ID)
	if err != nil {
		return SlotResult{}, err
	}
	// The busy read is widened by a day either side of the window: a booking that STARTS
	// before it can still block an hour inside it once the buffers are applied.
	busy, err := s.repo.ListBusy(ctx, mentor.ID, from.AddDate(0, 0, -1), to.AddDate(0, 0, 1))
	if err != nil {
		return SlotResult{}, err
	}

	return Slots(SlotRequest{
		Rules:      rules,
		MentorZone: zone,
		Params:     mentor.Session,
		Busy:       busy,
		From:       from,
		To:         to,
		Now:        s.now(),
		ViewerZone: viewerZone,
	})
}

// MyAvailability is the owner's whole schedule.
func (s *Service) MyAvailability(ctx context.Context, userID int64) ([]Rule, error) {
	profile, found, err := s.repo.ProfileByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrProfileNotFound
	}
	return s.repo.ListAvailability(ctx, profile.ID)
}

// ReplaceWeeklySchedule swaps the recurring week, leaving dated overrides alone.
func (s *Service) ReplaceWeeklySchedule(ctx context.Context, userID int64, rules []Rule) error {
	profile, found, err := s.repo.ProfileByUser(ctx, userID)
	if err != nil {
		return err
	}
	if !found {
		return ErrProfileNotFound
	}
	return s.repo.ReplaceWeeklyAvailability(ctx, profile.ID, rules)
}

// AddOverride adds one dated rule.
func (s *Service) AddOverride(ctx context.Context, userID int64, rule Rule) error {
	if !rule.IsDated() {
		return fmt.Errorf("%w: an override names a date", ErrInvalidSpan)
	}
	profile, found, err := s.repo.ProfileByUser(ctx, userID)
	if err != nil {
		return err
	}
	if !found {
		return ErrProfileNotFound
	}
	return s.repo.AddAvailabilityRule(ctx, profile.ID, rule)
}

// DeleteAvailabilityRule removes one row of the caller's own schedule.
func (s *Service) DeleteAvailabilityRule(ctx context.Context, userID, ruleID int64) error {
	profile, found, err := s.repo.ProfileByUser(ctx, userID)
	if err != nil {
		return err
	}
	if !found {
		return ErrProfileNotFound
	}
	return s.repo.DeleteAvailabilityRule(ctx, ruleID, profile.ID)
}
