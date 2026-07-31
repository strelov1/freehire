// Command backfill-applications repairs application links on facts already recorded:
// ledger events and mail that name a posting but not the application it belongs to.
//
// Its first pass — carrying user_jobs rows into applications — was retired with the columns
// it read. It ran once on production (240 applications) and cannot run again, because
// user_jobs no longer holds an application. Every write path now sets application_id at the
// time it links, so this is a repair tool rather than a migration.
//
// Three passes, and the order between them is load-bearing: the applications must exist
// before anything can be attached to them.
//
//  1. user_jobs rows with applied_at set become applications, taking the employer and role
//     title from the posting while it is still there.
//  2. application_events written before this change find their application.
//  3. emails linked to a posting find it too.
//
// Interactions that were only viewed or saved are skipped: there is no application in
// them, and inventing one would put a date on something that never happened.
//
// Run it AFTER cmd/backfill-application-events, so the events that pass replays are
// themselves attached here rather than left for a second sweep.
//
// Idempotent: the partial unique index makes the carry-over a no-op on a re-run, and both
// link passes skip rows that already name an application. An interrupted run is restarted
// rather than repaired.
//
// Run: DATABASE_URL=... go run ./cmd/backfill-applications [-batch 500]
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	batch := flag.Int("batch", 500, "rows per keyset batch")
	flag.Parse()

	worker.Main(func() int { return run(*batch) })
}

func run(batch int) int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	queries := db.New(pool)
	started := time.Now()

	events, err := drain(ctx, "events", func() (int64, error) {
		return queries.BackfillApplicationEventLinks(ctx, int32(batch))
	})
	if err != nil {
		log.Printf("event links: %v", err)
		return 1
	}
	mail, err := drain(ctx, "mail", func() (int64, error) {
		return queries.BackfillEmailApplicationLinks(ctx, int32(batch))
	})
	if err != nil {
		log.Printf("mail links: %v", err)
		return 1
	}

	log.Printf("repair complete in %s: %d events and %d messages attached",
		time.Since(started).Round(time.Second), events, mail)
	return 0
}

// drain repeats a batched update until it stops moving rows. Both link passes select the
// rows that still need attaching, so each call shrinks the remaining set and a zero means
// there is nothing left rather than that the cursor ran out.
func drain(ctx context.Context, what string, step func() (int64, error)) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := step()
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, nil
		}
		total += n
		log.Printf("%s: attached %d so far", what, total)
	}
}
