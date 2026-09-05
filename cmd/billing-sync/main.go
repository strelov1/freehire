// Command billing-sync repairs what webhook delivery lost. Schedule it hourly.
//
// It is not a safety net bolted onto a reliable channel. The provider retries a delivery
// for up to three days and then stops for good, and an endpoint it decides is broken can be
// disabled sooner than that — after either, this worker is the ONLY path by which a paid
// subscription becomes Pro. Three passes:
//
//  1. apply events the webhook recorded but could not apply — the provider was
//     unreachable, the pool was saturated, the process died between the two;
//  2. re-read subscriber state for accounts whose plan expiry is near, catching a renewal
//     whose webhook never arrived at all;
//  3. settle referral rewards: earn the ones whose invitee has an invoice that actually
//     collected, then place the earned ones on their referrers' balances. Stripe only —
//     a store subscription produces no invoice we can read, so there is nothing to earn
//     a reward from.
//
// The second pass reaches its candidates through what each provider can address them by —
// the stored customer binding for Stripe, the store entitlement column for RevenueCat — so it
// walks the accounts that have actually transacted rather than every account on the site.
//
// BOTH PROVIDERS ARE RECONCILED, each against its own events and its own source column, and
// each independently of the other. One provider being unreachable must not stop the other:
// they are different companies with different outages, and a Stripe incident that also
// stalled App Store renewals would be an outage we invented ourselves.
//
// Needs DATABASE_URL and the credentials of at least one provider. With neither it is a no-op
// that never opens the pool.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/identity/promo"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

const (
	// maxPerRunDefault bounds one run so a backlog cannot turn a Type=oneshot unit into a
	// run that outlives its own timer — systemd will not start a second instance while the
	// first is active, so an unbounded run silently skips firings.
	maxPerRunDefault = 500

	// expiryWindow is how far either side of now the second pass looks. A renewal moves the
	// expiry forward by a month, so anything within a day of lapsing is either about to
	// renew or has just failed to, and both are worth re-reading.
	expiryWindow = 24 * time.Hour
)

func main() { worker.Main(run) }

func run() int {
	// Gate before Bootstrap, not after. With no provider credentials there is nothing to
	// reconcile against, and the spec requires an unconfigured deployment to run without
	// touching the database at all — which is also what keeps this binary harmless in a
	// checkout that is not freehire.me.
	stripeCfg := billing.ConfigFromEnv()
	storeCfg := billing.RevenueCatConfigFromEnv()
	if !stripeCfg.Enabled() && !storeCfg.Enabled() {
		log.Print("billing-sync: no billing provider is configured, nothing to reconcile")
		return 0
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	queries := db.New(pool)
	max := maxPerRun()

	// Named so every log line says which provider it is about. With two of them, "applying
	// event evt_1 failed" without a name is a line somebody has to go and look up.
	passes := []struct {
		name string
		svc  reconcilable
	}{
		{"stripe", billing.New(stripeCfg, queries)},
		{"revenuecat", billing.NewRevenueCat(storeCfg, queries)},
	}

	var failures int
	for _, p := range passes {
		if !p.svc.Enabled() {
			continue
		}
		applied, failed := applyPending(ctx, p.name, p.svc, max)
		refreshed, refreshFailed := refreshNearExpiry(ctx, p.name, p.svc, max)
		failures += failed + refreshFailed

		log.Printf("billing-sync: provider=%s applied=%d refreshed=%d failed=%d",
			p.name, applied, refreshed, failed+refreshFailed)
	}

	// The referral pass rides here rather than in a binary of its own. It needs the same
	// database and the same Stripe credentials, it is the same kind of work — repairing what
	// a webhook could not tell us — and a new binary would need a new systemd unit, which
	// lives only on the production host and is a manual step easy to forget.
	//
	// Stripe only, and not because of an omission: a store subscription produces no invoice
	// we can read, so there is nothing to earn a reward from.
	if stripeCfg.Enabled() {
		failures += settleRewardsLocked(ctx, pool, billing.New(stripeCfg, queries), queries, max)
	}
	return worker.ExitCode(failures, 0)
}

// rewardLockKey serializes the referral pass across processes. Registered in the list in
// internal/platform/migrate; "fhrw" as bytes.
const rewardLockKey int64 = 0x66687277

// settleRewardsLocked runs the referral pass under an advisory lock, or skips it.
//
// The lock is what makes the per-referrer ceiling a BOUND. Reading a count in one statement
// and acting on it in another is not atomic under READ COMMITTED — and neither is a count
// evaluated inside the UPDATE, because the subquery sees the snapshot the statement started
// with, not another transaction's uncommitted work. Two passes over different pending
// rewards of one referrer would each see the count below the ceiling and each grant.
//
// Non-blocking: a second run gives up rather than queueing. This pass is hourly and
// idempotent, so waiting buys nothing and a queued run holds a Type=oneshot unit open,
// which systemd shows as a hang.
func settleRewardsLocked(ctx context.Context, pool *pgxpool.Pool, provider rewardProvider, queries *db.Queries, max int32) int {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("billing-sync: referrals: acquiring a connection: %v", err)
		return 1
	}
	defer conn.Release()

	var held bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", rewardLockKey).Scan(&held); err != nil {
		log.Printf("billing-sync: referrals: taking the lock: %v", err)
		return 1
	}
	if !held {
		log.Print("billing-sync: referrals: another run holds the lock, skipping")
		return 0
	}
	defer func() {
		// context.Background(), because the release must happen even when the run's own
		// context is already cancelled — a lock left held would skip every later pass until
		// the connection is reaped.
		if _, err := conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", rewardLockKey); err != nil {
			log.Printf("billing-sync: referrals: releasing the lock: %v", err)
		}
	}()

	return settleRewards(ctx, provider, queries, max)
}

// rewardProvider is what the referral pass needs of Stripe. Narrow on purpose: this pass
// must not be able to touch a subscription.
type rewardProvider interface {
	CheckoutPriceCents(ctx context.Context) (int64, error)
	HasCollectedAtLeast(ctx context.Context, customerID string, minCents int64) (bool, error)
	CreditAccount(ctx context.Context, userID, cents int64, idempotencyKey string) error
}

// settleRewards is the third pass: earn the referral rewards whose invitee has paid, then
// place the earned ones on their referrers' balances.
//
// Both halves are guarded on the row's own state, so the pass is idempotent and stopping it
// mid-way is free. A failure here is counted but never abandons the run: a referral reward
// arriving an hour late is a smaller harm than a reconciliation that did not happen.
func settleRewards(ctx context.Context, provider rewardProvider, queries *db.Queries, max int32) int {
	svc := promo.New(promo.NewQueriesRepository(queries), promo.ConfigFromEnv())

	// Read once per run rather than per reward. It is one provider call, it cannot change
	// mid-pass, and a failure to read it must stop the pass rather than let it grant at a
	// price it guessed.
	priceCents, err := provider.CheckoutPriceCents(ctx)
	if err != nil {
		log.Printf("billing-sync: referrals: reading the sale price: %v", err)
		return 1
	}

	var failures int
	granted, err := svc.GrantEarned(ctx, max, priceCents, provider)
	if err != nil {
		log.Printf("billing-sync: referrals: granting: %v", err)
		failures++
	}

	delivered, err := svc.DeliverEarned(ctx, max, provider)
	if err != nil {
		log.Printf("billing-sync: referrals: delivering: %v", err)
		failures++
	}

	log.Printf("billing-sync: referrals granted=%d delivered=%d failed=%d", granted, delivered, failures)
	return failures
}

// reconcilable is what this worker needs of a billing provider, and it is exactly the shared
// engine's surface. Both providers satisfy it; nothing here knows which one it is holding
// beyond the name it prints.
type reconcilable interface {
	Enabled() bool
	PendingEvents(ctx context.Context, max int32) ([]db.ListUnprocessedBillingEventsRow, error)
	Apply(ctx context.Context, rowID int64, ev billing.Event) error
	MarkProcessed(ctx context.Context, rowID int64) error
	SubscribersNearExpiry(ctx context.Context, window time.Duration, max int32) ([]int64, error)
	SyncUser(ctx context.Context, userID int64) error
}

// applyPending is the first pass: events recorded by the webhook but never applied.
//
// One account's failure does not abort the run. The rows are independent, and a provider that
// is refusing one identifier should not stop the other subscriptions from being repaired — the
// failed row simply stays unprocessed for the next hour.
func applyPending(ctx context.Context, name string, svc reconcilable, max int32) (applied, failed int) {
	pending, err := svc.PendingEvents(ctx, max)
	if err != nil {
		log.Printf("billing-sync: %s: reading pending events: %v", name, err)
		return 0, 1
	}

	for _, ev := range pending {
		// The stored row already knows whose event this is — it was resolved when the
		// delivery was recorded. Rebuilding the event without it would make the retry
		// WEAKER than the first attempt: with only a customer id, an account whose binding
		// never got written resolves to nobody, and the branch below would stamp a real paid
		// subscription as unattributable. Forever, because a stamped row is never retried.
		replay := billing.Event{ID: ev.EventID, CustomerID: ev.AppUserID, Type: ev.EventType}
		if ev.UserID.Valid {
			replay.UserRef = strconv.FormatInt(ev.UserID.Int64, 10)
		}

		err := svc.Apply(ctx, ev.ID, replay)
		switch {
		case err == nil:
			applied++
		case errors.Is(err, billing.ErrUnknownSubscriber), errors.Is(err, billing.ErrNoSubscription):
			// Nobody can ever apply this — an event about something we do not meter, or an
			// object created outside this integration. Stamping it keeps it out of this queue
			// rather than failing the run every hour forever.
			log.Printf("billing-sync: %s: event %s (%s) names no account we meter — stamping it", name, ev.EventID, ev.EventType)
			if markErr := svc.MarkProcessed(ctx, ev.ID); markErr != nil {
				log.Printf("billing-sync: %s: stamping event %s: %v", name, ev.EventID, markErr)
				failed++
			}
		default:
			log.Printf("billing-sync: %s: applying event %s: %v", name, ev.EventID, err)
			failed++
		}
	}
	return applied, failed
}

// refreshNearExpiry is the second pass: subscriptions about to lapse or just lapsed, whose
// renewal webhook may never have arrived.
//
// It re-reads even for accounts nothing has changed for. That is the point — the whole
// design derives the column WHOLE from provider state, so a sync that finds nothing new
// writes nothing, and the cheapest way to be sure is to ask.
func refreshNearExpiry(ctx context.Context, name string, svc reconcilable, max int32) (refreshed, failed int) {
	ids, err := svc.SubscribersNearExpiry(ctx, expiryWindow, max)
	if err != nil {
		log.Printf("billing-sync: %s: reading subscribers near expiry: %v", name, err)
		return 0, 1
	}

	for _, id := range ids {
		if err := svc.SyncUser(ctx, id); err != nil {
			log.Printf("billing-sync: %s: refreshing user %d: %v", name, id, err)
			failed++
			continue
		}
		refreshed++
	}
	return refreshed, failed
}

// maxPerRun reads the per-run bound. An unreadable value keeps the default and says so: a
// typo resolving to zero would make every run a silent no-op, which looks exactly like a
// backlog that has already been drained.
//
// ParseInt at 32 bits rather than Atoi, because the value ends up as the int32 the query
// takes as its LIMIT. Atoi parses at the platform's int width, so a figure above
// MaxInt32 would not be rejected here — it would WRAP on conversion, and a wrapped
// negative limit is a query error rather than a large batch. Parsing at the width the
// value is actually used at makes an out-of-range figure a rejection, which is what the
// log line below already claims to be doing.
func maxPerRun() int32 {
	raw := os.Getenv("BILLING_SYNC_MAX_PER_RUN")
	if raw == "" {
		return maxPerRunDefault
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n <= 0 {
		log.Printf("billing-sync: BILLING_SYNC_MAX_PER_RUN=%q is not a positive number that fits a batch size — keeping %d", raw, maxPerRunDefault)
		return maxPerRunDefault
	}
	return int32(n)
}
