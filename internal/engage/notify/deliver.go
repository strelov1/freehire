package notify

import (
	"context"
	"errors"
	"log"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/application/deliverywindow"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// deliver leases a batch of pending matches, groups them by subscription, and
// sends one digest per subscription. On success the included matches are marked
// notified; on failure the delivery bookkeeping retries/dead-letters them; a
// subscription that is not currently deliverable (e.g. Telegram unlinked) has its
// claim released so it is retried promptly rather than waiting out the lease.
func (r *Runner) deliver(ctx context.Context, stats *Stats) error {
	// One digest's worth per subscription, so a pass serves every subscription that has
	// anything pending rather than the lowest ids only. Floored like the batch size it
	// sits beside: a zero here is LIMIT 0 in the LATERAL, which claims nothing and
	// reports no error — the whole worker would go quietly idle.
	perSubscription := int32(r.cfg.SnapshotCap)
	if perSubscription < 1 {
		perSubscription = 1
	}
	claimed, err := r.store.ClaimSubscriptionMatches(ctx, db.ClaimSubscriptionMatchesParams{
		LeaseSeconds:    r.cfg.LeaseSeconds,
		PerSubscription: perSubscription,
		BatchSize:       r.cfg.ClaimBatch,
	})
	if err != nil {
		return err
	}

	// Group the claimed matches by subscription so each becomes one digest.
	jobsBySub := make(map[int64][]int64)
	order := make([]int64, 0)
	for _, c := range claimed {
		if _, seen := jobsBySub[c.SubscriptionID]; !seen {
			order = append(order, c.SubscriptionID)
		}
		jobsBySub[c.SubscriptionID] = append(jobsBySub[c.SubscriptionID], c.JobID)
	}

	for _, subID := range order {
		r.deliverOne(ctx, subID, jobsBySub[subID], stats)
	}
	return nil
}

// deliverOne sends one subscription's digest and finalizes its claimed matches.
func (r *Runner) deliverOne(ctx context.Context, subID int64, jobIDs []int64, stats *Stats) {
	info, err := r.store.GetSubscriptionForDelivery(ctx, subID)
	if err != nil {
		log.Printf("notify: load subscription %d for delivery: %v", subID, err)
		r.release(ctx, subID, jobIDs)
		return
	}

	// Delivery timing: an `instant` subscription (the default — anything other
	// than exactly "daily" reads as instant) defers to quiet hours; a `daily`
	// one waits for its own configured time instead and is exempt from quiet
	// hours — a chosen digest time is itself the user's preference.
	daily := info.DigestFrequency == "daily"
	tz := info.Timezone.String
	if daily {
		if !deliverywindow.DigestDue(r.now(), tz, pgconv.DurationPtr(info.DigestTime), pgconv.TimePtr(info.LastDigestSentAt)) {
			r.release(ctx, subID, jobIDs)
			stats.Deferred++
			return
		}
	} else if deliverywindow.InQuietHours(r.now(), tz, pgconv.DurationPtr(info.QuietHoursStart), pgconv.DurationPtr(info.QuietHoursEnd)) {
		r.release(ctx, subID, jobIDs)
		stats.Deferred++
		return
	}

	dest, ok := recipient(info)
	if !ok {
		// Not deliverable right now (e.g. Telegram not linked): soft-skip, keep the
		// matches pending for a later pass, do not count a failed attempt.
		r.release(ctx, subID, jobIDs)
		stats.SoftSkips++
		return
	}

	jobs, err := r.store.GetJobsForDigest(ctx, jobIDs)
	if err != nil {
		log.Printf("notify: load jobs for subscription %d: %v", subID, err)
		r.release(ctx, subID, jobIDs)
		return
	}

	jobs, jobIDs = r.deferOverflow(ctx, subID, jobs, jobIDs)
	digest := buildDigest(info.SavedSearchName, jobs)

	// Record the in-app notification BEFORE sending, so the digest can carry its
	// own row's id and each channel's "and N more" tail can link to the page
	// that lists the full match set. A recording failure is not a delivery
	// failure — the digest goes out with a zero id and the tail falls back to a
	// generic destination.
	digest.NotificationID = r.recordNotification(ctx, subID, info, digest)

	if err := r.notifier.Send(ctx, info.Channel, dest, digest); err != nil {
		// Withdraw on EVERY send error, including an ambiguous one (a timeout may
		// mean the mail went out and only the acknowledgement was lost). Keeping
		// the row on ambiguous errors would leave one behind per attempt, and the
		// matches stay pending either way — so a channel outage would fill the
		// history with up to MaxAttempts rows per digest nobody received, which is
		// the failure this ordering exists to avoid. The cost of withdrawing is
		// narrower: a mail that did slip out links to a page that 404s.
		r.withdrawNotification(ctx, subID, digest.NotificationID)
		// A channel with no registered notifier (e.g. email while SES is
		// unconfigured) is not a delivery failure: soft-skip so the matches stay
		// pending for a pass once the channel is provisioned, without burning an
		// attempt toward the dead-letter limit.
		if errors.Is(err, ErrChannelNotConfigured) {
			r.release(ctx, subID, jobIDs)
			stats.SoftSkips++
			return
		}
		// The recipient is gone for good — a blocked/removed Telegram bot, or (per
		// webhooknotify's own mapping) a webhook destination that answered 410
		// Gone — and no retry reaches it again. Forget the recipient for whichever
		// channel reported it and soft-skip: the subscription survives (relinking
		// Telegram, or re-enabling the webhook, resumes it) while every other
		// delivery to that same recipient, in this worker and (for Telegram) in
		// remind/nudge alike, now reads as "not linked"/"disabled" and soft-skips
		// too.
		//
		// Without this one blocked/gone recipient failed a digest per pass forever,
		// and the exit code that failure produces turned the whole run red — which
		// is how a stranger's choice to mute a bot, or retire an endpoint, became a
		// worker in a failed state. This must dispatch on info.Channel: an
		// unconditional unlinkTelegram here would disable a user's Telegram link
		// over an unrelated webhook's 410.
		if errors.Is(err, ErrRecipientGone) {
			r.forgetRecipient(ctx, subID, info, err)
			r.release(ctx, subID, jobIDs)
			stats.SoftSkips++
			return
		}
		log.Printf("notify: deliver subscription %d: %v", subID, err)
		if ferr := r.store.RecordMatchDeliveryFailure(ctx, db.RecordMatchDeliveryFailureParams{
			SubscriptionID: subID,
			JobIds:         jobIDs,
			LastError:      err.Error(),
			MaxAttempts:    r.cfg.MaxAttempts,
		}); ferr != nil {
			log.Printf("notify: record delivery failure for subscription %d: %v", subID, ferr)
		}
		stats.Failed++
		return
	}

	if _, err := r.store.MarkMatchesNotified(ctx, db.MarkMatchesNotifiedParams{
		SubscriptionID: subID,
		JobIds:         jobIDs,
	}); err != nil {
		// Delivered but not stamped: the lease expiry will re-deliver (a rare
		// duplicate), which is preferable to losing the notification.
		log.Printf("notify: mark notified for subscription %d: %v", subID, err)
	}

	if info.Channel == ChannelWebhook {
		if err := r.store.RecordWebhookDeliverySuccess(ctx, info.UserID); err != nil {
			// Read-side only (the settings page's "last delivered" line) — never a
			// reason to treat an otherwise-successful send as a failure.
			log.Printf("notify: record webhook delivery success for user %d (subscription %d): %v", info.UserID, subID, err)
		}
	}

	if daily {
		if err := r.store.MarkDigestSent(ctx, subID); err != nil {
			// Delivered but not stamped: DigestDue would deliver again on the next
			// pass today (a rare duplicate), preferable to skipping tomorrow's digest.
			log.Printf("notify: mark digest sent for subscription %d: %v", subID, err)
		}
	}

	stats.Delivered++
}

// recordNotification writes the digest's in-app notification-center row and
// returns its id, or zero if the write failed. A failure is a degraded read-side
// feature (and a tail that falls back to a generic destination), never a reason
// to hold back a digest that is otherwise ready to send.
func (r *Runner) recordNotification(ctx context.Context, subID int64, info db.GetSubscriptionForDeliveryRow, d Digest) int64 {
	title, body, slug := renderDigest(d)
	var publicSlug pgtype.Text
	if slug != "" {
		publicSlug = pgtype.Text{String: slug, Valid: true}
	}
	id, err := r.store.RecordNotification(ctx, db.RecordNotificationParams{
		UserID:     info.UserID,
		Kind:       "subscription_digest",
		Title:      title,
		Body:       body,
		PublicSlug: publicSlug,
		Jobs:       digestJobsSnapshot(d),
	})
	if err != nil {
		log.Printf("notify: record notification for subscription %d: %v", subID, err)
		return 0
	}
	return id
}

// withdrawNotification removes the row recordNotification wrote for a digest
// that then failed to send. A failure here leaves one history row describing a
// digest nobody received — logged, and strictly better than the alternative
// reading of the same window, which would be to drop a delivery that succeeded.
func (r *Runner) withdrawNotification(ctx context.Context, subID, id int64) {
	if id == 0 {
		return
	}
	if err := r.store.DeleteNotification(ctx, id); err != nil {
		log.Printf("notify: withdraw notification %d for subscription %d: %v", id, subID, err)
	}
}

// unlinkTelegram forgets the user's Telegram chat after a send reported it
// permanently closed, and says so in the log — this is a visible change to the
// user's settings made without them asking, so it should not happen silently.
//
// A failure to unlink is logged and nothing more: the digest is soft-skipped
// either way, and the next pass will meet the same 403 and try again.
func (r *Runner) unlinkTelegram(ctx context.Context, subID, userID int64, cause error) {
	rows, err := r.store.DeleteTelegramLink(ctx, userID)
	if err != nil {
		log.Printf("notify: unlink telegram for user %d (subscription %d): %v", userID, subID, err)
		return
	}
	if rows == 0 {
		// Already unlinked by an earlier subscription in this same pass — every
		// telegram subscription this user has meets the same 403.
		return
	}
	log.Printf("notify: unlinked telegram for user %d after subscription %d: %v", userID, subID, cause)
}

// forgetRecipient dispatches an ErrRecipientGone side effect by channel: which
// stored recipient to forget differs per channel, so this cannot be one call.
// Anything other than telegram/webhook has no such record to forget — for those,
// the soft-skip in deliverOne is already the whole response.
func (r *Runner) forgetRecipient(ctx context.Context, subID int64, info db.GetSubscriptionForDeliveryRow, cause error) {
	switch info.Channel {
	case ChannelTelegram:
		r.unlinkTelegram(ctx, subID, info.UserID, cause)
	case ChannelWebhook:
		r.disableWebhook(ctx, subID, info.UserID, cause)
	}
}

// disableWebhook turns off a user's webhook destination after a send reported
// it permanently gone (an HTTP 410) — the webhook channel's counterpart to
// unlinkTelegram, and, like it, a visible change to the user's settings made
// without them asking, so it should not happen silently.
func (r *Runner) disableWebhook(ctx context.Context, subID, userID int64, cause error) {
	rows, err := r.store.DisableWebhookConfig(ctx, userID)
	if err != nil {
		log.Printf("notify: disable webhook for user %d (subscription %d): %v", userID, subID, err)
		return
	}
	if rows == 0 {
		// Already disabled by an earlier subscription in this same pass — every
		// webhook subscription this user has meets the same 410.
		return
	}
	log.Printf("notify: disabled webhook for user %d after subscription %d: %v", userID, subID, cause)
}

// release drops the lease on a subscription's claimed matches so they are retried
// promptly on a later pass.
func (r *Runner) release(ctx context.Context, subID int64, jobIDs []int64) {
	if err := r.store.ReleaseMatchClaim(ctx, db.ReleaseMatchClaimParams{
		SubscriptionID: subID,
		JobIds:         jobIDs,
	}); err != nil {
		log.Printf("notify: release claim for subscription %d: %v", subID, err)
	}
}

// deferOverflow trims a claimed set larger than one digest may carry, releasing
// the excess back to the pending queue so a later pass delivers it.
//
// Truncating without releasing would stamp those matches notified while they
// appeared in no message and in no recorded snapshot — the postings would leave
// the alert having never been shown. Because GetJobsForDigest orders freshest
// first, what is held back is the oldest of the claimed set.
//
// A claimed id with no job row (pruned between match and delivery) is NOT held
// back: it stays in jobIDs and is stamped notified, or it would be re-claimed
// every pass forever.
func (r *Runner) deferOverflow(ctx context.Context, subID int64, jobs []db.GetJobsForDigestRow, jobIDs []int64) ([]db.GetJobsForDigestRow, []int64) {
	if len(jobs) <= r.cfg.SnapshotCap {
		return jobs, jobIDs
	}

	held := make(map[int64]bool, len(jobs)-r.cfg.SnapshotCap)
	for _, j := range jobs[r.cfg.SnapshotCap:] {
		held[j.ID] = true
	}
	deferred := make([]int64, 0, len(held))
	kept := make([]int64, 0, len(jobIDs)-len(held))
	for _, id := range jobIDs {
		if held[id] {
			deferred = append(deferred, id)
			continue
		}
		kept = append(kept, id)
	}
	log.Printf("notify: subscription %d claimed %d matches, delivering %d and deferring %d", subID, len(jobIDs), len(kept), len(deferred))
	r.release(ctx, subID, deferred)
	return jobs[:r.cfg.SnapshotCap], kept
}

// buildDigest assembles a digest over the jobs this delivery carries. Total is
// len(jobs) because deferOverflow has already held back anything that would not
// fit, so a digest never announces a job it cannot show; the "and N more" tail a
// renderer draws is the difference between Total and Digest.Listed.
func buildDigest(name string, jobs []db.GetJobsForDigestRow) Digest {
	d := Digest{SavedSearchName: name, Total: len(jobs)}
	for _, j := range jobs {
		d.Jobs = append(d.Jobs, DigestJob{
			Title:          j.Title,
			Company:        j.Company,
			Slug:           j.PublicSlug,
			URL:            j.URL,
			SalaryMin:      int(j.SalaryMin),
			SalaryMax:      int(j.SalaryMax),
			SalaryCurrency: j.SalaryCurrency,
			SalaryPeriod:   j.SalaryPeriod,
		})
	}
	return d
}
