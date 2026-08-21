package notify

import (
	"context"
	"errors"
	"log"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/deliverywindow"
	"github.com/strelov1/freehire/internal/pgconv"
)

// deliver leases a batch of pending matches, groups them by subscription, and
// sends one digest per subscription. On success the included matches are marked
// notified; on failure the delivery bookkeeping retries/dead-letters them; a
// subscription that is not currently deliverable (e.g. Telegram unlinked) has its
// claim released so it is retried promptly rather than waiting out the lease.
func (r *Runner) deliver(ctx context.Context, stats *Stats) error {
	claimed, err := r.store.ClaimSubscriptionMatches(ctx, db.ClaimSubscriptionMatchesParams{
		LeaseSeconds: r.cfg.LeaseSeconds,
		BatchSize:    r.cfg.ClaimBatch,
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

	digest := buildDigest(info.SavedSearchName, jobs, r.cfg.SnapshotCap)

	// Record the in-app notification BEFORE sending, so the digest can carry its
	// own row's id and each channel's "and N more" tail can link to the page
	// that lists the full match set. A recording failure is not a delivery
	// failure — the digest goes out with a zero id and the tail falls back to a
	// generic destination.
	notificationID := r.recordNotification(ctx, subID, info, digest)
	digest.NotificationID = notificationID

	if err := r.notifier.Send(ctx, info.Channel, dest, digest); err != nil {
		// Nothing was delivered, so the row recorded above must not survive:
		// the history holds a digest if and only if that digest went out.
		r.withdrawNotification(ctx, subID, notificationID)
		// A channel with no registered notifier (e.g. email while SES is
		// unconfigured) is not a delivery failure: soft-skip so the matches stay
		// pending for a pass once the channel is provisioned, without burning an
		// attempt toward the dead-letter limit.
		if errors.Is(err, ErrChannelNotConfigured) {
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

// buildDigest assembles a digest carrying the first `limit` matched jobs — the
// snapshot ceiling, not the message listing bound (see Digest.Listed) — while
// Total reflects all matched jobs so the renderer can summarize the remainder.
func buildDigest(name string, jobs []db.GetJobsForDigestRow, limit int) Digest {
	d := Digest{SavedSearchName: name, Total: len(jobs)}
	for i, j := range jobs {
		if i >= limit {
			break
		}
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
