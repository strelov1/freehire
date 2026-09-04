// Command billing-sync repairs what webhook delivery lost. Schedule it hourly.
//
// It is not a safety net bolted onto a reliable channel. The provider retries a delivery
// for up to three days and then stops for good, and an endpoint it decides is broken can be
// disabled sooner than that — after either, this worker is the ONLY path by which a paid
// subscription becomes Pro. Two passes:
//
//  1. apply events the webhook recorded but could not apply — the provider was
//     unreachable, the pool was saturated, the process died between the two;
//  2. re-read subscriber state for accounts whose plan expiry is near, catching a renewal
//     whose webhook never arrived at all.
//
// The second pass reaches its candidates through the stored customer binding, so it walks
// the accounts that have actually transacted rather than every account on the site.
//
// Needs DATABASE_URL and the STRIPE_* credentials. Without the credentials it is a no-op
// that never opens the pool.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/strelov1/freehire/internal/identity/billing"
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
	cfg := billing.ConfigFromEnv()
	if !cfg.Enabled() {
		log.Print("billing-sync: billing is not configured, nothing to reconcile")
		return 0
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	svc := billing.New(cfg, db.New(pool))
	max := maxPerRun()

	applied, failed := applyPending(ctx, svc, max)
	refreshed, refreshFailed := refreshNearExpiry(ctx, svc, max)

	log.Printf("billing-sync: applied=%d refreshed=%d failed=%d", applied, refreshed, failed+refreshFailed)
	return worker.ExitCode(failed+refreshFailed, 0)
}

// applyPending is the first pass: events recorded by the webhook but never applied.
//
// One account's failure does not abort the run. The rows are independent, and a provider
// that is refusing one identifier should not stop the other subscriptions from being
// repaired — the failed row simply stays unprocessed for the next hour.
func applyPending(ctx context.Context, svc *billing.Service, max int32) (applied, failed int) {
	pending, err := svc.PendingEvents(ctx, max)
	if err != nil {
		log.Printf("billing-sync: reading pending events: %v", err)
		return 0, 1
	}

	for _, ev := range pending {
		err := svc.Apply(ctx, ev.ID, billing.Event{ID: ev.EventID, CustomerID: ev.AppUserID, Type: ev.EventType})
		switch {
		case err == nil:
			applied++
		case errors.Is(err, billing.ErrUnknownSubscriber), errors.Is(err, billing.ErrNoSubscription):
			// Nobody can ever apply this — an event about something we do not meter, or an
			// object created outside this integration. Stamping it keeps it out of this queue
			// rather than failing the run every hour forever.
			log.Printf("billing-sync: event %s (%s) names no account we meter — stamping it", ev.EventID, ev.EventType)
			if markErr := svc.MarkProcessed(ctx, ev.ID); markErr != nil {
				log.Printf("billing-sync: stamping event %s: %v", ev.EventID, markErr)
				failed++
			}
		default:
			log.Printf("billing-sync: applying event %s: %v", ev.EventID, err)
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
func refreshNearExpiry(ctx context.Context, svc *billing.Service, max int32) (refreshed, failed int) {
	ids, err := svc.SubscribersNearExpiry(ctx, expiryWindow, max)
	if err != nil {
		log.Printf("billing-sync: reading subscribers near expiry: %v", err)
		return 0, 1
	}

	for _, id := range ids {
		if err := svc.SyncUser(ctx, id); err != nil {
			log.Printf("billing-sync: refreshing user %d: %v", id, err)
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
