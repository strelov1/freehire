package notify

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/db"
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

	digest := buildDigest(info.SavedSearchName, jobs, r.cfg.DigestCap)
	if err := r.notifier.Send(ctx, info.Channel, dest, digest); err != nil {
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
	stats.Delivered++
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

// buildDigest assembles a capped digest: the first `limit` jobs are listed, but
// Total reflects all matched jobs so the renderer can summarize the remainder.
func buildDigest(name string, jobs []db.GetJobsForDigestRow, limit int) Digest {
	d := Digest{SavedSearchName: name, Total: len(jobs)}
	for i, j := range jobs {
		if i >= limit {
			break
		}
		d.Jobs = append(d.Jobs, DigestJob{
			Title:   j.Title,
			Company: j.Company,
			Slug:    j.PublicSlug,
			URL:     j.URL,
		})
	}
	return d
}
