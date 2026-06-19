// Command recount-companies recomputes the denormalized companies.job_count for
// every company in one set-based pass and exits. The list endpoint and the
// sidebar company typeahead read that column and order by it (most active first),
// instead of joining and counting jobs on every request. The count changes both
// when jobs are ingested and when they are closed (closed_at set by the ingest
// sweep / liveness worker), so it is maintained by this periodic recompute rather
// than a write-path trigger — eventually consistent within the cron interval.
// Idempotent: re-running rewrites only the rows whose count actually changed.
package main

import (
	"context"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	updated, err := db.New(pool).RecountCompanyJobCounts(ctx)
	if err != nil {
		log.Printf("recount-companies: %v", err)
		return 1
	}
	log.Printf("recount-companies done: companies updated=%d", updated)
	return 0
}
