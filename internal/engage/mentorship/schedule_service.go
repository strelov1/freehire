package mentorship

import (
	"context"
	"fmt"
	"time"
)

// There is deliberately no uncached MentorSlots beside CachedMentorSlots.
//
// It existed, and the dead-code guard found it unreachable the moment the publication
// check moved ahead of the cache — because that is where the "look the mentor up, then
// compute" pair now lives, and one entry point cannot drift from another. A second
// public read would be a second place to forget that a paused mentor answers as though
// they do not exist.

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
	profile, err := s.ownProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListAvailability(ctx, profile.ID)
}

// ReplaceWeeklySchedule swaps the recurring week, leaving dated overrides alone.
func (s *Service) ReplaceWeeklySchedule(ctx context.Context, userID int64, rules []Rule) error {
	profile, err := s.ownProfile(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.ReplaceWeeklyAvailability(ctx, profile.ID, rules)
}

// AddOverride adds one dated rule.
func (s *Service) AddOverride(ctx context.Context, userID int64, rule Rule) error {
	if !rule.IsDated() {
		return fmt.Errorf("%w: an override names a date", ErrInvalidSpan)
	}
	profile, err := s.ownProfile(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.AddAvailabilityRule(ctx, profile.ID, rule)
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
// operation starts here: an account with no profile and one that does not exist are the
// same answer, and writing that out at each call site is how one of them eventually says
// something else.
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
