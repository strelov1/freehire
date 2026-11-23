// Command recount-companies recomputes each company's denormalized state (the open-job count
// and the facet arrays regions/countries/domains/company_types/company_sizes) in one set-based
// pass and exits. The list endpoint reads these columns instead of joining jobs per request.
// They change on ingest and on close, so a periodic recompute maintains them (eventually
// consistent within the cron interval). Idempotent: only changed rows are rewritten.
//
// The jobs scan reads the analytics replica (ANALYTICS_DATABASE_URL, see worker.AnalyticsPool);
// only the companies UPDATE runs on the primary.
package main

import (
	"context"
	"encoding/json"
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
	analytics, err := worker.AnalyticsPool(ctx, pool)
	if err != nil {
		log.Printf("analytics database: %v", err)
		return 1
	}
	defer analytics.Close()
	rows, err := worker.RetryStandbyConflict(ctx, "ComputeCompanyJobFacets", func() ([]db.ComputeCompanyJobFacetsRow, error) {
		return db.New(analytics).ComputeCompanyJobFacets(ctx, db.ComputeCompanyJobFacetsParams{
			UnseenSources: unseenSources,
			UnseenSeconds: unseenSeconds,
		})
	})
	if err != nil {
		log.Printf("recount-companies: compute: %v", err)
		return 1
	}
	// An empty result is a failed measurement (a misconfigured reader, say), not an empty catalogue:
	// applying it would zero every company's job_count and empty the sitemap.
	if len(rows) == 0 {
		log.Print("recount-companies: no company has open jobs; refusing to apply")
		return 1
	}
	facets, err := json.Marshal(rows)
	if err != nil {
		log.Printf("recount-companies: encode: %v", err)
		return 1
	}
	updated, err := db.New(pool).ApplyCompanyFacets(ctx, db.ApplyCompanyFacetsParams{
		Facets:            facets,
		MappingDomains:    mappingDomains,
		MappingIndustries: mappingIndustries,
	})
	if err != nil {
		log.Printf("recount-companies: apply: %v", err)
		return 1
	}
	log.Printf("recount-companies done: companies with open jobs=%d updated=%d", len(rows), updated)
	return 0
}
