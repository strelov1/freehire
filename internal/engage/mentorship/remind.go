package mentorship

import (
	"context"
	"fmt"
	"time"
)

// ReminderOffsets is how far ahead of a session each reminder goes out.
//
// Fixed, not configurable: 24 hours and 1 hour are a guess until there is no-show data.
// The claim is keyed on the offset in minutes, so adding one needs no migration.
var ReminderOffsets = []time.Duration{24 * time.Hour, time.Hour}

// reminderWindow is how wide each offset's firing window is.
//
// A reminder is due when the session starts between `offset - reminderWindow` and `offset`
// from now; without the lower bound the 24-hour reminder would fire for a session three
// hours away.
//
// An hour, against a ten-minute worker schedule: wide enough to survive a few missed
// firings, narrow enough that "in 24 hours" is true to within an hour.
//
// A session booked closer than an offset never gets that reminder.
const reminderWindow = time.Hour

// reminderFloor is the bottom of one offset's window, never below zero.
func reminderFloor(offset time.Duration) time.Duration {
	if floor := offset - reminderWindow; floor > 0 {
		return floor
	}
	return 0
}

// ReminderStats is what one run did.
type ReminderStats struct {
	// Sent counts reminders actually delivered.
	Sent int
	// Failed counts the ones claimed but not delivered. Their claim is released, so the next
	// run retries.
	Failed int
}

// SendDueReminders delivers the reminders that have come due, at most once each.
//
// Order matters: each reminder is claimed first and sent only if the claim was won, since
// send-then-record sends twice whenever recording fails.
//
// The claim is an insert on a composite primary key, so two concurrent runs cannot both
// win it; losing it means somebody else is sending.
//
// A delivery failure releases the claim so the next run retries: a missed reminder costs
// someone the session, a duplicate only a duplicate. The worst case is one repeat against
// a server that fails after accepting.
//
// Failing to read the work returns an error rather than a quiet success.
func (s *Service) SendDueReminders(ctx context.Context, maxPerRun int32) (ReminderStats, error) {
	var stats ReminderStats

	// Nothing to send with means nothing to claim: a claim without a delivery would mark the
	// reminder sent forever.
	if s.notifier == nil {
		return stats, nil
	}

	for _, offset := range ReminderOffsets {
		due, err := s.repo.ListBookingsDueForReminder(ctx, offset, reminderFloor(offset), maxPerRun)
		if err != nil {
			return stats, fmt.Errorf("mentorship: reading the %v reminders: %w", offset, err)
		}

		for _, booking := range due {
			claimed, err := s.repo.ClaimReminder(ctx, booking.ID, offset)
			if err != nil {
				stats.Failed++
				logDeliveryFailure("reminder claim", booking, err)
				continue
			}
			if !claimed {
				continue
			}
			if err := s.notifier.BookingReminder(ctx, booking, offset); err != nil {
				stats.Failed++
				logDeliveryFailure("reminder", booking, err)
				// Give the claim back so the next run retries. A failed release leaves the reminder
				// claimed and unsent, so it is logged loudly.
				if err := s.repo.ReleaseReminderClaim(ctx, booking.ID, offset); err != nil {
					logDeliveryFailure("reminder claim release", booking, err)
				}
				continue
			}
			stats.Sent++
		}
	}
	return stats, nil
}
