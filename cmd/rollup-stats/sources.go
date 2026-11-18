package main

import (
	"context"
	"log"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/ingest/sourcestats"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// sourceFacetAttr is the index attribute holding a posting's source key — the same
// attribute the public `source` filter reads, so the counts published here are the ones
// /jobs?source=<key> will show.
const sourceFacetAttr = "source"

// sourceFacetTimeout bounds the one facet request. The SDK's HTTP client has no overall
// request timeout, so without this a stalled Meilisearch holds the run open.
const sourceFacetTimeout = 15 * time.Second

// rebuildSourceStats measures each source and replaces the source_stats snapshot the
// public /sources page reads. It runs after the rollups have committed, outside their
// transaction, and never changes the exit code: every failure is logged and swallowed.
func rebuildSourceStats(ctx context.Context, reader, pool *pgxpool.Pool, meiliURL, meiliKey string) {
	// One report for every ending, so no `return` path ends silently.
	published, err := publishSourceStats(ctx, reader, pool, meiliURL, meiliKey)
	if err != nil {
		log.Printf("rollup-stats: source snapshot not published: %v", err)
		return
	}
	log.Printf("rollup-stats: published the source snapshot (%d sources)", published)
}

// publishSourceStats does the work and returns how many sources it wrote.
func publishSourceStats(ctx context.Context, reader, pool *pgxpool.Pool, meiliURL, meiliKey string) (int, error) {
	q := db.New(pool)

	agg, err := db.New(reader).AggregateOpenJobsBySource(ctx)
	if err != nil {
		return 0, err
	}

	// The snapshot this run replaces, read for its keys only: a quiet source must keep a
	// measured zero rather than vanish from the page. See sourcestats.Union.
	previous, err := q.ListSourceStats(ctx)
	if err != nil {
		return 0, err
	}

	// Taxonomy, not a crawl registry: a keyed adapter is absent from a crawl registry
	// wherever its credential is unset. Built once because it assembles every adapter.
	registry := slices.Collect(maps.Keys(sources.Taxonomy()))

	rows := sourcestats.Rows(registry, agg, previous, fetchBrowsableCounts(ctx, meiliURL, meiliKey), time.Now().UTC())

	// Delete-and-reinsert in one transaction so a reader never sees a partial snapshot.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	qtx := q.WithTx(tx)
	if err := qtx.DeleteAllSourceStats(ctx); err != nil {
		return 0, err
	}
	for _, row := range rows {
		if err := qtx.InsertSourceStat(ctx, row); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// fetchBrowsableCounts asks Meilisearch for the de-duplicated open count of every source in
// one facet request. An absent credential or unreachable Meilisearch costs only this
// figure (recorded as absent); it must never degrade to zero, which would state publicly
// that every source carries nothing.
func fetchBrowsableCounts(ctx context.Context, meiliURL, meiliKey string) sourcestats.BrowsableCounts {
	if meiliKey == "" {
		log.Print("rollup-stats: MEILI_MASTER_KEY not set, publishing the source snapshot without de-duplicated counts")
		return sourcestats.Unmeasured()
	}

	fctx, cancel := context.WithTimeout(ctx, sourceFacetTimeout)
	defer cancel()

	res, err := search.NewClient(meiliURL, meiliKey).FacetCounts(fctx, search.FacetParams{
		Facets: []string{sourceFacetAttr},
	})
	if err != nil {
		log.Printf("rollup-stats: de-duplicated source counts unavailable, publishing without them: %v", err)
	}
	return resolveBrowsableCounts(res, err)
}

// resolveBrowsableCounts decides whether a facet response is a measurement. A response
// with no distribution for the source attribute is unmeasured, not empty: read literally
// it would publish a zero for every source.
func resolveBrowsableCounts(res search.FacetResult, err error) sourcestats.BrowsableCounts {
	if err != nil {
		return sourcestats.Unmeasured()
	}
	dist, ok := res.Facets[sourceFacetAttr]
	if !ok {
		return sourcestats.Unmeasured()
	}
	return sourcestats.Measured(dist)
}
