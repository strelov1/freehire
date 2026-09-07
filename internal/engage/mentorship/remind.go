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
	// Failed counts the ones claimed but not delivered. Their claim is RELEASED, so the
	// next run tries again — see the argument in SendDueReminders.
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
// A DELIVERY FAILURE RELEASES THE CLAIM, so the next run tries again. Holding it would
// mean a mail server down for one run loses that reminder permanently, and the two
// failures are not equal: a missing "your session starts in an hour" costs somebody the
// session, while a duplicate costs them a duplicate. The release is safe in the shape
// that matters — the transport reported an error, so the message almost certainly did
// not go out — and the worst case it admits is one repeated reminder against a server
// that fails after accepting.
//
// Failing to READ the work is different again: nothing was attempted, and the run says so
// rather than reporting a quiet success.
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
				// Give the claim back so the next run retries. A release that itself
				// fails leaves the reminder claimed and unsent — the outcome this whole
				// branch exists to avoid — so it is logged loudly rather than swallowed.
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
