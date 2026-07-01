// Command recount-companies recomputes each company's denormalized state — the
// open-job count and the facet arrays (regions/countries/domains/company_types/
// company_sizes) derived from its open jobs — in one set-based pass and exits. The
// list endpoint reads and filters on these columns (and orders by job_count, most
// active first) instead of joining jobs on every request. They change both when
// jobs are ingested and when they are closed (closed_at set by the ingest sweep /
// liveness worker), so they are maintained by this periodic recompute rather than a
// write-path trigger — eventually consistent within the cron interval. Idempotent:
// re-running rewrites only the rows whose state actually changed.
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

	updated, err := db.New(pool).RefreshCompanyFacets(ctx)
	if err != nil {
		log.Printf("recount-companies: %v", err)
		return 1
	}
	log.Printf("recount-companies done: companies updated=%d", updated)
	return 0
}
