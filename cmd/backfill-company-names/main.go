// Command backfill-company-names replaces slug-like company names with the company's real
// display name, resolved from that ATS's own careers-page title or API. Many ATS adapters set
// jobs.company straight from the board file, so a squished slug (e.g. "lbresearch", "gs1ca")
// leaks into the UI and the /companies/<slug> URL and breaks logo.dev name resolution.
//
//	backfill-company-names [--dry-run] [--provider=paycor]   # needs DATABASE_URL
//
// It touches only slug-like companies that have open jobs, resolves a candidate name per board
// (bounded-concurrent HTTP), and applies it only when it passes the confidence gate, never
// guessing from the slug. Each name is written to its board before its jobs (see writeRename),
// or the next crawl puts the slug back. Renames re-key company_slug via normalize.Slug; the
// companies catalogue reconciles for exactly the touched slugs (reconcileRenamedSlugs).
// Idempotent. Follow with `make reindex` so corrected names reach search.
//
// A second pass fixes boards whose company is the board id itself (a bare host, a
// "<host>/<site>" path or a GUID a harvest wrote into boards.company). It is driven by the
// board, asks the ATS for exactly that board's name (companyname.NewPlaceholderRegistry), and
// renames the board and its jobs in one transaction (writePlaceholderRename).
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
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/strelov1/freehire/internal/dict/companyname"
	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// perSourceConcurrency bounds simultaneous fetches against ONE source. A source serves its
// boards from one host or a handful (join.com, jobs.lever.co, recruiting.paylocity.com), so a
// run-wide bound lands on one host and draws 429s.
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

// llmProviders are the placeholder providers a model names from posting text; each is capped
// at COMPANY_NAMES_LLM_MAX boards a run.
var llmProviders = []string{"adp", "paycor"}

func main() { worker.Main(run) }

func run() int {
	dryRun := flag.Bool("dry-run", false, "print proposed renames without writing")
	provider := flag.String("provider", "", "run only the placeholder pass, for this one provider (e.g. paycor)")
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

	var rows []db.ListSlugLikeCompaniesForBackfillRow
	if *provider == "" {
		if rows, err = queries.ListSlugLikeCompaniesForBackfill(ctx); err != nil {
			log.Printf("list companies: %v", err)
			return 1
		}
	}

	renames, stats := resolveNames(ctx, rows, registry)
	sort.Slice(renames, func(i, j int) bool { return renames[i].oldSlug < renames[j].oldSlug })

	placeholders := companyname.NewPlaceholderRegistry(client)
	// adp (and paycor, where the listing title names no employer) states the employer only in
	// the posting text, so a model reads that text (postingTextResolver): only when LLM_* is configured, and at most COMPANY_NAMES_LLM_MAX
	// boards a run (unset = all), since each board costs one call.
	llmMax, err := worker.EnvInt64("COMPANY_NAMES_LLM_MAX", 0)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	if *provider != "" && !slices.Contains(llmProviders, *provider) {
		// No model for a run that cannot use it.
	} else if lcfg := config.LoadLLM(); lcfg.Require() == nil {
		model, flush, err := llm.NewClient(lcfg.Settings(lcfg.Model), "company-names")
		if err != nil {
			log.Printf("llm: %v", err)
			return 1
		}
		defer flush()
		// Service credential, filed under its own feature so this pass's spend reads apart from
		// enrichment's.
		model = model.As("", nil, llm.Feature("company-names"))
		placeholders["adp"] = postingTextResolver{q: queries, model: model, source: "adp"}
		placeholders["paycor"] = newPaycorResolver(placeholders["paycor"], postingTextResolver{q: queries, model: model, source: "paycor", vendor: paycorVendor})
	} else {
		log.Printf("LLM_* not configured: adp placeholder boards are left for a run that has it; paycor boards are named from their listing title only")
	}
	if *provider != "" {
		r, ok := placeholders[*provider]
		if !ok {
			log.Printf("--provider=%s: no placeholder resolver for it", *provider)
			return 1
		}
		placeholders = companyname.Registry{*provider: r}
	}
	boards, err := queries.ListPlaceholderCompanyBoards(ctx, slices.Sorted(maps.Keys(placeholders)))
	if err != nil {
		log.Printf("list placeholder boards: %v", err)
		return 1
	}
	// Days since the epoch, times the cap: the window advances by one cap a day (capProvider).
	for _, p := range llmProviders {
		boards = capProvider(boards, p, llmMax, time.Now().UTC().Unix()/86400*llmMax)
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

	// The slugs the renames moved jobs onto and away from, for the companies reconcile below.
	var movedTo, movedFrom []string
	applied, failed := applyRenames(ctx, renames, func(ctx context.Context, r rename) (bool, error) {
		changed, err := writeRename(ctx, queries, r)
		if changed {
			movedTo = append(movedTo, normalize.Slug(r.name))
			movedFrom = append(movedFrom, r.oldSlug)
		}
		return changed, err
	})
	boardsApplied, boardsFailed := applyRenames(ctx, boardRenames, func(ctx context.Context, r placeholderRename) (bool, error) {
		var (
			slug     string
			oldSlugs []string
		)
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			var err error
			slug, oldSlugs, err = writePlaceholderRename(ctx, queries.WithTx(tx), r)
			return err
		})
		if err != nil || len(oldSlugs) == 0 {
			return false, err
		}
		movedTo = append(movedTo, slug)
		movedFrom = append(movedFrom, oldSlugs...)
		return true, nil
	})

	// jobs now carry the resolved names/slugs; re-key the derived catalogue and sweep the rows
	// orphaned by the re-key so company pages resolve.
	orphans, err := reconcileRenamedSlugs(ctx, queries, movedTo, movedFrom, reconcileChunk)
	if err != nil {
		log.Printf("reconcile companies: %v", err)
		return 1
	}

	log.Printf("backfill-company-names done: candidates=%d resolved=%d applied=%d failed=%d no_source=%d rejected=%d placeholder_boards=%d resolved_boards=%d boards_applied=%d boards_failed=%d unresolved_boards=%d companies_orphaned=%d",
		len(rows), len(renames), applied, failed, stats.noSource, stats.rejected,
		len(boards), len(boardRenames), boardsApplied, boardsFailed, unresolved, orphans)
	return worker.ExitCode(failed+boardsFailed, 0)
}

// reconcileChunk is how many slugs one reconcile statement covers. Each statement is its own
// transaction, so this bounds how long any companies row stays locked against ingest.
const reconcileChunk = 500

// companyReconciler is the write side of the companies reconcile, narrowed so the chunking
// can be tested without a database.
type companyReconciler interface {
	SyncCompaniesForSlugs(ctx context.Context, slugs []string) (int64, error)
	DeleteOrphanCompaniesForSlugs(ctx context.Context, slugs []string) (int64, error)
}

// reconcileRenamedSlugs gives each slug a rename moved jobs ONTO a companies row carrying the
// new name, then drops the rows of slugs it moved jobs AWAY from that no job carries any more.
// Syncs go first, so a slug that is both keeps its row.
//
// Scoped to those slugs and chunked, NOT the whole-table SyncCompaniesFromJobs +
// DeleteOrphanCompanies pair: that sync is one transaction of about an hour and held jobs even
// when nothing was renamed. A run that renamed nothing writes nothing.
func reconcileRenamedSlugs(ctx context.Context, r companyReconciler, movedTo, movedFrom []string, chunk int) (orphaned int64, err error) {
	for part := range slices.Chunk(movedTo, chunk) {
		if _, err := r.SyncCompaniesForSlugs(ctx, part); err != nil {
			return orphaned, fmt.Errorf("sync companies: %w", err)
		}
	}
	for part := range slices.Chunk(movedFrom, chunk) {
		n, err := r.DeleteOrphanCompaniesForSlugs(ctx, part)
		if err != nil {
			return orphaned, fmt.Errorf("delete orphan companies: %w", err)
		}
		orphaned += n
	}
	return orphaned, nil
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

// writeRename applies one resolved name to the board and then to its jobs, reporting whether
// any job changed.
//
// The board goes FIRST: the adapter copies boards.company onto every posting, so a rename
// written to jobs alone is undone by the next crawl. Renamed jobs no longer look slug-like, so
// if the board write failed after them nothing would notice. In this order a failed jobs write
// heals itself (the crawl carries the board's name) and a failed board write leaves the
// company listed for the next run.
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

// applyRenames writes each rename via write, counting rows changed (applied) and errors
// (failed). One company's failure (e.g. a unique-constraint collision on the derived slug)
// must not abort the batch; the caller uses failed for the exit code.
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
