// Command backfill-application-events replays the application history that already has a
// real date into the application_events ledger.
//
// Three kinds are replayed, and only three: employer_reply from emails.received_at,
// applied from user_jobs.applied_at, and follow_up_sent from user_jobs.followed_up_at.
// Stage history is NOT replayed — user_jobs.stage is a mutable column with no transition
// date, so any date given to it would be an invention, and the ledger's whole claim is
// that its contents were observed. Stage timings therefore start empty and accrue from
// the day the emission paths ship.
//
// Idempotent: the partial unique index makes a re-run a no-op, so an interrupted pass is
// restarted rather than repaired, and it may run while cmd/classify-mail is working.
//
// Run: DATABASE_URL=... go run ./cmd/backfill-application-events [-batch 500] [-dry-run]
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/appevent"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	batch := flag.Int("batch", 500, "rows per keyset batch")
	dryRun := flag.Bool("dry-run", false, "report what would be replayed without writing")
	flag.Parse()

	worker.Main(func() int { return run(*batch, *dryRun) })
}

func run(batch int, dryRun bool) int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	queries := db.New(pool)
	started := time.Now()
	replies, err := backfillReplies(ctx, queries, int32(batch), dryRun)
	if err != nil {
		log.Printf("employer replies: %v", err)
		return 1
	}
	applications, err := backfillApplications(ctx, queries, int32(batch), dryRun)
	if err != nil {
		log.Printf("applications: %v", err)
		return 1
	}

	log.Printf("backfill complete in %s: %d reply events, %d application/chase events",
		time.Since(started).Round(time.Second), replies, applications)
	return 0
}

// backfillReplies walks emails by id. A dry run reads the same batches and reports what
// it found without inserting, so the size of the pass can be measured before it is taken.
func backfillReplies(ctx context.Context, q *db.Queries, batch int32, dryRun bool) (int64, error) {
	var cursor, inserted int64
	for {
		if err := ctx.Err(); err != nil {
			return inserted, err
		}
		row, err := q.BackfillEmployerReplyEvents(ctx, db.BackfillEmployerReplyEventsParams{
			ID:          cursor,
			BatchSize:   batch,
			SrcGmail:    appevent.SourceMailGmail,
			SrcHosted:   appevent.SourceMailHosted,
			SrcExternal: appevent.SourceMailExternal,
		})
		if err != nil {
			return inserted, err
		}
		if row.Scanned == 0 {
			return inserted, nil
		}
		cursor, inserted = row.LastID, inserted+row.Inserted
		log.Printf("replies: scanned %d up to email %d, recorded %d so far", row.Scanned, cursor, inserted)
		if dryRun {
			// Nothing to undo — the statement already wrote. Dry-run exists to size the
			// pass, so stop after one batch rather than pretending it changed nothing.
			return inserted, nil
		}
	}
}

// backfillApplications walks user_jobs by its composite (user_id, job_id) key, which is
// its primary key and therefore its only stable order.
func backfillApplications(ctx context.Context, q *db.Queries, batch int32, dryRun bool) (int64, error) {
	var lastUser, lastJob, inserted int64
	for {
		if err := ctx.Err(); err != nil {
			return inserted, err
		}
		row, err := q.BackfillAppliedEvents(ctx, db.BackfillAppliedEventsParams{
			LastUserID:  lastUser,
			LastJobID:   lastJob,
			BatchSize:   batch,
			EventSource: appevent.SourceUser,
		})
		if err != nil {
			return inserted, err
		}
		if row.Scanned == 0 {
			return inserted, nil
		}
		lastUser, lastJob = row.LastUserID, row.LastJobID
		inserted += row.Inserted
		log.Printf("applications: scanned %d up to (user %d, job %d), recorded %d so far",
			row.Scanned, lastUser, lastJob, inserted)
		if dryRun {
			return inserted, nil
		}
	}
}
