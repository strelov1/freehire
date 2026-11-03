// Command merge-companies collapses the company slugs that are one employer written more
// than one way, and records each retirement so the old URL keeps working.
//
// It reports by default and writes only under --apply: the election is
// deterministic but the catalogue is not clean, so a wave is meant to be read before it runs.
// --min-jobs bounds a wave to the groups big enough to matter, which is how the backlog is
// taken in reviewed passes (1000, then 100, then 10, then 1) instead of one 333k-row leap.
//
// Most groups it finds by folding the NAME, which covers the two duplicate classes the rule
// can reach. The pairs it cannot — one employer whose boards disagree on the name itself, like
// "Exadel" beside "Exadel Inc (Website)" — come from the hand-written list in curated.go, and
// that file argues why they are a list and not another rule.
//
// It does NOT touch the search index. A push to the facet index costs 90-180s regardless of
// batch size, so feeding a wave through search_outbox would be tens of hours of pushes and the
// shape that took the site down on 2026-08-05. The scheduled reindex picks the re-key up; until
// it does, a merged company under-counts its jobs for a few hours and nothing 404s.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/job/companymerge"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	apply := flag.Bool("apply", false, "actually merge; without it the run only reports the plan")
	minJobs := flag.Int("min-jobs", 0, "only merge folded groups whose combined open jobs reach this")
	reason := flag.String("reason", "", "only merge groups whose every alias retires for this reason ("+
		reasonLegalForm+" or "+reasonSpelling+"); empty takes both")
	onlySlugsFile := flag.String("only-slugs", "", "only merge groups holding a slug listed in this file, one per line")
	exactSlugsFile := flag.String("exact-slugs", "", "plan over exactly the slugs listed in this file, one per line, reading only their rows; "+
		"the list must name every slug of each group it takes")
	flag.Parse()
	if *onlySlugsFile != "" && *exactSlugsFile != "" {
		log.Printf("merge-companies: --only-slugs and --exact-slugs are alternatives")
		return 2
	}
	var exact []string
	if *exactSlugsFile != "" {
		slugs, err := readSlugList(*exactSlugsFile)
		if err != nil {
			log.Printf("merge-companies: --exact-slugs: %v", err)
			return 2
		}
		exact = slices.Sorted(maps.Keys(slugs))
	}
	if *reason != "" && *reason != reasonLegalForm && *reason != reasonSpelling {
		log.Printf("merge-companies: unknown --reason %q", *reason)
		return 2
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)
	plan, err := loadPlan(ctx, q, *minJobs, exact)
	if err != nil {
		log.Printf("merge-companies: plan: %v", err)
		return 1
	}
	plan = onlyReason(plan, *reason)
	if *onlySlugsFile != "" {
		slugs, err := readSlugList(*onlySlugsFile)
		if err != nil {
			log.Printf("merge-companies: --only-slugs: %v", err)
			return 2
		}
		plan = onlySlugs(plan, slugs)
	}
	report(plan)
	if !*apply {
		log.Printf("merge-companies: dry run, nothing written. Re-run with --apply to merge.")
		return 0
	}

	moved, err := applyMerges(ctx, &store{q: q}, plan, companymerge.Chunk)
	log.Printf("merge-companies: moved %d job rows across %d groups", moved, len(plan))
	if err != nil {
		log.Printf("merge-companies: %v", err)
		return 1
	}

	// The jobs now carry the canonical slug; give each canonical a company row and sweep the
	// rows the re-key orphaned.
	//
	// This is not optional bookkeeping. Until the orphan row is gone, GET /companies/<retired>
	// still FINDS a company — one with no jobs left — so it answers 200 instead of falling
	// through to the alias lookup and the 301. The merge would look done and the redirect,
	// which is the whole reason the alias row exists, would not be running.
	//
	// Scoped to the alias registry, NOT the full SyncCompaniesFromJobs + DeleteOrphanCompanies
	// this used to run. That pair costs a whole-table pass whatever the wave's size, and on
	// 2026-10-28 a 20-group run held its transaction for 56 minutes while every ingest
	// UpsertJob queued behind it. The registry scope also heals a run killed before this step:
	// its drained slugs are no longer in any plan, but they are in the registry.
	created, orphans, err := companymerge.Reconcile(ctx, q)
	if err != nil {
		log.Printf("merge-companies: %v", err)
		return 1
	}
	log.Printf("merge-companies: reconciled the catalogue, %d canonical rows created, %d retired rows removed",
		created, orphans)

	log.Printf("merge-companies: done. Job counts stay stale until cmd/recount-companies runs, " +
		"and the search facet until the scheduled reindex — do NOT run a reindex by hand.")
	return 0
}

// loadPlan reads the catalogue and the frozen canons and returns the wave to perform.
//
// The whole company list comes back in one read — ~153k rows on prod, a few megabytes — because
// the grouping key is normalize.CompanyKey, a repeating legal-form strip. Reproducing that in
// SQL would be a second implementation of the one rule this change exists to stop duplicating.
//
// exact, when set, replaces that read with one over only the named slugs. It is not a filter
// over the same plan, as --only-slugs is: the election sees only these slugs, so a list that
// leaves out a member of a group leaves that member unmerged and may elect another canon.
func loadPlan(ctx context.Context, q *db.Queries, minJobs int, exact []string) ([]merge, error) {
	companies, err := readCompanies(ctx, q, exact)
	if err != nil {
		return nil, err
	}
	canonical, err := q.ListCanonicalCompanySlugs(ctx)
	if err != nil {
		return nil, err
	}
	frozen := make(map[string]bool, len(canonical))
	for _, slug := range canonical {
		frozen[slug] = true
	}
	return planMerges(companies, frozen, minJobs, curatedAliases), nil
}

func readCompanies(ctx context.Context, q *db.Queries, exact []string) ([]company, error) {
	if len(exact) > 0 {
		rows, err := q.ListCompaniesForMergeBySlugs(ctx, exact)
		if err != nil {
			return nil, err
		}
		out := make([]company, 0, len(rows))
		for _, r := range rows {
			out = append(out, company{Slug: r.Slug, Name: r.Name, JobCount: int(r.JobCount)})
		}
		return out, nil
	}
	rows, err := q.ListCompaniesForMerge(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]company, 0, len(rows))
	for _, r := range rows {
		out = append(out, company{Slug: r.Slug, Name: r.Name, JobCount: int(r.JobCount)})
	}
	return out, nil
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
	// OPEN jobs, because that is what companies.job_count holds and what elects a winner.
	// The re-key moves closed rows too — it has to, or a closed posting would keep the
	// retired slug and resurrect the duplicate the day it reopens — so the number of rows
	// actually written is larger, on prod about 3x.
	log.Printf("merge-companies: %d groups, %d slugs retiring, %d open jobs (closed rows move too)",
		len(plan), aliases, jobs)
}

// store is the writer applyMerges drives, over the real queries.
type store struct{ q *db.Queries }

func (s *store) InsertAlias(ctx context.Context, a alias, canonical string) error {
	return companymerge.Retire(ctx, s.q, a.Slug, canonical, a.FoldedKey, a.Reason)
}

func (s *store) RekeyChunk(ctx context.Context, aliasSlug, canonical string, chunk int) (int64, error) {
	return companymerge.Rekey(s.q)(ctx, aliasSlug, canonical, chunk)
}

// readSlugList reads one slug per line, skipping blank lines. An empty list is refused: a
// filter that admits nothing would report an empty wave, which reads exactly like a list
// whose groups have all been merged already.
func readSlugList(path string) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	slugs := make(map[string]bool)
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			slugs[line] = true
		}
	}
	if len(slugs) == 0 {
		return nil, errors.New(path + " lists no slugs")
	}
	return slugs, nil
}
