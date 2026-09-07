// Command discord-sync keeps the paid role on the community Discord server in step with the
// subscription. Schedule it hourly.
//
// For every linked account it resolves the tier the ordinary way — plan.TierOf over
// users.pro_until and users.ultra_until — and grants or revokes one role accordingly. It
// never asks a payment provider: those columns are the only answer that accounts for Stripe,
// the app stores and granted promo time alike, and a worker reading Stripe directly would
// deny access to every App Store subscriber.
//
// WHY THIS IS NOT A PASS INSIDE cmd/billing-sync, which reconciles the same subscriptions on
// the same schedule: the layering guard forbids it. billing is in the identity block (layer
// 3) and this is outbound community engagement in engage (layer 7), and layer 3 may not
// import layer 7. It is also the separation billing-sync's own doc comment argues for
// between providers — a Discord outage must not stall payment reconciliation.
//
// A per-account failure is counted and stepped over, never fatal: one broken account must
// not cost everybody else their reconciliation. The run reports a non-zero exit only when it
// could not read the work at all.
//
// Needs DATABASE_URL and the five DISCORD_* values. Without them it is a no-op that never
// opens the pool — which is how this ships before the Discord application exists, and what
// rolling it back looks like.
package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/strelov1/freehire/internal/engage/discordlink"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// maxPerRunDefault bounds one run so a backlog cannot turn a Type=oneshot unit into a run
// that outlives its own timer — systemd will not start a second instance while the first is
// active, so an unbounded run silently skips firings.
//
// Nothing is lost to the bound: the reconciliation page is ordered by least-recently-synced,
// so a run that cannot reach everybody leaves the rest at the front of the queue for the
// next hour.
const maxPerRunDefault = 500

func main() { worker.Main(run) }

func run() int {
	// Gate before Bootstrap, not after: with no Discord application there is nothing to
	// reconcile, and an unconfigured deployment must run without touching the database at
	// all. The predicate is config's, so this worker, the routes and the SPA cannot
	// disagree about whether the feature exists.
	cfg := config.Load()
	if !cfg.DiscordPaidAccessConfigured() {
		log.Print("discord-sync: Discord paid channels are not configured, nothing to reconcile")
		return 0
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// The gate above already refused an unconfigured deployment, so this is necessarily true.
	// Checked anyway rather than discarded: the day the two conditions drift apart, a
	// discarded bool is a nil dereference and a checked one is a clean exit.
	svc, ok := discordlink.NewFromSettings(cfg, db.New(pool))
	if !ok {
		log.Print("discord-sync: the configuration gate and the constructor disagree — nothing done")
		return 1
	}

	stats, err := svc.Sync(ctx, maxPerRun())
	if err != nil {
		log.Printf("discord-sync: %v", err)
		return 1
	}
	log.Printf("discord-sync: examined=%d granted=%d revoked=%d failed=%d",
		stats.Examined, stats.Granted, stats.Revoked, stats.Failed)
	return 0
}

// maxPerRun reads the per-run bound.
//
// A set-but-unreadable value falls back and says so, rather than failing the run. That is
// the opposite of what the one-off backfill passes do, and deliberately: those run once
// under an operator's eye, where a typo taking the default silently is the outcome nobody
// notices. This one runs every hour unattended, where failing hard would stop reconciling
// everybody's role for as long as nobody reads systemctl — a far worse trade for a typo in a
// batch size. cmd/billing-sync, this worker's closest sibling, makes the same choice.
func maxPerRun() int32 {
	raw := os.Getenv("DISCORD_SYNC_MAX_PER_RUN")
	if raw == "" {
		return maxPerRunDefault
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n <= 0 {
		log.Printf("discord-sync: DISCORD_SYNC_MAX_PER_RUN=%q is not a positive number that fits a batch size — keeping %d", raw, maxPerRunDefault)
		return maxPerRunDefault
	}
	return int32(n)
}
