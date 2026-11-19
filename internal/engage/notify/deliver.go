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

// deliver leases a batch of pending matches, groups them by subscription and recipient, and
// sends one message per person and channel with a digest per subscription. Sent matches are
// marked notified; failures go to the retry/dead-letter bookkeeping. A subscription not
// deliverable now (quiet hours, daily digest not due, Telegram unlinked) is never leased
// (deliverableSubscriptions); one that became undeliverable after the lease has its claim
// released. Stale matches are expired first, so nothing stale is claimed.
func (r *Runner) deliver(ctx context.Context, stats *Stats) error {
	expired, err := r.expireStale(ctx)
	stats.Expired += expired
	if err != nil {
		return fmt.Errorf("expire stale matches: %w", err)
	}

	// One digest's worth per subscription, so a pass serves every subscription with anything
	// pending, not the lowest ids only. Floored like the batch size: zero is LIMIT 0 in the LATERAL,
	// which claims nothing, reports no error and idles the whole worker.
	perSubscription := int32(r.cfg.SnapshotCap)
	if perSubscription < 1 {
		perSubscription = 1
	}
	eligible, err := r.deliverableSubscriptions(ctx, stats)
	if err != nil {
		return err
	}
	if len(eligible) == 0 {
		return nil
	}
	claimed, err := r.store.ClaimSubscriptionMatches(ctx, db.ClaimSubscriptionMatchesParams{
		LeaseSeconds:    r.cfg.LeaseSeconds,
		PerSubscription: perSubscription,
		BatchSize:       r.cfg.ClaimBatch,
		SubscriptionIds: eligible,
	})
	if err != nil {
		return err
	}

	// Group claimed matches by subscription (one digest each) and by recipient (one message per channel).
	claims := make(map[int64]*subscriptionClaim)
	messages := make(map[messageKey][]int64)
	order := make([]messageKey, 0)
	for _, c := range claimed {
		sc, seen := claims[c.SubscriptionID]
		if !seen {
			sc = &subscriptionClaim{oldest: c, personLastSent: c.PersonLastSentAt}
			claims[c.SubscriptionID] = sc
			key := keyOf(c)
			if _, ok := messages[key]; !ok {
				order = append(order, key)
			}
			messages[key] = append(messages[key], c.SubscriptionID)
		}
		sc.add(c)
	}
	for _, sc := range claims {
		sc.full = len(sc.jobIDs) >= int(perSubscription)
	}

	for i, key := range order {
		// A cancelled run stops here: every remaining message would fail against the same dead context
		// and count as a delivery failure, so one SIGTERM would look like an outage. Their claims expire
		// and the next pass retries them.
		if ctx.Err() != nil {
			log.Printf("notify: delivery stopped after %d of %d claimed messages: %v", i, len(order), ctx.Err())
			return nil
		}
		r.deliverMessage(ctx, messages[key], claims, stats)
	}
	return nil
}

// messageKey is who one message goes to: a person on a channel. A webhook is keyed
// by its subscription too, since its receiver reads one saved search per payload.
type messageKey struct {
	userID       int64
	channel      string
	subscription int64
}

func keyOf(c db.ClaimSubscriptionMatchesRow) messageKey {
	return newMessageKey(c.UserID, c.Channel, c.SubscriptionID)
}

func newMessageKey(userID int64, channel string, subscriptionID int64) messageKey {
	k := messageKey{userID: userID, channel: channel}
	if channel == ChannelWebhook {
		k.subscription = subscriptionID
	}
	return k
}

// hold says why a message cannot go out right now; holdNone means it can.
type hold int

const (
	holdNone hold = iota
	// holdDeferred: quiet hours, or a daily digest before its time or already sent today.
	holdDeferred
	// holdNoRecipient: the channel has nowhere to deliver (Telegram unlinked, no push device).
	holdNoRecipient
)

// deliveryHold is the one definition of whether a message to this person on this channel can go
// out at now, and where to. deliver asks it BEFORE leasing, so a held message costs no writes;
// deliverMessage asks again after the lease for the races between the two.
//
// A `daily` subscription waits for its own configured time and is exempt from quiet hours.
// `instant` and `interval` (the default) defer to quiet hours; interval's elapsed-interval gate
// stays in ClaimSubscriptionMatches. personLastSent is when this person was last mailed on this
// channel across ALL their alerts: a sent message stamps every subscription it carried, and a new
// alert waits for tomorrow's digest rather than sending its own.
func deliveryHold(now time.Time, info db.GetSubscriptionForDeliveryRow, personLastSent pgtype.Timestamptz) (dest string, h hold) {
	tz := info.Timezone.String
	if info.DigestFrequency == FrequencyDaily {
		if !deliverywindow.DigestDue(now, tz, pgconv.DurationPtr(info.DigestTime), pgconv.TimePtr(personLastSent)) {
			return "", holdDeferred
		}
	} else if deliverywindow.InQuietHours(now, tz, pgconv.DurationPtr(info.QuietHoursStart), pgconv.DurationPtr(info.QuietHoursEnd)) {
		return "", holdDeferred
	}
	dest, ok := recipient(info)
	if !ok {
		return "", holdNoRecipient
	}
	return dest, holdNone
}

// deliverableSubscriptions lists the subscriptions with pending matches whose message can go out
// now, so only they are leased. Stats count each held message once.
func (r *Runner) deliverableSubscriptions(ctx context.Context, stats *Stats) ([]int64, error) {
	rows, err := r.store.ListDeliveryCandidates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list delivery candidates: %w", err)
	}
	now := r.now()
	eligible := make([]int64, 0, len(rows))
	held := make(map[messageKey]bool)
	for _, row := range rows {
		// Same columns as GetSubscriptionForDelivery: the conversion stops compiling if the
		// two queries drift apart.
		info := db.GetSubscriptionForDeliveryRow(row)
		_, h := deliveryHold(now, info, row.PersonLastSentAt)
		if h == holdNone {
			eligible = append(eligible, row.ID)
			continue
		}
		key := newMessageKey(row.UserID, row.Channel, row.ID)
		if held[key] {
			continue
		}
		held[key] = true
		if h == holdDeferred {
			stats.Deferred++
		} else {
			stats.SoftSkips++
		}
	}
	return eligible, nil
}

// part is one subscription's digest inside a message being assembled.
type part struct {
	subID  int64
	claim  *subscriptionClaim
	info   db.GetSubscriptionForDeliveryRow
	digest Digest
	// covered is how many pending matches the digest counts without carrying.
	covered int
	// createdRow says whether THIS delivery wrote the digest's notification-centre row or joined
	// one another channel had written; only the creator may withdraw it.
	createdRow bool
}

// finalize records one delivered digest (deliverMessage's success half): its history row, its matches, and the
// subscription's last-sent stamp.
func (r *Runner) finalize(ctx context.Context, p *part, stamped bool) {
	// The digest went out. A zero id means nothing in the history names it yet (the pre-send write
	// failed, or conflicted with a row withdrawn before this channel could read it). Record it now:
	// the id is no longer needed, and the history must admit a digest the person received. Safe to
	// attempt blindly: the dedup key makes a redundant insert a no-op.
	if p.digest.NotificationID == 0 {
		r.recordNotification(ctx, p.info, p.digest, p.claim.jobIDs)
	}

	if _, err := r.store.MarkMatchesNotified(ctx, db.MarkMatchesNotifiedParams{
		SubscriptionID: p.subID,
		JobIds:         p.claim.jobIDs,
	}); err != nil {
		// Delivered but not stamped: the lease expiry will re-deliver (a rare
		// duplicate), which is preferable to losing the notification.
		log.Printf("notify: mark notified for subscription %d: %v", p.subID, err)
	}
	if p.covered > 0 {
		if _, err := r.store.MarkCoveredMatchesNotified(ctx, db.MarkCoveredMatchesNotifiedParams(p.claim.coveredParams(p.subID))); err != nil {
			// The digest counted them but they stay pending: the next digest carries the
			// newest of them again or they go stale — a repeat at worst, never a loss.
			log.Printf("notify: mark covered matches notified for subscription %d: %v", p.subID, err)
		}
	}

	// Every subscription the message carried is stamped, which is what the claim's
	// per-person interval gate reads.
	if stamped {
		if err := r.store.MarkDigestSent(ctx, p.subID); err != nil {
			// Delivered but not stamped: the next pass delivers again (a rare
			// duplicate), preferable to skipping the next digest.
			log.Printf("notify: mark digest sent for subscription %d: %v", p.subID, err)
		}
	}
}

func subIDsOf(parts []*part) []int64 {
	ids := make([]int64, len(parts))
	for i, p := range parts {
		ids[i] = p.subID
	}
	return ids
}

// recordNotification writes the digest's in-app notification-center row and returns its id, or
// zero if the write failed: a degraded read side, never a reason to hold back the send.
//
// created reports whether this delivery WROTE the row rather than joining one another channel
// had written. The centre holds one row per EVENT, so only the writer may withdraw it.
func (r *Runner) recordNotification(ctx context.Context, info db.GetSubscriptionForDeliveryRow, d Digest, jobIDs []int64) (id int64, created bool) {
	title, body, slug := renderDigest(d)
	if d.Overflowed() {
		// The row's page lists only the snapshot, under this body; say so, or it
		// reads as a count the list beneath it does not reach.
		body = fmt.Sprintf("%s — the newest %d are listed", body, len(d.Jobs))
	}
	var publicSlug pgtype.Text
	if slug != "" {
		publicSlug = pgconv.TextValid(slug)
	}
	dedupKey := pgconv.TextValid(digestDedupKey(info.SavedSearchID, jobIDs))
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
	// No row inserted: another channel of this saved search already recorded this event. Link this
	// channel's message at that row instead of adding a duplicate to the history.
	id, err = r.store.GetNotificationIDByDedupKey(ctx, db.GetNotificationIDByDedupKeyParams{
		UserID:   info.UserID,
		DedupKey: dedupKey,
	})
	if err != nil {
		// The row was withdrawn between the two statements, which takes a SECOND notify process
		// (deliver() is sequential and systemd will not start a oneshot unit while the first is active),
		// i.e. a hand-run pass overlapping the timer's. The send goes out with a zero id and a generic
		// tail; deliverMessage records again after a successful send because of this branch.
		// Resurrecting a row another worker withdrew is right here: it withdrew because ITS send
		// failed, and this digest was received.
		log.Printf("notify: read deduped notification for subscription %d: %v", info.ID, err)
		return 0, false
	}
	return id, false
}

// withdrawNotification removes the row recordNotification wrote for a digest that then failed to
// send. A failure here is logged and leaves one history row for an undelivered digest, which
// beats dropping a delivery that succeeded.
func (r *Runner) withdrawNotification(ctx context.Context, subID, id int64) {
	if id == 0 {
		return
	}
	if err := r.store.DeleteNotification(ctx, id); err != nil {
		log.Printf("notify: withdraw notification %d for subscription %d: %v", id, subID, err)
	}
}

// unlinkTelegram forgets the user's Telegram chat after a send reported it permanently closed,
// and logs it: a visible settings change the user did not make. A failure to unlink is logged
// only; the digest is soft-skipped either way and the next pass meets the same 403.
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
// the soft-skip in deliverMessage is already the whole response.
func (r *Runner) forgetRecipient(ctx context.Context, subID int64, info db.GetSubscriptionForDeliveryRow, cause error) {
	switch info.Channel {
	case ChannelTelegram:
		r.unlinkTelegram(ctx, subID, info.UserID, cause)
	case ChannelWebhook:
		r.disableWebhook(ctx, subID, info.UserID, cause)
	}
}

// disableWebhook turns off a user's webhook after a send reported it permanently gone (HTTP
// 410): the counterpart of unlinkTelegram, and likewise a visible settings change, so it is logged.
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

// recordWebhookFailure counts a non-410 webhook failure toward the destination's auto-disable
// threshold and logs when it crosses it. The destination never said it was gone; it keeps failing.
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
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := r.store.ReleaseMatchClaim(releaseCtx, db.ReleaseMatchClaimParams{
		SubscriptionID: subID,
		JobIds:         jobIDs,
	}); err != nil {
		log.Printf("notify: release claim for subscription %d: %v", subID, err)
	}
}

// expireBatch bounds one ExpireStaleSubscriptionMatches statement, so a large backlog is a run
// of short statements instead of one long one holding row locks.
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
	// personLastSent is when the subscription's owner was last sent a message on its
	// channel (ClaimSubscriptionMatches) — the same for every subscription of a message.
	personLastSent pgtype.Timestamptz
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
