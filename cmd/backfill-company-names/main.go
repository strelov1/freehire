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
// slug). Each name is written to the board it came from before its jobs (see
// writeRename) — otherwise the board's next crawl puts the slug back. Renames re-key
// company_slug via normalize.Slug; the derived companies catalogue reconciles through
// SyncCompaniesFromJobs + DeleteOrphanCompanies. Idempotent — a second run finds
// nothing slug-like left to fix. Follow with `make reindex` so corrected names reach
// search.
//
// A second pass fixes the boards whose company is not a slug but the board id itself — a
// "<host>/<site>" path or a GUID a harvest wrote into boards.company verbatim (measured
// 2026-10-23: ~2,200 companies carrying ~156k open postings under a host path, ~7,400 carrying
// ~44k under a GUID). It is driven by the board rather than by the jobs, asks the ATS for
// exactly that board's name (companyname.NewPlaceholderRegistry), and renames the board and its
// stored jobs in one transaction — see writePlaceholderRename.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"maps"
	"slices"
	"sort"
	"sync"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/strelov1/freehire/internal/dict/companyname"
	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// perSourceConcurrency bounds simultaneous fetches against ONE source. Every source this
// worker reads serves its boards from one host or a handful (join.com, jobs.lever.co,
// recruiting.paylocity.com), so a bound across the whole run lands on one host: at 24, the
// first prod run of the placeholder pass drew 429s from Paylocity within minutes and 1,382
// from join.com, and crawled at their retry backoff.
const perSourceConcurrency = 4

// eachPerSource calls fn for every item — the sources in parallel, each held to
// perSourceConcurrency — and returns when all are done.
func eachPerSource[T any](items []T, source func(T) string, fn func(T)) {
	bySource := map[string][]T{}
	for _, it := range items {
		bySource[source(it)] = append(bySource[source(it)], it)
	}
	var wg sync.WaitGroup
	for _, group := range bySource {
		wg.Go(func() {
			var g errgroup.Group
			g.SetLimit(perSourceConcurrency)
			for _, it := range group {
				g.Go(func() error { fn(it); return nil })
			}
			_ = g.Wait()
		})
	}
	wg.Wait()
}

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

	placeholders := companyname.NewPlaceholderRegistry(client)
	boards, err := queries.ListPlaceholderCompanyBoards(ctx, slices.Sorted(maps.Keys(placeholders)))
	if err != nil {
		log.Printf("list placeholder boards: %v", err)
		return 1
	}
	boardRenames, unresolved := resolvePlaceholders(ctx, boards, placeholders)
	sort.Slice(boardRenames, func(i, j int) bool {
		a, b := boardRenames[i], boardRenames[j]
		return a.provider < b.provider || (a.provider == b.provider && a.board < b.board)
	})

	if *dryRun {
		for _, r := range renames {
			log.Printf("dry-run: %s -> %q", r.oldSlug, r.name)
		}
		for _, r := range boardRenames {
			log.Printf("dry-run: %s %s -> %q", r.provider, r.board, r.name)
		}
		log.Printf("backfill-company-names dry-run: candidates=%d would_resolve=%d no_source=%d rejected=%d placeholder_boards=%d would_resolve_boards=%d unresolved_boards=%d",
			len(rows), len(renames), stats.noSource, stats.rejected, len(boards), len(boardRenames), unresolved)
		return 0
	}

	applied, failed := applyRenames(ctx, renames, func(ctx context.Context, r rename) (bool, error) {
		return writeRename(ctx, queries, r)
	})
	boardsApplied, boardsFailed := applyRenames(ctx, boardRenames, func(ctx context.Context, r placeholderRename) (bool, error) {
		var changed bool
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			var err error
			changed, err = writePlaceholderRename(ctx, queries.WithTx(tx), r)
			return err
		})
		return changed, err
	})

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

	log.Printf("backfill-company-names done: candidates=%d resolved=%d applied=%d failed=%d no_source=%d rejected=%d placeholder_boards=%d resolved_boards=%d boards_applied=%d boards_failed=%d unresolved_boards=%d companies_orphaned=%d",
		len(rows), len(renames), applied, failed, stats.noSource, stats.rejected,
		len(boards), len(boardRenames), boardsApplied, boardsFailed, unresolved, orphans)
	return worker.ExitCode(failed+boardsFailed, 0)
}

type rename struct {
	oldSlug string
	oldName string
	name    string
	// source and board locate the boards row the name came from, so the rename can be
	// written back to it — see writeRename.
	source string
	board  string
}

// renamer is the slice of db.Queries writeRename needs, narrowed so a test can record
// the order of the two writes.
type renamer interface {
	RenameSlugBoardCompany(context.Context, db.RenameSlugBoardCompanyParams) (int64, error)
	RenameSlugCompany(context.Context, db.RenameSlugCompanyParams) (int64, error)
}

// writeRename applies one resolved name to the board and then to its jobs, reporting
// whether any job changed.
//
// The board is what makes the rename stick: the adapter copies boards.company onto
// every posting and the job upsert overwrites jobs.company, so a rename written to jobs
// alone was undone by the board's next crawl. And the board goes FIRST. Renamed jobs no
// longer look slug-like, so no later run lists the company again; were the board write
// to fail after them, the next crawl would restore the slug with nothing left to notice.
// In this order a failed jobs write heals itself — the crawl carries the name from the
// board — and a failed board write leaves the company listed for the next run.
func writeRename(ctx context.Context, q renamer, r rename) (bool, error) {
	if _, err := q.RenameSlugBoardCompany(ctx, db.RenameSlugBoardCompanyParams{
		Name:     r.name,
		Provider: r.source,
		Board:    r.board,
		OldName:  r.oldName,
	}); err != nil {
		return false, fmt.Errorf("board: %w", err)
	}
	n, err := q.RenameSlugCompany(ctx, db.RenameSlugCompanyParams{
		Name:    r.name,
		NewSlug: normalize.Slug(r.name),
		OldSlug: r.oldSlug,
	})
	return n > 0, err
}

// applyRenames writes each rename via write, counting how many actually
// changed a row (applied) and how many errored (failed). A single company's
// write failure (e.g. a unique-constraint collision on the derived slug) must
// not abort the batch, so the loop always continues; the caller uses failed
// to decide the process exit code instead.
func applyRenames[T any](ctx context.Context, renames []T, write func(context.Context, T) (bool, error)) (applied, failed int) {
	for _, r := range renames {
		ok, err := write(ctx, r)
		if err != nil {
			log.Printf("rename %+v: %v", r, err)
			failed++
			continue
		}
		if ok {
			applied++
		}
	}
	return applied, failed
}

type resolveStats struct {
	noSource int // no resolver for the source, or board not derivable from the URL
	rejected int // candidate failed the confidence gate
}

// resolveNames fetches a display-name candidate per eligible company concurrently
// and keeps those that pass the gate. It is order-independent; the caller sorts.
func resolveNames(ctx context.Context, rows []db.ListSlugLikeCompaniesForBackfillRow, registry companyname.Registry) ([]rename, resolveStats) {
	type job struct {
		row      db.ListSlugLikeCompaniesForBackfillRow
		resolver companyname.Resolver
		board    string
	}
	var (
		jobs  []job
		stats resolveStats
	)
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
		board, ok := companyname.Board(row.Source, row.Name, row.URL)
		if !ok {
			stats.noSource++
			continue
		}
		jobs = append(jobs, job{row: row, resolver: resolver, board: board})
	}

	var (
		mu      sync.Mutex
		renames []rename
	)
	eachPerSource(jobs, func(j job) string { return j.row.Source }, func(j job) {
		candidate, err := j.resolver.Name(ctx, j.board)
		if err != nil {
			log.Printf("resolve %s (%s): %v", j.row.Slug, j.row.Source, err)
			return // a single board failing must not abort the run
		}
		name, ok := companyname.Accept(j.row.Source, j.row.Name, candidate)
		mu.Lock()
		defer mu.Unlock()
		if !ok {
			stats.rejected++
			return
		}
		renames = append(renames, rename{oldSlug: j.row.Slug, oldName: j.row.Name, name: name, source: j.row.Source, board: j.board})
	})

	return renames, stats
}
