// Command recount-companies recomputes each company's denormalized state (the open-job count
// and the facet arrays regions/countries/domains/company_types/company_sizes) in one set-based
// pass and exits. The list endpoint reads these columns instead of joining jobs per request.
// They change on ingest and on close, so a periodic recompute maintains them (eventually
// consistent within the cron interval). Idempotent: only changed rows are rewritten.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/dict/industrytag"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

func main() {
	worker.Main(run)
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	mappingDomains, mappingIndustries := industrytag.DomainIndustryPairs()
	// The count covers what search holds, so it takes search's unseen rule too: a posting withheld
	// from the index (search.Withheld) must not inflate its employer's count.
	var unseenSources []string
	var unseenSeconds []int64
	for source, window := range sources.UnseenWindows(sources.Taxonomy(), search.UnseenWindow) {
		unseenSources = append(unseenSources, source)
		unseenSeconds = append(unseenSeconds, int64(window.Seconds()))
	}
	q := db.New(pool)
	updated, err := q.RefreshCompanyFacets(ctx, db.RefreshCompanyFacetsParams{
		MappingDomains:    mappingDomains,
		MappingIndustries: mappingIndustries,
		UnseenSources:     unseenSources,
		UnseenSeconds:     unseenSeconds,
	})
	if err != nil {
		log.Printf("recount-companies: %v", err)
		return 1
	}
	log.Printf("recount-companies done: companies updated=%d", updated)
	return 0
}
