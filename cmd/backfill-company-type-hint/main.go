// Command backfill-company-type-hint force-requeues the OPEN postings of the companies in
// enrich.CompanyTypeHints into enrichment_outbox, so the next cmd/enrich runs rewrite their
// company_type using the "Known company type" prompt hint instead of waiting for a Version
// bump, which would re-enrich the whole catalogue.
//
// One-off: run it after adding entries to enrich.CompanyTypeHints, then let the cmd/enrich
// cron drain the rows (EnqueueEnrichmentForCompanySlugs is ON CONFLICT-guarded, so re-running
// before the drain costs nothing).
//
// Enrichment writes are not pushed to search_outbox, so follow the drain with a full `make
// reindex`.
//
// Needs DATABASE_URL.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/ai/enrich"
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

	slugs := make([]string, 0, len(enrich.CompanyTypeHints))
	for slug := range enrich.CompanyTypeHints {
		slugs = append(slugs, slug)
	}

	q := db.New(pool)
	n, err := q.EnqueueEnrichmentForCompanySlugs(ctx, db.EnqueueEnrichmentForCompanySlugsParams{
		CompanySlugs:  slugs,
		TargetVersion: int32(enrich.Version),
	})
	if err != nil {
		log.Printf("backfill-company-type-hint: enqueue: %v", err)
		return 1
	}
	log.Printf("backfill-company-type-hint: requeued %d jobs across %d companies for re-enrichment", n, len(slugs))
	return 0
}
