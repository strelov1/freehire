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

	"github.com/strelov1/freehire/internal/dict/industrytag"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
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
	// The count covers what search holds, so it takes search's unseen rule too: a posting
	// its source stopped showing is withheld from the index (search.Withheld) and must not
	// keep its employer at a number the company page contradicts.
	var unseenSources []string
	var unseenSeconds []int64
	for source, window := range sources.UnseenWindows(sources.Taxonomy(), search.UnseenWindow) {
		unseenSources = append(unseenSources, source)
		unseenSeconds = append(unseenSeconds, int64(window.Seconds()))
	}
	q := db.New(pool)
	kept, err := keptUncategorizedIDs(ctx, q)
	if err != nil {
		log.Printf("recount-companies: list uncategorized titles: %v", err)
		return 1
	}
	updated, err := q.RefreshCompanyFacets(ctx, db.RefreshCompanyFacetsParams{
		MappingDomains:    mappingDomains,
		MappingIndustries: mappingIndustries,
		UnseenSources:     unseenSources,
		UnseenSeconds:     unseenSeconds,
		KeptIds:           kept,
		EmployerSource:    job.EmployerSource,
	})
	if err != nil {
		log.Printf("recount-companies: %v", err)
		return 1
	}
	log.Printf("recount-companies done: companies updated=%d kept_uncategorized=%d", updated, len(kept))
	return 0
}

const uncategorizedPage = 50000

type uncategorizedLister interface {
	ListUncategorizedOpenJobTitles(context.Context, db.ListUncategorizedOpenJobTitlesParams) ([]db.ListUncategorizedOpenJobTitlesRow, error)
}

// keptUncategorizedIDs pages through the postings the recount's category predicate drops and
// returns those whose title search keeps anyway (search.TitleKeepsUncategorized), so a
// company hiring only for such roles is counted and indexed (#4503).
func keptUncategorizedIDs(ctx context.Context, q uncategorizedLister) ([]int64, error) {
	var kept []int64
	var after int64
	for {
		rows, err := q.ListUncategorizedOpenJobTitles(ctx, db.ListUncategorizedOpenJobTitlesParams{AfterID: after, BatchSize: uncategorizedPage})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if search.TitleKeepsUncategorized(r.Title) {
				kept = append(kept, r.ID)
			}
		}
		if len(rows) < uncategorizedPage {
			return kept, nil
		}
		after = rows[len(rows)-1].ID
	}
}
