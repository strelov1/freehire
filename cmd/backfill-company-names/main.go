// Command backfill-company-names replaces slug-like company names with the
// company's real display name, resolved from that ATS's own careers-page title
// or API. Many ATS adapters set jobs.company straight from the board file, so a
// board whose file entry is a squished slug (e.g. "lbresearch", "gs1ca", "afcb")
// carries that slug as its display name — which leaks into the UI, the public
// /companies/<slug> URL, and breaks logo.dev name resolution.
//
//	backfill-company-names [--dry-run]   # needs DATABASE_URL
//
// It touches only companies whose name is still slug-like AND that have open
// jobs, resolves a candidate name per board (bounded-concurrent HTTP), and
// applies it only when it passes the confidence gate (never guessing from the
// slug). Renames re-key company_slug via normalize.Slug; the derived companies
// catalogue reconciles through SyncCompaniesFromJobs + DeleteOrphanCompanies.
// Idempotent — a second run finds nothing slug-like left to fix. Follow with
// `make reindex` so corrected names reach search.
package main

import (
	"context"
	"flag"
	"log"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/strelov1/freehire/internal/companyname"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

// resolveConcurrency bounds simultaneous careers-page / API fetches so a large
// ATS completes without hammering a single host.
const resolveConcurrency = 24

func main() { worker.Main(run) }

func run() int {
	dryRun := flag.Bool("dry-run", false, "print proposed renames without writing")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	queries := db.New(pool)
	client := sources.NewClient()
	registry := companyname.NewRegistry(client)

	rows, err := queries.ListSlugLikeCompaniesForBackfill(ctx)
	if err != nil {
		log.Printf("list companies: %v", err)
		return 1
	}

	renames, stats := resolveNames(ctx, rows, registry)
	sort.Slice(renames, func(i, j int) bool { return renames[i].oldSlug < renames[j].oldSlug })

	if *dryRun {
		for _, r := range renames {
			log.Printf("dry-run: %s -> %q", r.oldSlug, r.name)
		}
		log.Printf("backfill-company-names dry-run: candidates=%d would_resolve=%d no_source=%d rejected=%d",
			len(rows), len(renames), stats.noSource, stats.rejected)
		return 0
	}

	applied := 0
	for _, r := range renames {
		n, err := queries.RenameSlugCompany(ctx, db.RenameSlugCompanyParams{
			Name:    r.name,
			NewSlug: normalize.Slug(r.name),
			OldSlug: r.oldSlug,
		})
		if err != nil {
			log.Printf("rename %s: %v", r.oldSlug, err)
			continue
		}
		if n > 0 {
			applied++
		}
	}

	// jobs now carry the resolved names/slugs; re-key the derived catalogue and
	// sweep the rows orphaned by the re-key so company pages resolve.
	if err := queries.SyncCompaniesFromJobs(ctx); err != nil {
		log.Printf("sync companies: %v", err)
		return 1
	}
	orphans, err := queries.DeleteOrphanCompanies(ctx)
	if err != nil {
		log.Printf("delete orphan companies: %v", err)
		return 1
	}

	log.Printf("backfill-company-names done: candidates=%d resolved=%d applied=%d no_source=%d rejected=%d companies_orphaned=%d",
		len(rows), len(renames), applied, stats.noSource, stats.rejected, orphans)
	return 0
}

type rename struct {
	oldSlug string
	name    string
}

type resolveStats struct {
	noSource int // no resolver for the source, or board not derivable from the URL
	rejected int // candidate failed the confidence gate
}

// resolveNames fetches a display-name candidate per eligible company concurrently
// and keeps those that pass the gate. It is order-independent; the caller sorts.
func resolveNames(ctx context.Context, rows []db.ListSlugLikeCompaniesForBackfillRow, registry companyname.Registry) ([]rename, resolveStats) {
	var (
		mu      sync.Mutex
		renames []rename
		stats   resolveStats
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(resolveConcurrency)

	for _, row := range rows {
		// Authoritative slug check (the SQL filter is an approximation).
		if !companyname.SlugLike(row.Name) {
			continue
		}
		resolver, ok := registry[row.Source]
		if !ok {
			stats.noSource++
			continue
		}
		board, ok := companyname.BoardFromURL(row.Source, row.URL)
		if !ok {
			stats.noSource++
			continue
		}

		g.Go(func() error {
			candidate, err := resolver.Name(ctx, board)
			if err != nil {
				log.Printf("resolve %s (%s): %v", row.Slug, row.Source, err)
				return nil // a single board failing must not abort the run
			}
			name, ok := companyname.Accept(row.Name, candidate)
			mu.Lock()
			defer mu.Unlock()
			if !ok {
				stats.rejected++
				return nil
			}
			renames = append(renames, rename{oldSlug: row.Slug, name: name})
			return nil
		})
	}
	_ = g.Wait()

	return renames, stats
}
