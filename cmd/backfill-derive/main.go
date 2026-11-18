// Command backfill-derive re-derives, in one keyset pass over the jobs table, every column
// ingest computes as a pure function of a row's own raw fields: the dictionary facets and
// synthetic enrichment facets (jobderive.Derive, plus engagement), requirements_derived
// (internal/job/reqextract), role_fingerprint (internal/job/jobhash) and company_slug
// (internal/dict/normalize, via jobderive).
//
// Ingest fills all of these on every crawl, so new rows need no backfill; rows that predate a
// dictionary or algorithm change, and closed jobs that never re-crawl, keep stale values until
// this worker rewrites them. A re-derived row is what a fresh ingest of the same raw fields
// would produce, except public_slug, which it never touches: that is a published URL minted
// once at insert, and re-deriving it from a changed title would 404 every link to the posting.
// Idempotent: a second run rewrites nothing.
//
// The re-derive is CPU-bound (skilltag.Parse runs ~150 regexes per description).
// BACKFILL_CONCURRENCY (default 1) fans the per-row work out: one reader pages the table by
// keyset and feeds a channel, N workers derive and write. Rows are order-independent. Set a low
// CPUWeight on the unit so a big backfill never starves the live API.
//
// Preserved when already set: work_mode (jobderive keeps an existing, possibly adapter-structured
// value over the parsed-location hint) and employment_type (jobderive.RetainEmploymentType),
// except a stored part_time whose only textual evidence is a pay-policy sentence ("salaries for
// part-time roles will be pro-rated"): that is the false positive, and keeping it would freeze
// the mislabel. An ATS part_time whose description never says the words is kept.
// Other structured-source facets are NOT preserved: an adapter that emits a grade, category,
// skills or required experience directly (e.g. getmatch) supplies them only at ingest, and this
// command overwrites seniority/category/skills/experience_years_min with the dictionary's,
// because its job is to propagate dictionary changes. A boardless adapter re-supplies them on
// its next full crawl.
//
// Country and region are preserved whenever the row carries a country (rederive.StoredGeography):
// re-deriving them from free-text location alone moved SEEK "Perth WA" rows to the US. A row
// with no stored country is re-derived in full. Cities are not preserved, nor is a region on a
// row with no country. A moderator- or submission-authored vacancy may state them
// (internal/ingest/moderation passes them as authoritative), and this command re-derives them
// from the free-text location, which usually yields nothing. Unlike getmatch, a manual job is
// never crawled again, so nothing restores them. That is deliberate: moderation's own edit path
// already re-derives every facet from content with no structured overrides, so the project
// treats stored geography as re-derivable. If that intent changes, change BOTH places (skip the
// two columns for `created_by IS NOT NULL` rows in UpdateJobDerived, and stop blanking them in
// moderation); two doors disagreeing about whether stated geography survives is worse than either answer.
//
// When a company slug moves (a deliberate slug-builder change), the run re-keys the companies
// catalogue afterwards, only for the slugs it moved, in chunks (reconcileMovedSlugs). Follow the
// run with a reindex (make reindex), whose duplicate_of recompute collapses newly-clustered
// reposts and unions their geography onto each canon.
package main

import (
	"context"
	"fmt"
	"log"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// backfillBatchSize bounds how many jobs are read per keyset page.
const backfillBatchSize = 500

// progressEvery is how many rows pass between progress lines, so a working run and a wedged
// one are distinguishable during a multi-hour pass.
const progressEvery = 100_000

// deriveStore is the slice of the data layer the concurrent pass needs. *db.Queries satisfies
// it; tests use a fake. UpdateJobDerived is called concurrently by the worker pool, so the
// store must be safe for concurrent use. The companies reconcile (reconcileMovedSlugs) is
// deliberately not here: it runs once, single-threaded, after the pass.
type deriveStore interface {
	// The three reads worker.NewFullScanReader needs: the wide keyset batch, the id-only
	// fallback, and the single-row fetch that isolates a damaged row.
	worker.FullScanQueries
	// The same three, narrowed to rows that can still reach the catalogue; used when
	// BACKFILL_DERIVE_CLOSED_WITHIN_DAYS bounds the pass.
	worker.LiveScanQueries
	UpdateJobDerived(ctx context.Context, arg db.UpdateJobDerivedParams) error
	ListCompanySlugAliases(ctx context.Context) ([]db.ListCompanySlugAliasesRow, error)
}

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

	// Default 1. BACKFILL_CONCURRENCY=6 has degraded prod; hold it at 2-3.
	concurrency, err := worker.EnvInt64("BACKFILL_CONCURRENCY", 1)
	if err != nil {
		log.Printf("backfill-derive: %v", err)
		return 1
	}

	// Unset is unbounded (the whole table). worker.EnvInt64 refuses a 0, so "unset" is the only
	// way to say unbounded and a typo cannot silently mean "scan nothing".
	maxRows, err := worker.EnvInt64("BACKFILL_DERIVE_MAX", 0)
	if err != nil {
		log.Printf("backfill-derive: %v", err)
		return 1
	}
	fromID, err := worker.EnvInt64("BACKFILL_DERIVE_FROM_ID", 0)
	if err != nil {
		log.Printf("backfill-derive: %v", err)
		return 1
	}

	// Skip the postings nothing can surface any more (most of the table is closed).
	//
	// Expressed in DAYS CLOSED rather than an open-only flag, because a closed posting can
	// return: ingest's Toucher reopens it without rewriting its facets, so an open-only pass
	// would hand those rows back with the old dictionary's facets and nothing would report it.
	// The number is how far back a posting might still return from; unset keeps the whole table.
	closedWithinDays, err := worker.EnvInt64("BACKFILL_DERIVE_CLOSED_WITHIN_DAYS", 0)
	if err != nil {
		log.Printf("backfill-derive: %v", err)
		return 1
	}
	var closedSince time.Time
	if closedWithinDays > 0 {
		if closedWithinDays > math.MaxInt32 {
			log.Printf("backfill-derive: BACKFILL_DERIVE_CLOSED_WITHIN_DAYS too large: %d", closedWithinDays)
			return 1
		}
		closedSince = time.Now().UTC().AddDate(0, 0, -int(closedWithinDays))
	}

	queries := db.New(pool)
	log.Printf("backfill-derive starting: concurrency=%d from_id=%d max=%d closed_within_days=%d",
		concurrency, fromID, maxRows, closedWithinDays)
	pass, orphaned, err := derivePass(ctx, queries, concurrency, scanWindow{
		fromID: fromID, maxRows: maxRows, closedSince: closedSince,
	})

	// ONE report, reached by every path: deciding what to say at each `return` is how the
	// resume point went missing before.
	msg, exit := runOutcome(pass, orphaned, err)
	log.Print(msg)
	if err != nil {
		log.Printf("backfill-derive: %v", err)
	}
	return exit
}

// passStore is what derivePass needs: the scan's store plus the reconcile's writer.
// *db.Queries satisfies it.
type passStore interface {
	deriveStore
	companyReconciler
}

// derivePass runs the scan and, when a slug moved, reconciles the companies catalogue derived
// from it. It returns whatever it managed before failing, so the caller still holds the resume
// point when a later step is what failed.
func derivePass(ctx context.Context, queries passStore, concurrency int64, win scanWindow) (backfillRun, int64, error) {
	pass, err := backfillPass(ctx, queries, concurrency, win)

	// A slug rewrite re-keys jobs.company_slug; reconcile the companies catalogue for the slugs
	// that moved (and drop rows orphaned by the change) so company pages resolve. This runs even
	// when the pass failed or was stopped: the rows it did move are committed, and leaving their
	// companies unreconciled 404s every new slug. The reconcile runs on a detached context that
	// the run's own stop (SIGTERM, timeout) ends only after a grace period, rather than a fixed
	// deadline a large reconcile could outlast.
	rctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	stop := context.AfterFunc(ctx, func() { time.AfterFunc(reconcileGrace, cancel) })
	defer stop()
	rec, rerr := reconcileMovedSlugs(rctx, queries, pass, reconcileChunk)
	orphaned := rec.orphaned
	if rec != (reconciled{}) {
		log.Printf("backfill-derive: companies reconciled for %d new and %d old slugs: "+
			"%d rows created, %d redirects recorded (of %d eligible), %d retired",
			len(pass.MovedTo), len(pass.MovedFrom), rec.created, rec.aliased, len(pass.Aliases), rec.orphaned)
	}
	switch {
	case err != nil && rerr != nil:
		// The pass's error decides the exit and the resume report; the reconcile's still gets a line.
		log.Printf("backfill-derive: companies reconcile after a failed pass: %v", rerr)
		return pass, orphaned, err
	case err != nil:
		return pass, orphaned, err
	default:
		return pass, orphaned, rerr
	}
}

// reconcileGrace bounds the reconcile of a run that was stopped (SIGTERM, timeout): long
// enough for a few thousand slugs in chunks, short of systemd's stop timeout.
const reconcileGrace = 60 * time.Second

// reconcileChunk is how many slugs one reconcile statement covers. Each statement is its
// own transaction, so this bounds how long any companies row stays locked against ingest.
const reconcileChunk = 500

// companyReconciler is the write side of the companies reconcile, narrowed so the chunking
// can be tested without a database.
type companyReconciler interface {
	InsertDerivedCompanySlugAliases(ctx context.Context, arg db.InsertDerivedCompanySlugAliasesParams) (int64, error)
	InsertMissingCompaniesForSlugs(ctx context.Context, slugs []string) (int64, error)
	DeleteOrphanCompaniesForSlugs(ctx context.Context, slugs []string) (int64, error)
}

// reconciled is what one reconcile wrote.
type reconciled struct{ created, aliased, orphaned int64 }

// reconcileMovedSlugs gives each slug the run moved jobs ONTO a companies row, records the
// redirects the run earned (old slug -> new, see movedSlugs.add), then drops the rows of the
// slugs it moved jobs AWAY from that no job carries any more.
//
// Scoped to those slugs and chunked, NOT a whole-table SyncCompaniesFromJobs +
// DeleteOrphanCompanies: that sync ran about an hour in one transaction, keeping every inserted
// row locked and queueing ingest's UpsertJob behind it. The order is the durability story: a
// canonical row exists before anything redirects to it, and a redirect exists before the old
// row is dropped, as in cmd/merge-companies.
func reconcileMovedSlugs(ctx context.Context, r companyReconciler, pass backfillRun, chunk int) (reconciled, error) {
	var out reconciled
	for part := range slices.Chunk(pass.MovedTo, chunk) {
		n, err := r.InsertMissingCompaniesForSlugs(ctx, part)
		if err != nil {
			return out, fmt.Errorf("insert companies: %w", err)
		}
		out.created += n
	}
	for part := range slices.Chunk(pass.Aliases, chunk) {
		var arg db.InsertDerivedCompanySlugAliasesParams
		for _, a := range part {
			arg.AliasSlugs = append(arg.AliasSlugs, a.Alias)
			arg.CanonicalSlugs = append(arg.CanonicalSlugs, a.Canonical)
			arg.FoldedKeys = append(arg.FoldedKeys, a.FoldedKey)
		}
		n, err := r.InsertDerivedCompanySlugAliases(ctx, arg)
		if err != nil {
			return out, fmt.Errorf("record company slug aliases: %w", err)
		}
		out.aliased += n
	}
	for part := range slices.Chunk(pass.MovedFrom, chunk) {
		n, err := r.DeleteOrphanCompaniesForSlugs(ctx, part)
		if err != nil {
			return out, fmt.Errorf("delete orphan companies: %w", err)
		}
		out.orphaned += n
	}
	return out, nil
}

// runOutcome turns what a run managed into the one line it prints and the code it exits with.
// Pure, so every ending is a test table. The resume point is the difference between "that was
// all" and "most of the table is still stale", so it is said plainly whenever there is one,
// whether or not the run also failed.
func runOutcome(pass backfillRun, orphaned int64, err error) (string, int) {
	exit := 0
	if err != nil {
		exit = 1
	}
	switch {
	case pass.ResumeID > 0:
		return fmt.Sprintf("backfill-derive stopped early: scanned=%d updated=%d slugs_moved=%d companies_orphaned=%d — "+
			"the table is NOT fully derived, continue with BACKFILL_DERIVE_FROM_ID=%d",
			pass.Scanned, pass.Updated, pass.SlugsMoved, orphaned, pass.ResumeID), exit
	case err != nil:
		// No resume point and a failure: the run never got far enough to have one (a registry
		// that would not load, say). Claiming `done` would be a lie about the whole table.
		return fmt.Sprintf("backfill-derive stopped before it could derive anything: scanned=%d", pass.Scanned), exit
	default:
		return fmt.Sprintf("backfill-derive done: scanned=%d updated=%d slugs_moved=%d companies_orphaned=%d (follow with a reindex)",
			pass.Scanned, pass.Updated, pass.SlugsMoved, orphaned), exit
	}
}

// scanWindow bounds one run of the pass. A full pass de-TOASTs a description for every row and
// outlasts any sensible unit timeout, so a run has to be able to stop and be continued.
type scanWindow struct {
	// fromID is the first id a run must DO, inclusive.
	fromID int64
	// maxRows bounds how many rows one run scans. Zero is unbounded; worker.EnvInt64 refuses
	// anything but a positive value, so "unset" is the only way to ask for the whole table.
	maxRows int64
	// closedSince narrows the scan to open postings plus ones closed at or after it; the zero
	// time means the whole table. A cutoff rather than `closed_at IS NULL` because a closed
	// posting reopens without its facets being rewritten (ingest's Toucher), so the cutoff is
	// the operator's statement of how far back a posting might still come from.
	closedSince time.Time
}

// pendingRows are the rows handed to the worker pool that have not finished.
//
// The reader runs AHEAD of the pool (jobsCh is buffered at backfillBatchSize), so resuming
// from the reader's cursor would skip the queued backlog, and a skipped row keeps stale facets
// for good. The resume point therefore comes from the OLDEST unfinished row. A row is added
// when the reader hands it over and removed when a worker is done with it, including when it
// needs no write. A row whose write FAILED stays, so the resume point falls before it.
type pendingRows struct {
	mu  sync.Mutex
	ids map[int64]struct{}
}

func newPendingRows() *pendingRows { return &pendingRows{ids: make(map[int64]struct{})} }

func (p *pendingRows) add(id int64) {
	p.mu.Lock()
	p.ids[id] = struct{}{}
	p.mu.Unlock()
}

func (p *pendingRows) done(id int64) {
	p.mu.Lock()
	delete(p.ids, id)
	p.mu.Unlock()
}

// oldest is the lowest id still unfinished. Read once, after the pool has joined.
func (p *pendingRows) oldest() (int64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var lo int64
	var found bool
	for id := range p.ids {
		if !found || id < lo {
			lo, found = id, true
		}
	}
	return lo, found
}

// movedSlugs collects the distinct slugs a run moved jobs from and to. Guarded: the worker
// pool records into it concurrently.
type movedSlugs struct {
	mu       sync.Mutex
	from, to map[string]struct{}
	// aliases holds, per slug moved away from, the redirect it would earn, or a nil entry once
	// any row disqualified it.
	aliases map[string]*slugAlias
}

// slugAlias is a retirement recorded in company_slug_aliases, so the old company URL 301s to
// the new one.
type slugAlias struct {
	Alias, Canonical, FoldedKey string
}

func newMovedSlugs() *movedSlugs {
	return &movedSlugs{
		from: make(map[string]struct{}), to: make(map[string]struct{}),
		aliases: make(map[string]*slugAlias),
	}
}

// add records one moved row; name is the company name both slugs were derived from.
//
// A slug earns an alias only when EVERY row moved off it qualifies: its slug was minted from
// this same name by an older rule (normalize.SlugPredatesRule, as in cmd/merge-companies), and
// all of them landed on ONE new slug under one folded key. A slug a renamed employer left
// behind, or one two employers shared and now split, is a judgement and gets no redirect.
func (m *movedSlugs) add(from, to, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if from != "" {
		m.from[from] = struct{}{}
	}
	if to != "" {
		m.to[to] = struct{}{}
	}
	if from == "" || to == "" {
		return
	}
	cand, seen := m.aliases[from]
	key := normalize.CompanyKey(name)
	switch {
	case seen && cand == nil:
		// already disqualified
	case !normalize.SlugPredatesRule(from, name):
		m.aliases[from] = nil
	case !seen:
		m.aliases[from] = &slugAlias{Alias: from, Canonical: to, FoldedKey: key}
	case cand.Canonical != to || cand.FoldedKey != key:
		m.aliases[from] = nil
	}
}

// qualifiedAliases is called only after the pool has joined. A folded key the run would point
// at two DIFFERENT canons drops every alias on it: ingest resolves a spelling by that key, and
// one key must name one canon. The statement guards existing registry rows, not two rows
// arriving in the same batch.
func (m *movedSlugs) qualifiedAliases() []slugAlias {
	canonOf := map[string]string{}
	conflicted := map[string]bool{}
	for _, a := range m.aliases {
		if a == nil {
			continue
		}
		if c, seen := canonOf[a.FoldedKey]; seen && c != a.Canonical {
			conflicted[a.FoldedKey] = true
		}
		canonOf[a.FoldedKey] = a.Canonical
	}
	var out []slugAlias
	for _, a := range m.aliases {
		if a != nil && !conflicted[a.FoldedKey] {
			out = append(out, *a)
		}
	}
	slices.SortFunc(out, func(a, b slugAlias) int { return strings.Compare(a.Alias, b.Alias) })
	return out
}

// sorted is called only after the pool has joined.
func (m *movedSlugs) sorted(set map[string]struct{}) []string {
	return slices.Sorted(maps.Keys(set))
}
