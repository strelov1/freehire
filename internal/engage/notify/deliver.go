package notify

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
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
// Stale matches are expired first, so nothing stale is ever claimed.
func (r *Runner) deliver(ctx context.Context, stats *Stats) error {
	expired, err := r.expireStale(ctx)
	stats.Expired += expired
	if err != nil {
		return fmt.Errorf("expire stale matches: %w", err)
	}

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
	claims := make(map[int64]*subscriptionClaim)
	order := make([]int64, 0)
	for _, c := range claimed {
		sc, seen := claims[c.SubscriptionID]
		if !seen {
			sc = &subscriptionClaim{oldest: c}
			claims[c.SubscriptionID] = sc
			order = append(order, c.SubscriptionID)
		}
		sc.add(c)
	}
	for _, sc := range claims {
		sc.full = len(sc.jobIDs) >= int(perSubscription)
	}

	for _, subID := range order {
		// A cancelled run stops here rather than walking the rest of the claimed
		// subscriptions: each would fail instantly against the same dead context and be
		// counted a delivery failure, so one SIGTERM would report as an outage across
		// every subscription. Their claims simply expire and the next pass retries them,
		// which is what the lease is for.
		if ctx.Err() != nil {
			log.Printf("notify: delivery stopped after %d of %d claimed subscriptions: %v",
				stats.Delivered+stats.SoftSkips+stats.Deferred+stats.Failed, len(order), ctx.Err())
			return nil
		}
		r.deliverOne(ctx, subID, claims[subID], stats)
	}
	return nil
}

// deliverOne sends one subscription's digest and finalizes its claimed matches.
func (r *Runner) deliverOne(ctx context.Context, subID int64, claim *subscriptionClaim, stats *Stats) {
	jobIDs := claim.jobIDs
	info, err := r.store.GetSubscriptionForDelivery(ctx, subID)
	if err != nil {
		// Counted, not merely logged. This digest did not go out and nothing burned an
		// attempt toward the dead-letter limit, so the only trace it leaves is the exit
		// code — and a pass that failed to read every subscription it claimed used to
		// print `delivered=0 failed=0` and exit 0, which reads as an idle queue.
		log.Printf("notify: load subscription %d for delivery: %v", subID, err)
		r.release(ctx, subID, jobIDs)
		stats.Failed++
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
		// Counted for the same reason as the subscription read above: the digest is not
		// going out, and releasing the claim leaves no other record that it did not.
		log.Printf("notify: load jobs for subscription %d: %v", subID, err)
		r.release(ctx, subID, jobIDs)
		stats.Failed++
		return
	}

	// The claim took the newest SnapshotCap matches. Anything pending behind it is
	// COUNTED by this digest rather than re-queued for a later one: re-queuing is what
	// left a busy `daily` subscription mailing the same backlog, oldest first, weeks
	// behind. The count is read before the send so the message can state it.
	covered := r.countCovered(ctx, subID, claim)
	digest := buildDigest(info.SavedSearchName, jobs, covered)
	digest.SearchQuery = info.SavedSearchQuery
	digest.UserID = info.UserID

	// Record the in-app notification BEFORE sending, so the digest can carry its
	// own row's id and each channel's "and N more" tail can link to the page
	// that lists the full match set. A recording failure is not a delivery
	// failure — the digest goes out with a zero id and the tail falls back to a
	// generic destination.
	//
	// createdRow says whether THIS delivery wrote the row or joined one another
	// channel of the same saved search had already written; only the creator may
	// withdraw it below.
	notificationID, createdRow := r.recordNotification(ctx, info, digest, jobIDs)
	digest.NotificationID = notificationID

	if err := r.notifier.Send(ctx, info.Channel, dest, digest); err != nil {
		// Withdraw on EVERY send error, including an ambiguous one (a timeout may
		// mean the mail went out and only the acknowledgement was lost). Keeping
		// the row on ambiguous errors would leave one behind per attempt, and the
		// matches stay pending either way — so a channel outage would fill the
		// history with up to MaxAttempts rows per digest nobody received, which is
		// the failure this ordering exists to avoid. The cost of withdrawing is
		// narrower: a mail that did slip out links to a page that 404s.
		//
		// Only when THIS delivery created the row. A saved search on three channels
		// records the event once, and withdrawing it because the third channel failed
		// would erase the history of the two digests that did arrive — the matches
		// that failed stay pending and the retry finds the row still there.
		//
		// The gate is AUTHORSHIP, not "has anybody delivered this yet", and the two
		// differ only when a second notify process is running: deliver() is one
		// sequential goroutine, so within a pass the creator sends before any joiner
		// records. Across two overlapping passes a joiner can send successfully and
		// the creator then withdraw underneath it, leaving a delivered digest with
		// no history row.
		//
		// That ONE ordering is the remaining gap, and it is not the one the
		// post-send record below closes: a joiner that read a live id has nothing to
		// notice, because the row is removed after it has finished. Closing it needs
		// per-event delivery state for "has anybody delivered" to read, which this
		// engine does not keep; systemd will not stack this unit on itself, so
		// reaching it takes a hand-run pass overlapping the timer's.
		if createdRow {
			r.withdrawNotification(ctx, subID, digest.NotificationID)
		}
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
		// A webhook failure that is NOT a 410 (handled above) still counts toward
		// the destination's own auto-disable threshold — see recordWebhookFailure
		// — because subscription_matches.attempts resets with every new match,
		// so a destination that is simply dead would otherwise fail one match
		// per pass forever instead of being disabled once.
		if info.Channel == ChannelWebhook {
			r.recordWebhookFailure(ctx, subID, info.UserID, err)
		}
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

	// The digest went out. If it is carrying a zero id, nothing in the history
	// names it yet: either the pre-send write failed, or it conflicted with a row
	// that was then withdrawn before this channel could read it. Record it now.
	//
	// This is the ONE place the ordering can be after the send, because the id is
	// no longer needed for anything — the message is gone, its tail already fell
	// back to the generic destination. What is left to get right is only whether
	// the history admits a digest the person actually received, and it must.
	//
	// Safe to attempt blindly: the dedup key makes a redundant insert a no-op, so
	// a first write that in fact succeeded and only lost its answer cannot become
	// a second row here.
	if digest.NotificationID == 0 {
		r.recordNotification(ctx, info, digest, jobIDs)
	}

	if _, err := r.store.MarkMatchesNotified(ctx, db.MarkMatchesNotifiedParams{
		SubscriptionID: subID,
		JobIds:         jobIDs,
	}); err != nil {
		// Delivered but not stamped: the lease expiry will re-deliver (a rare
		// duplicate), which is preferable to losing the notification.
		log.Printf("notify: mark notified for subscription %d: %v", subID, err)
	}
	if covered > 0 {
		if _, err := r.store.MarkCoveredMatchesNotified(ctx, db.MarkCoveredMatchesNotifiedParams(claim.coveredParams(subID))); err != nil {
			// The digest counted them but they stay pending: the next digest carries the
			// newest of them again or they go stale — a repeat at worst, never a loss.
			log.Printf("notify: mark covered matches notified for subscription %d: %v", subID, err)
		}
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
//
// created reports whether this delivery WROTE the row, as opposed to joining one
// another channel of the same saved search had already written. The notification
// centre holds one row per EVENT, so only the writer may withdraw it.
func (r *Runner) recordNotification(ctx context.Context, info db.GetSubscriptionForDeliveryRow, d Digest, jobIDs []int64) (id int64, created bool) {
	title, body, slug := renderDigest(d)
	if d.Overflowed() {
		// The row's page lists only the snapshot, under this body; say so, or it
		// reads as a count the list beneath it does not reach.
		body = fmt.Sprintf("%s — the newest %d are listed", body, len(d.Jobs))
	}
	var publicSlug pgtype.Text
	if slug != "" {
		publicSlug = pgtype.Text{String: slug, Valid: true}
	}
	dedupKey := pgtype.Text{String: digestDedupKey(info.SavedSearchID, jobIDs), Valid: true}
	id, err := r.store.RecordNotification(ctx, db.RecordNotificationParams{
		UserID:     info.UserID,
		Kind:       kindSubscriptionDigest,
		Title:      title,
		Body:       body,
		PublicSlug: publicSlug,
		Jobs:       digestJobsSnapshot(d),
		DedupKey:   dedupKey,
	})
	if err == nil {
		return id, true
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("notify: record notification for subscription %d: %v", info.ID, err)
		return 0, false
	}
	// No row inserted: another channel of this same saved search already
	// recorded this event. Link this channel's message at that row rather than
	// adding a second indistinguishable one to the history (freehire#3020).
	id, err = r.store.GetNotificationIDByDedupKey(ctx, db.GetNotificationIDByDedupKeyParams{
		UserID:   info.UserID,
		DedupKey: dedupKey,
	})
	if err != nil {
		// The row was withdrawn between the two statements, which takes a SECOND
		// notify process: deliver() walks its subscriptions sequentially in one
		// goroutine, so within a pass the creator has finished sending (and
		// withdrawing) before this channel records at all. systemd will not start
		// a `Type=oneshot` unit while the first is active, so the way to reach
		// here is a hand-run pass overlapping the timer's.
		//
		// The send then goes out with a zero id and its tail falls back to a
		// generic destination — that much is unavoidable, the message has to be
		// built before it can be sent. What is NOT left to chance is the history:
		// deliverOne records again after a successful send precisely because of
		// this branch. Resurrecting a row another worker withdrew is the right
		// answer here and only here — that worker withdrew it because ITS OWN send
		// failed, which says nothing about this one, and the person reading the
		// history did receive this digest.
		log.Printf("notify: read deduped notification for subscription %d: %v", info.ID, err)
		return 0, false
	}
	return id, false
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

// recordWebhookFailure counts a non-410 webhook send failure toward the
// destination's auto-disable threshold and logs when that crosses it — the
// non-410 counterpart to disableWebhook, reached from the generic failure
// branch rather than the ErrRecipientGone one since the destination never
// said it was gone, it just keeps failing.
func (r *Runner) recordWebhookFailure(ctx context.Context, subID, userID int64, cause error) {
	row, err := r.store.RecordWebhookDeliveryFailure(ctx, db.RecordWebhookDeliveryFailureParams{
		UserID:      userID,
		MaxFailures: r.cfg.WebhookMaxConsecutiveFailures,
	})
	if err != nil {
		log.Printf("notify: record webhook delivery failure for user %d (subscription %d): %v", userID, subID, err)
		return
	}
	if !row.Enabled {
		log.Printf("notify: disabled webhook for user %d after %d consecutive failures (subscription %d): %v",
			userID, row.ConsecutiveFailures, subID, cause)
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

// expireBatch bounds one ExpireStaleSubscriptionMatches statement. The first pass
// after deploy meets a backlog of tens of thousands; batching keeps it a run of
// short statements instead of one long one holding row locks on a busy host.
const expireBatch = 5000

// expireStale takes every stale match out of the pending set, one bounded batch at
// a time until a batch comes back short, and reports how many it took.
//
// A window under a second is refused rather than run: it becomes a zero-second
// interval in SQL, and expiry is terminal — every pending match would go stale
// before it could be claimed.
func (r *Runner) expireStale(ctx context.Context) (int, error) {
	if r.cfg.FreshnessWindow < time.Second {
		return 0, fmt.Errorf("FreshnessWindow must be at least one second, got %v", r.cfg.FreshnessWindow)
	}
	total := 0
	for {
		n, err := r.store.ExpireStaleSubscriptionMatches(ctx, db.ExpireStaleSubscriptionMatchesParams{
			LeaseSeconds:     r.cfg.LeaseSeconds,
			FreshnessSeconds: int32(r.cfg.FreshnessWindow / time.Second),
			BatchSize:        expireBatch,
		})
		total += int(n)
		if err != nil {
			return total, err
		}
		if n < expireBatch {
			return total, nil
		}
	}
}

// subscriptionClaim is what one pass claimed for one subscription: the job ids,
// whether the claim was FULL (it hit the per-subscription cap, so more may be
// pending behind it), and the oldest row it took, which is where it stopped.
type subscriptionClaim struct {
	jobIDs []int64
	oldest db.ClaimSubscriptionMatchesRow
	full   bool
}

// add appends one claimed row. The claim is newest first in SQL, but UPDATE ...
// RETURNING promises no order, so the stopping point is computed here.
func (c *subscriptionClaim) add(row db.ClaimSubscriptionMatchesRow) {
	c.jobIDs = append(c.jobIDs, row.JobID)
	at, oldest := row.MatchedAt.Time, c.oldest.MatchedAt.Time
	if at.Before(oldest) || (at.Equal(oldest) && row.JobID < c.oldest.JobID) {
		c.oldest = row
	}
}

// coveredParams names the pending matches behind the claim's stopping point, for
// CountCoveredMatches and MarkCoveredMatchesNotified alike — one value, so the
// count a digest announces and the set it retires cannot drift apart.
func (c *subscriptionClaim) coveredParams(subID int64) db.CountCoveredMatchesParams {
	return db.CountCoveredMatchesParams{
		SubscriptionID:  subID,
		BeforeMatchedAt: c.oldest.MatchedAt,
		BeforeJobID:     c.oldest.JobID,
	}
}

// countCovered is how many pending matches the digest counts without carrying.
// Zero for a claim that came back short — nothing can be behind it — and zero
// when the count fails: the digest then announces only what it carries and
// retires nothing it did not count.
func (r *Runner) countCovered(ctx context.Context, subID int64, claim *subscriptionClaim) int {
	if !claim.full {
		return 0
	}
	n, err := r.store.CountCoveredMatches(ctx, claim.coveredParams(subID))
	if err != nil {
		log.Printf("notify: count covered matches for subscription %d: %v", subID, err)
		return 0
	}
	return int(n)
}

// buildDigest assembles a digest over the jobs this delivery carries plus the
// covered matches it only counts. The "and N more" tail a renderer draws is the
// difference between Total and Digest.Listed; with covered above zero the
// digest is Overflowed and that tail leads to the saved search.
//
// A claimed id with no job row (pruned between match and delivery) carries no
// job here but stays in the claim, so it is stamped notified with the rest
// rather than re-claimed every pass forever.
func buildDigest(name string, jobs []db.GetJobsForDigestRow, covered int) Digest {
	d := Digest{SavedSearchName: name, Total: len(jobs) + covered}
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
