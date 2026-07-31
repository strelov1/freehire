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
// -dry-run runs the real statements in a transaction and rolls it back, so it reports
// exactly what the pass would record and leaves nothing behind.
//
// Run: DATABASE_URL=... go run ./cmd/backfill-application-events [-batch 500] [-dry-run]
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/jackc/pgx/v5"

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

	// A dry run executes the real statements inside a transaction and rolls it back.
	//
	// The obvious alternative — a second pair of counting queries — duplicates the batch
	// predicates, and two copies of a predicate drift: the sizing run would eventually
	// report a number the real pass does not produce. This way the count IS the real
	// pass, measured and then discarded.
	//
	// The trade is one open transaction for the length of the walk. That is fine for a
	// sizing run over a table this size and would not be for a long one: an open
	// transaction holds its snapshot, and prod has been taken down before by a long read
	// meeting DDL. Size first, then decide.
	var tx pgx.Tx
	if dryRun {
		if tx, err = pool.Begin(ctx); err != nil {
			log.Printf("dry run: begin: %v", err)
			return 1
		}
		// Rollback, never commit: the whole point is that nothing survives.
		defer func() { _ = tx.Rollback(ctx) }()
		queries = queries.WithTx(tx)
	}

	replies, err := backfillReplies(ctx, queries, int32(batch))
	if err != nil {
		log.Printf("employer replies: %v", err)
		return 1
	}
	applications, err := backfillApplications(ctx, queries, int32(batch))
	if err != nil {
		log.Printf("applications: %v", err)
		return 1
	}

	outcome, verb := "complete", "recorded"
	if dryRun {
		outcome, verb = "dry run", "would record"
	}
	log.Printf("backfill %s in %s: %s %d reply events and %d application/chase events",
		outcome, time.Since(started).Round(time.Second), verb, replies, applications)
	return 0
}

// backfillReplies walks emails by id.
func backfillReplies(ctx context.Context, q *db.Queries, batch int32) (int64, error) {
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
	}
}

// backfillApplications walks user_jobs by its composite (user_id, job_id) key, which is
// its primary key and therefore its only stable order.
func backfillApplications(ctx context.Context, q *db.Queries, batch int32) (int64, error) {
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
	}
}
