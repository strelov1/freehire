package buddy

import (
	"context"
	"fmt"
	"log"
	"time"
)

// MockReminderOffsets is how far ahead of a mock each reminder goes out: the mentorship
// offsets, for the same unmeasured reason. The claim is keyed on the offset in minutes, so
// a third offset costs no migration.
var MockReminderOffsets = []time.Duration{24 * time.Hour, time.Hour}

// mockReminderWindow is each offset's firing window, against a worker run every ten
// minutes: wide enough to survive missed runs, narrow enough that "in 24 hours" is true.
const mockReminderWindow = time.Hour

// mockReminderFloor is the bottom of one offset's window, never below zero.
func mockReminderFloor(offset time.Duration) time.Duration {
	if floor := offset - mockReminderWindow; floor > 0 {
		return floor
	}
	return 0
}

// MockReminderStats is what one run did.
type MockReminderStats struct {
	Sent   int
	Failed int
}

// SendDueMockReminders reminds both participants of each confirmed mock at most once per
// offset, in mentorship's order: CLAIM first and send only what was won, and RELEASE the
// claim when delivery fails so the next run retries — a missing "starts in an hour" costs
// somebody the mock, a duplicate costs them a duplicate. A mock booked inside an offset's
// window never gets that reminder (there was never that much notice to give); one already
// started gets none. Failing to read the work is the only error returned.
func (s *MockService) SendDueMockReminders(ctx context.Context, maxPerRun int32) (MockReminderStats, error) {
	var stats MockReminderStats
	// Nothing to send with means nothing to claim: a claim without a delivery is a
	// reminder marked sent forever.
	if s.notifier == nil {
		return stats, nil
	}
	for _, offset := range MockReminderOffsets {
		due, err := s.repo.MocksDueForReminder(ctx, offset, mockReminderFloor(offset), maxPerRun)
		if err != nil {
			return stats, fmt.Errorf("buddy: reading the %v mock reminders: %w", offset, err)
		}
		for _, m := range due {
			claimed, err := s.repo.ClaimMockReminder(ctx, m.ID, offset)
			if err != nil {
				stats.Failed++
				log.Printf("buddy: claiming the %v reminder of mock %s: %v", offset, m.ID, err)
				continue
			}
			if !claimed {
				continue
			}
			if err := s.notifier.MockReminder(ctx, m, offset); err != nil {
				stats.Failed++
				log.Printf("buddy: the %v reminder of mock %s: %v", offset, m.ID, err)
				// The release must land even when the run's own context is what failed the
				// delivery: a claim left in place silences this reminder for good.
				rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				if err := s.repo.ReleaseMockReminder(rctx, m.ID, offset); err != nil {
					log.Printf("buddy: releasing the %v reminder of mock %s: %v", offset, m.ID, err)
				}
				cancel()
				continue
			}
			stats.Sent++
		}
	}
	return stats, nil
}
