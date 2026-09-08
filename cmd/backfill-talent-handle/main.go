// Command backfill-talent-handle is a one-off: mints the catalogue handle for the Talent
// Network members who joined before handles existed.
//
// Migration 0148 rewrote every 'public' row to 'anonymous', and migration 0149 added the
// column — but neither could mint. A handle's readable part is the category a job title
// resolves to through internal/dict/classify, which is a Go dictionary and not something
// SQL can reach. So those accounts, and any that were already 'anonymous', came out of
// the migration as members with no public address: ListTalentNetworkMembers requires
// `talent_handle IS NOT NULL`, so they are absent from the catalogue and their card 404s,
// until they happen to toggle the control again.
//
// It walks the whole list rather than chunking: the population is bounded by how many
// people opted in while the control was unreachable from the account navigation, which is
// small. Idempotent — the claim is `talent_handle IS NULL`-guarded, so a re-run mints
// nothing and stopping it mid-way is free.
//
// **Run it AFTER the deploy**, not before: it calls the same talentnetwork.Join the
// handler does, so the code has to be live. Nothing needs a reindex afterwards — the
// catalogue is served from Postgres, not from Meilisearch.
//
// Needs only DATABASE_URL.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	queries := db.New(pool)

	ids, err := queries.ListMembersMissingTalentHandle(ctx)
	if err != nil {
		log.Printf("backfill-talent-handle: listing members: %v", err)
		return 1
	}
	if len(ids) == 0 {
		log.Print("backfill-talent-handle: every member already has a handle")
		return 0
	}

	var minted, failed int
	for _, id := range ids {
		// A per-account failure is counted and stepped over rather than ending the run:
		// one unreadable stored CV must not leave the rest of the membership invisible,
		// and the guard makes a re-run cost nothing for the ones already done.
		if err := talentnetwork.Join(ctx, queries, id); err != nil {
			log.Printf("backfill-talent-handle: user %d: %v", id, err)
			failed++
			continue
		}
		minted++
	}

	log.Printf("backfill-talent-handle: minted %d of %d, %d failed", minted, len(ids), failed)
	if failed > 0 {
		return 1
	}
	return 0
}
