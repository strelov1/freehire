// Command backfill-company-type-hint force-requeues the existing OPEN postings of the
// companies in enrich.CompanyTypeHints back into enrichment_outbox, so the next
// cmd/enrich runs rewrite their company_type using the "Known company type" prompt hint
// instead of waiting for the next Version bump — which would re-enrich the whole
// catalogue just to reach a handful of companies.
//
// One-off: run it once after adding entries to enrich.CompanyTypeHints, then let the
// normal cmd/enrich cron drain the requeued rows (EnqueueEnrichmentForCompanySlugs is
// ON CONFLICT-guarded, so re-running this before that drain completes costs nothing).
//
// Enrichment writes are not pushed to search_outbox, so the corrected values only reach
// Meilisearch on the next full rebuild — follow the drain with a full `make reindex`,
// the same gap cmd/backfill-clearance documents.
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
