// Command rollup-facets is the standalone daily facet-distribution rollup worker.
// It recomputes insights_facet_stats — the value→count distribution for the four
// facets the /open transparency page renders (countries, skills, seniority,
// work_mode) — and swaps it in atomically.
//
// Unlike cmd/rollup-stats, whose source is the `jobs` table, this snapshot's source
// is Meilisearch's facet count: it calls the same search.FacetCounts the live
// filters use, so the snapshot matches the live catalogue and the SPA filter facets
// exactly. The facet count runs BEFORE the transaction opens, so a slow or failing
// Meili never holds a transaction open, and a Meili error aborts the run before it
// touches the table (the prior snapshot serves on).
//
// It is a run-once-and-exit worker (cron-scheduled once per day — facet
// distributions move slowly). A second pass then rebuilds insights_skill_neighbours,
// the skills listed most often beside each skill, which the /skills/<slug> glossary
// pages read instead of asking the engine on every render. Each delete + reinsert runs
// inside one transaction, so a reader never sees a partial rebuild and a rerun is idempotent. It exits non-zero if
// the facet count or the rebuild fails, so cron can alert.
package main

import (
	"context"
	"log"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/facetsnapshot"
	"github.com/strelov1/freehire/internal/search/search"
)

func main() {
	worker.Main(run)
}

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	if cfg.MeiliKey == "" {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}

	// Compute the distribution BEFORE opening the transaction: the unfiltered,
	// whole-catalogue facet count over the four covered attributes.
	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	res, err := client.FacetCounts(ctx, search.FacetParams{Facets: facetsnapshot.Attributes()})
	if err != nil {
		log.Printf("facet count: %v", err)
		return 1
	}
	rows := facetsnapshot.Rows(res)

	// The delete + reinsert run in one transaction so the swap is atomic: readers keep
	// seeing the previous snapshot until commit, and a failed rebuild rolls back.
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Printf("begin: %v", err)
		return 1
	}
	defer tx.Rollback(ctx)

	q := db.New(pool).WithTx(tx)

	if err := q.DeleteAllFacetStats(ctx); err != nil {
		log.Printf("clear snapshot: %v", err)
		return 1
	}
	for _, r := range rows {
		if err := q.InsertFacetStat(ctx, db.InsertFacetStatParams{Facet: r.Facet, Value: r.Value, Count: r.Count}); err != nil {
			log.Printf("insert facet stat: %v", err)
			return 1
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("commit: %v", err)
		return 1
	}

	log.Printf("rollup-facets: rebuilt insights_facet_stats (%d rows across %d facets)", len(rows), len(facetsnapshot.Facets))

	// Second pass, its own swap: each skill's neighbours, for the glossary pages. One
	// engine query per DESCRIBED skill — exactly the skills that have a page, read from
	// the dictionary rather than from the distribution above, which the engine caps at
	// maxValuesPerFacet. It runs after the first snapshot is committed, so a failure
	// here costs only this table's refresh.
	skills := slices.Sorted(maps.Keys(skilltag.Descriptions()))
	pairs, err := facetsnapshot.Neighbours(ctx, client, skills, facetsnapshot.NeighbourLimit)
	if err != nil {
		log.Printf("skill neighbours: %v", err)
		return 1
	}
	if err := storeNeighbours(ctx, pool, pairs); err != nil {
		log.Printf("skill neighbours: %v", err)
		return 1
	}
	log.Printf("rollup-facets: rebuilt insights_skill_neighbours (%d pairs across %d skills)", len(pairs), len(skills))
	return 0
}

// storeNeighbours swaps the co-occurrence snapshot in one transaction, so a reader
// keeps the previous one until commit.
func storeNeighbours(ctx context.Context, pool *pgxpool.Pool, pairs []facetsnapshot.NeighbourRow) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := db.New(pool).WithTx(tx)
	if err := q.DeleteAllSkillNeighbours(ctx); err != nil {
		return err
	}
	for _, p := range pairs {
		if err := q.InsertSkillNeighbour(ctx, db.InsertSkillNeighbourParams{Skill: p.Skill, Neighbour: p.Neighbour, Count: p.Count}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
