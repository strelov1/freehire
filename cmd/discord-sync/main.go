// Command discord-sync keeps the paid role on the community Discord server in step with the
// subscription. Schedule it hourly.
//
// For every linked account it resolves the tier via plan.TierOf over users.pro_until and
// users.ultra_until and grants or revokes one role. It never asks a payment provider: those
// columns are the only answer that covers Stripe, app stores and promo time alike, and reading
// Stripe would deny every App Store subscriber.
//
// It is not a pass inside cmd/billing-sync: billing is in identity (layer 3), this is engage
// (layer 7), and layer 3 may not import layer 7. It also keeps a Discord outage from stalling
// payment reconciliation.
//
// A per-account failure is counted and stepped over; the run exits non-zero only when it could
// not read the work at all.
//
// Needs DATABASE_URL and the five DISCORD_* values. Without them it is a no-op that never opens
// the pool.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/engage/discordlink"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// maxPerRunDefault bounds one run so a backlog cannot outlive its timer (systemd will not start
// a second instance while one is active, so an unbounded run skips firings). Nothing is lost:
// pages are ordered least-recently-synced, so the rest lead the next run.
const maxPerRunDefault = 500

func main() { worker.Main(run) }

func run() int {
	// Gate before Bootstrap: with no Discord application the worker must not touch the database.
	// The predicate is config's, so the worker, routes and SPA agree.
	cfg := config.Load()
	if !cfg.DiscordPaidAccessConfigured() {
		log.Print("discord-sync: Discord paid channels are not configured, nothing to reconcile")
		return worker.Skip("DISCORD_CLIENT_ID+DISCORD_CLIENT_SECRET+DISCORD_BOT_TOKEN+DISCORD_GUILD_ID+DISCORD_PAID_ROLE_ID")
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// The gate above guarantees this is true; checked anyway so drift becomes a clean exit, not a
	// nil dereference.
	svc, ok := discordlink.NewFromSettings(cfg, db.New(pool))
	if !ok {
		log.Print("discord-sync: the configuration gate and the constructor disagree — nothing done")
		return 1
	}

	stats, err := svc.Sync(ctx, worker.EnvInt32OrDefault("DISCORD_SYNC_MAX_PER_RUN", maxPerRunDefault))
	if err != nil {
		log.Printf("discord-sync: %v", err)
		return 1
	}
	log.Printf("discord-sync: examined=%d granted=%d revoked=%d failed=%d",
		stats.Examined, stats.Granted, stats.Revoked, stats.Failed)
	return 0
}
