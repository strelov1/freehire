package mentorship

import (
	"context"
	"fmt"
	"time"
)

// ReminderOffsets is how far ahead of a session each reminder goes out.
//
// Fixed rather than configurable, and deliberately: 24 hours and 1 hour are a guess until
// real no-show data exists, and a knob invites tuning a number nobody has measured. When
// there is data, this slice is what changes — and because the claim is keyed on the
// offset in minutes rather than on an enum, adding a third costs no migration.
var ReminderOffsets = []time.Duration{24 * time.Hour, time.Hour}

// ReminderStats is what one run did.
type ReminderStats struct {
	// Sent counts reminders actually delivered.
	Sent int
	// Failed counts the ones claimed but not delivered. They are NOT retried: the claim
	// has committed, and the alternative — releasing it — is how a mail server that fails
	// intermittently sends the same reminder every few minutes.
	Failed int
}

// SendDueReminders delivers the reminders that have come due, at most once each.
//
// The ORDER is the whole design. For each due session the reminder is CLAIMED first and
// sent only if the claim was won. A worker that sends first and records afterwards sends
// twice whenever the second step fails — and this runs every few minutes, so "whenever"
// is often.
//
// The claim is an insert against a composite primary key, so two concurrent runs cannot
// both win it. Losing it means somebody else is sending that reminder; sending anyway is
// the one outcome worse than not sending at all.
//
// A delivery failure is counted and stepped over. Failing to READ the work is different —
// nothing was attempted, and the run says so rather than reporting a quiet success.
func (s *Service) SendDueReminders(ctx context.Context, maxPerRun int32) (ReminderStats, error) {
	var stats ReminderStats

	// Nothing to send with means nothing to claim: a claim without a delivery marks a
	// reminder sent forever, and the next run would skip it.
	if s.notifier == nil {
		return stats, nil
	}

	for _, offset := range ReminderOffsets {
		due, err := s.repo.ListBookingsDueForReminder(ctx, offset, maxPerRun)
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
				continue
			}
			stats.Sent++
		}
	}
	return stats, nil
}
