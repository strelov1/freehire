// Command merge-companies collapses the company slugs that are one employer written more
// than one way, and records each retirement so the old URL keeps working.
//
// It reports by default and writes only under --apply, like cmd/prune: the election is
// deterministic but the catalogue is not clean, so a wave is meant to be read before it runs.
// --min-jobs bounds a wave to the groups big enough to matter, which is how the backlog is
// taken in reviewed passes (1000, then 100, then 10, then 1) instead of one 333k-row leap.
//
// It does NOT touch the search index. A push to the facet index costs 90-180s regardless of
// batch size, so feeding a wave through search_outbox would be tens of hours of pushes and the
// shape that took the site down on 2026-08-05. The scheduled reindex picks the re-key up; until
// it does, a merged company under-counts its jobs for a few hours and nothing 404s.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	worker.Main(run)
}

// rekeyChunk is how many job rows one UPDATE moves. Small because one company can carry
// twenty thousand open jobs (dollar-tree does), and each statement is a transaction against
// the hottest table in the schema — a wave should be interruptible at any moment, not held
// open while it rewrites a large employer whole.
const rekeyChunk = 500

func run() int {
	apply := flag.Bool("apply", false, "actually merge; without it the run only reports the plan")
	minJobs := flag.Int("min-jobs", 0, "only merge folded groups whose combined open jobs reach this")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)
	plan, err := loadPlan(ctx, q, *minJobs)
	if err != nil {
		log.Printf("merge-companies: plan: %v", err)
		return 1
	}
	report(plan)
	if !*apply {
		log.Printf("merge-companies: dry run, nothing written. Re-run with --apply to merge.")
		return 0
	}

	moved, err := applyMerges(ctx, &store{q: q}, plan, rekeyChunk)
	log.Printf("merge-companies: moved %d job rows across %d groups", moved, len(plan))
	if err != nil {
		log.Printf("merge-companies: %v", err)
		return 1
	}
	log.Printf("merge-companies: done. The scheduled reindex will refresh the search facet; " +
		"do NOT run one by hand.")
	return 0
}

// loadPlan reads the catalogue and the frozen canons and returns the wave to perform.
//
// The whole company list comes back in one read — ~153k rows on prod, a few megabytes — because
// the grouping key is normalize.CompanyKey, a repeating legal-form strip. Reproducing that in
// SQL would be a second implementation of the one rule this change exists to stop duplicating.
func loadPlan(ctx context.Context, q *db.Queries, minJobs int) ([]merge, error) {
	rows, err := q.ListCompaniesForMerge(ctx)
	if err != nil {
		return nil, err
	}
	companies := make([]company, 0, len(rows))
	for _, r := range rows {
		companies = append(companies, company{Slug: r.Slug, Name: r.Name, JobCount: int(r.JobCount)})
	}
	canonical, err := q.ListCanonicalCompanySlugs(ctx)
	if err != nil {
		return nil, err
	}
	frozen := make(map[string]bool, len(canonical))
	for _, slug := range canonical {
		frozen[slug] = true
	}
	return planMerges(companies, frozen, minJobs), nil
}

// report prints the wave. It prints every group rather than a sample: a wave is bounded by
// --min-jobs precisely so that the whole of it can be read, and a truncated report would let
// a bad merge through in the part nobody saw.
func report(plan []merge) {
	var jobs, aliases int
	for _, m := range plan {
		for _, a := range m.Aliases {
			jobs += a.JobCount
			aliases++
			log.Printf("  %s <- %s (%s, %d jobs)", m.Canonical, a.Slug, a.Reason, a.JobCount)
		}
	}
	log.Printf("merge-companies: %d groups, %d slugs retiring, %d job rows to move",
		len(plan), aliases, jobs)
}

// store is the writer applyMerges drives, over the real queries.
type store struct{ q *db.Queries }

func (s *store) InsertAlias(ctx context.Context, a alias, canonical, foldedKey string) error {
	_, err := s.q.InsertCompanySlugAlias(ctx, db.InsertCompanySlugAliasParams{
		AliasSlug:     a.Slug,
		CanonicalSlug: canonical,
		FoldedKey:     foldedKey,
		Reason:        a.Reason,
	})
	return err
}

func (s *store) RekeyChunk(ctx context.Context, aliasSlug, canonical string, chunk int) (int64, error) {
	return s.q.RekeyCompanySlugChunk(ctx, db.RekeyCompanySlugChunkParams{
		CanonicalSlug: canonical,
		AliasSlug:     aliasSlug,
		ChunkSize:     int32(chunk),
	})
}
