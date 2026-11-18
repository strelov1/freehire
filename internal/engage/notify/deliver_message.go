package notify

import (
	"context"
	"errors"
	"log"
	"slices"

	"github.com/strelov1/freehire/internal/platform/db"
)

// deliverMessage sends one person's due digests on one channel as a single message
// and finalizes every claimed match in it. A subscription whose own reads fail is
// released and counted, and the rest of the message still goes out.
func (r *Runner) deliverMessage(ctx context.Context, subIDs []int64, claims map[int64]*subscriptionClaim, stats *Stats) {
	// The claim's UPDATE ... RETURNING has no order, so the sections get one here.
	slices.Sort(subIDs)
	parts := make([]*part, 0, len(subIDs))
	for _, subID := range subIDs {
		info, err := r.store.GetSubscriptionForDelivery(ctx, subID)
		if err != nil {
			// Counted, not merely logged: this digest did not go out and burned no attempt, so otherwise
			// the only trace is the exit code, and a pass that failed to read every claimed subscription
			// would exit 0 and look like an idle queue.
			log.Printf("notify: load subscription %d for delivery: %v", subID, err)
			r.release(ctx, subID, claims[subID].jobIDs)
			stats.Failed++
			continue
		}
		parts = append(parts, &part{subID: subID, claim: claims[subID], info: info})
	}
	if len(parts) == 0 {
		return
	}
	// One person, one channel: the settings, the timezone and the recipient are the
	// same in every part, so the first one answers for the message.
	info := parts[0].info
	releaseAll := func() {
		for _, p := range parts {
			r.release(ctx, p.subID, p.claim.jobIDs)
		}
	}

	// The same gate deliver ran before claiming, repeated because the world moves between the two:
	// quiet hours can begin, a Telegram chat can be unlinked.
	stamped := info.DigestFrequency != FrequencyInstant
	dest, h := deliveryHold(r.now(), info, parts[0].claim.personLastSent)
	switch h {
	case holdDeferred:
		releaseAll()
		stats.Deferred++
		return
	case holdNoRecipient:
		// Not deliverable right now (e.g. Telegram not linked): soft-skip, keep the
		// matches pending for a later pass, do not count a failed attempt.
		releaseAll()
		stats.SoftSkips++
		return
	}

	ready := parts[:0]
	for _, p := range parts {
		jobs, err := r.store.GetJobsForDigest(ctx, p.claim.jobIDs)
		if err != nil {
			// Counted for the same reason as the subscription read above: the digest is not
			// going out, and releasing the claim leaves no other record that it did not.
			log.Printf("notify: load jobs for subscription %d: %v", p.subID, err)
			r.release(ctx, p.subID, p.claim.jobIDs)
			stats.Failed++
			continue
		}
		// The claim took the newest SnapshotCap matches. Anything pending behind it is COUNTED by this
		// digest, not re-queued: re-queuing left a busy `daily` subscription mailing the same backlog
		// for weeks. The count is read before the send so the message can state it.
		p.covered = r.countCovered(ctx, p.subID, p.claim)
		p.digest = buildDigest(p.info.SavedSearchName, jobs, p.covered)
		p.digest.SearchQuery = p.info.SavedSearchQuery
		// Record the in-app notification BEFORE sending, so the digest carries its row's id and each
		// channel's "and N more" tail can link to the full match set. A recording failure is not a
		// delivery failure: the digest goes out with a zero id and a generic tail.
		p.digest.NotificationID, p.createdRow = r.recordNotification(ctx, p.info, p.digest, p.claim.jobIDs)
		ready = append(ready, p)
	}
	parts = ready
	if len(parts) == 0 {
		return
	}

	msg := Message{UserID: info.UserID, WebhookSecret: info.WebhookSecret.String, Digests: make([]Digest, len(parts))}
	for i, p := range parts {
		msg.Digests[i] = p.digest
	}

	if err := r.notifier.Send(ctx, info.Channel, dest, msg); err != nil {
		// Withdraw on EVERY send error, including an ambiguous one (a timeout may mean the mail went
		// out): keeping the row would leave one per attempt, up to MaxAttempts per digest nobody
		// received. The cost is that a mail that did slip out links to a 404.
		//
		// Only when THIS delivery created the row: withdrawing a row shared with channels that did
		// deliver would erase their history. Failed matches stay pending and the retry finds the row.
		//
		// The gate is authorship, not "has anybody delivered", and the two differ only across two
		// overlapping notify processes (deliver() is sequential within a pass): a joiner can send, then
		// the creator withdraw, leaving a delivered digest with no history row. That gap is open:
		// closing it needs per-event delivery state this engine lacks, and reaching it takes a hand-run
		// pass overlapping the timer's, since systemd will not stack the unit.
		for _, p := range parts {
			if p.createdRow {
				r.withdrawNotification(ctx, p.subID, p.digest.NotificationID)
			}
		}
		// A channel with no registered notifier (e.g. email unconfigured) is not a delivery failure:
		// soft-skip, so matches stay pending without burning an attempt.
		if errors.Is(err, ErrChannelNotConfigured) {
			releaseAll()
			stats.SoftSkips++
			return
		}
		// The recipient is gone for good (a blocked/removed Telegram bot, or a webhook that answered
		// 410 Gone): forget it for whichever channel reported it and soft-skip. The subscription
		// survives (relinking or re-enabling resumes it) and every other delivery to that recipient
		// reads as not linked/disabled. Without this, one gone recipient failed a digest per pass
		// forever and turned the run's exit code red.
		//
		// Must dispatch on info.Channel: an unconditional unlinkTelegram would disable a user's
		// Telegram over an unrelated webhook's 410.
		if errors.Is(err, ErrRecipientGone) {
			r.forgetRecipient(ctx, parts[0].subID, info, err)
			releaseAll()
			stats.SoftSkips++
			return
		}
		log.Printf("notify: deliver subscriptions %v: %v", subIDsOf(parts), err)
		// A non-410 webhook failure still counts toward the destination's auto-disable threshold (see
		// recordWebhookFailure): subscription_matches.attempts resets with every new match, so a dead
		// destination would otherwise fail one match per pass forever.
		if info.Channel == ChannelWebhook {
			r.recordWebhookFailure(ctx, parts[0].subID, info.UserID, err)
		}
		for _, p := range parts {
			if ferr := r.store.RecordMatchDeliveryFailure(ctx, db.RecordMatchDeliveryFailureParams{
				SubscriptionID: p.subID,
				JobIds:         p.claim.jobIDs,
				LastError:      err.Error(),
				MaxAttempts:    r.cfg.MaxAttempts,
			}); ferr != nil {
				log.Printf("notify: record delivery failure for subscription %d: %v", p.subID, ferr)
			}
		}
		stats.Failed++
		return
	}

	for _, p := range parts {
		r.finalize(ctx, p, stamped)
	}
	if info.Channel == ChannelWebhook {
		if err := r.store.RecordWebhookDeliverySuccess(ctx, info.UserID); err != nil {
			// Read-side only (the settings page's "last delivered" line) — never a
			// reason to treat an otherwise-successful send as a failure.
			log.Printf("notify: record webhook delivery success for user %d (subscription %d): %v", info.UserID, parts[0].subID, err)
		}
	}
	stats.Delivered++
}
