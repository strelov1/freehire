// Command backfill-preparing-stage is a one-time correction for applications the
// CV-tailoring board placement wrote as stage='applied' with no applied_at, before
// internal/handler/cv.go's EnsureOnBoard was changed to write stage='preparing' directly.
//
// It does not touch every stage='applied'/applied_at=NULL row — that shape is also what a
// candidate's own manual, undated drag into the Applied column produces (see
// JobBoard.svelte's persistMove), and relabeling that as "still preparing" would be wrong in
// the opposite direction. A tailored CV on record for the same (user, job) is the
// corroborating evidence that narrows the correction to tailoring's own rows; see
// internal/db/queries/applications.sql's BackfillPreparingStage for the exact shape.
//
// Idempotent: a second run finds nothing, because a corrected row no longer reads
// stage='applied'. Safe to run before or after the code deploy that stopped writing the old
// shape — the WHERE clause only ever matches what that OLD code produced.
//
//	go run ./cmd/backfill-preparing-stage             # apply the correction
//	go run ./cmd/backfill-preparing-stage --dry-run   # report how many rows, without writing
package main

import (
	"context"
	"flag"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	var dryRun bool
	flag.BoolVar(&dryRun, "dry-run", false, "report how many applications would be corrected, without writing")
	flag.Parse()

	worker.Main(func() int { return run(dryRun) })
}

func run(dryRun bool) int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	queries := db.New(pool)

	if dryRun {
		count, err := queries.CountPreparingBackfillCandidates(ctx)
		if err != nil {
			log.Printf("count candidates: %v", err)
			return 1
		}
		log.Printf("backfill-preparing-stage: %d application(s) would be corrected (dry-run)", count)
		return 0
	}

	count, err := queries.BackfillPreparingStage(ctx)
	if err != nil {
		log.Printf("backfill: %v", err)
		return 1
	}
	log.Printf("backfill-preparing-stage: done — %d application(s) corrected", count)
	return 0
}
