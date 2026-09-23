// Command backfill-derive re-derives, in a single keyset pass over the whole jobs
// table, every column that ingest computes as a pure function of a row's own raw
// fields: the deterministic dictionary facets (countries, regions, cities, work_mode,
// skills, seniority, category, is_tech, requires_clearance, and the synthetic enrichment facets
// posting_language, employment_type, education_level, english_level,
// experience_years_min — all from jobderive.Derive), the repost-identity
// role_fingerprint (internal/job/jobhash), and the public_slug/company_slug
// (internal/dict/normalize, via jobderive). It replaces the three former one-shots
// backfill-derive (facets), backfill-role-fingerprint, and reslug with one scan.
//
// Ingest fills all of these on every crawl (job.New + cmd/ingest/store.go), so new
// rows need no backfill; but rows that predate a dictionary or algorithm change — and
// closed jobs that never re-crawl — keep the stale values until this worker rewrites
// them. Because both the facets and the slugs come from jobderive.Derive and the
// fingerprint is computed from the freshly derived company_slug, a re-derived row is
// byte-for-byte what a fresh ingest of the same raw fields would produce. It pages the
// whole table and exits. Idempotent: every column is a pure function of the raw
// fields, so a second run rewrites nothing.
//
// The re-derive is CPU-bound (skilltag.Parse runs ~150 phrase regexes over each
// HTML description), so a single-threaded pass over millions of rows takes hours.
// BACKFILL_CONCURRENCY (default 1) fans the per-row work out across a worker pool:
// one reader pages the table by keyset and feeds a channel, N workers derive and
// write in parallel. The work is embarrassingly parallel (each row is a pure
// function of its own fields, order-independent), so this is near-linear until DB
// write or host CPU saturates. Set a low CPUWeight on the unit so a big backfill
// never starves the live API.
//
// work_mode is preserved when already set: jobderive keeps a row's existing (possibly
// adapter-structured) work_mode over the parsed-location hint. The other
// structured-source facets are NOT preserved: an adapter that emits a grade,
// category, skills, or required-experience directly (e.g. getmatch) supplies those
// only at ingest, and this command re-derives seniority/category/skills/
// experience_years_min from the stored description columns — so running it
// overwrites such structured values with the dictionary's. This is intentional:
// the command's job is to propagate dictionary changes, which must keep updating
// those facets for the dictionary-derived majority. A boardless adapter like
// getmatch re-supplies the structured facets on its next full crawl.
//
// Regions and cities go the same way, and they are the case the sentence above does NOT cover:
// a moderator- or submission-authored vacancy may STATE them (internal/ingest/moderation passes them as
// authoritative structured geography), and this command re-derives both from the free-text
// location — which for such a row usually yields nothing. Unlike getmatch, a manual job is never
// crawled again, so nothing restores them.
//
// That is deliberate rather than an oversight, and it is not this command's decision to make
// alone: internal/ingest/moderation's own edit path already re-derives every facet from content and
// passes no structured overrides, so ANY moderator edit — a title typo fix — does the same thing.
// The project treats stored geography as re-derivable. Measured on production 2026-08-02, the
// blast radius is seven manually-authored rows, all of which still carry regions.
//
// If that ever stops being the intent, the fix is in BOTH places at once (skip the two columns
// for `created_by IS NOT NULL` rows in UpdateJobDerived, and stop blanking them in moderation),
// never in one — two doors disagreeing about whether stated geography survives is worse than
// either answer.
//
// When a slug moves (a deliberate slug-builder change), the run re-keys the companies
// catalogue afterwards (SyncCompaniesFromJobs + DeleteOrphanCompanies), exactly as the
// former cmd/reslug did. Follow the run with a reindex (make reindex), whose
// duplicate_of recompute then collapses any newly-clustered reposts and unions their
// geography onto each canon.
package main

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// backfillBatchSize bounds how many jobs are read per keyset page.
const backfillBatchSize = 500

// progressEvery is how many rows pass between progress lines. A full pass covers over
// five million rows and used to print nothing between "starting" and "done" — hours in
// which a working run and a wedged one were indistinguishable, and "how much longer"
// had no answer short of reading pg_stat_activity. 100k is roughly a line every few
// minutes at prod's rate: enough to see movement, not enough to bury the log.
const progressEvery = 100_000

// deriveStore is the slice of the data layer the concurrent pass needs: page the
// table by keyset and rewrite a row's derived columns. *db.Queries satisfies it;
// tests use a fake. UpdateJobDerived is called concurrently by the worker pool, and
// pgxpool hands each goroutine its own connection, so the store must be safe for
// concurrent use. The companies reconcile (SyncCompaniesFromJobs /
// DeleteOrphanCompanies) is deliberately not here — it runs once, single-threaded,
// after the pass.
type deriveStore interface {
	// The three reads worker.NewFullScanReader needs: the wide keyset batch, the id-only
	// projection it falls back to, and the single-row fetch that isolates a damaged row
	// from its readable neighbours.
	worker.FullScanQueries
	// The same three, narrowed to the rows that can still reach the catalogue. Used when
	// the operator bounds the pass with BACKFILL_DERIVE_CLOSED_WITHIN_DAYS.
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

	// Default 1: the original single-threaded pass. BACKFILL_CONCURRENCY=6 has degraded
	// prod before, so hold it at 2-3 and measure.
	concurrency, err := worker.EnvInt64("BACKFILL_CONCURRENCY", 1)
	if err != nil {
		log.Printf("backfill-derive: %v", err)
		return 1
	}

	// Unset is unbounded — the whole table in one run, which is what a deploy-time
	// dictionary fix wants when the host has the hours to spare. Set it when the run has
	// to fit inside a unit's timeout; worker.EnvInt64 refuses a 0, so "unset" is the only
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

	// Skip the postings nothing can surface any more. Measured 2026-09-23: 1.9M of the
	// table's ~12.7M rows are open, so the unbounded pass spends ~85% of its ~30h on rows
	// no search, sitemap or page will read.
	//
	// Expressed in DAYS CLOSED rather than as an open-only flag, because a closed posting
	// is not permanently gone: ingest's Toucher reopens it without rewriting its facets,
	// and a posting that drifts out of a feed for 48h is routinely closed and reopened as
	// it drifts back. An open-only pass would hand those rows back to the catalogue with
	// the old dictionary's facets and nothing would report it. The number is how far back
	// a posting might still return from; unset keeps the whole table, so the cautious
	// behaviour stays the default.
	closedWithinDays, err := worker.EnvInt64("BACKFILL_DERIVE_CLOSED_WITHIN_DAYS", 0)
	if err != nil {
		log.Printf("backfill-derive: %v", err)
		return 1
	}
	var closedSince time.Time
	if closedWithinDays > 0 {
		closedSince = time.Now().UTC().AddDate(0, 0, -int(closedWithinDays))
	}

	queries := db.New(pool)
	log.Printf("backfill-derive starting: concurrency=%d from_id=%d max=%d closed_within_days=%d",
		concurrency, fromID, maxRows, closedWithinDays)
	pass, orphaned, err := derivePass(ctx, queries, concurrency, scanWindow{
		fromID: fromID, maxRows: maxRows, closedSince: closedSince,
	})

	// ONE report, reached by every path. Deciding what to say at each `return` instead is
	// what made the resume point vanish twice in one evening: the pass's own error is
	// only one of the ways this ends, the companies reconcile after it is two more, and
	// each new exit is another place to forget (freehire#2876).
	msg, exit := runOutcome(pass, orphaned, err)
	log.Print(msg)
	if err != nil {
		log.Printf("backfill-derive: %v", err)
	}
	return exit
}

// derivePass runs the scan and, when a slug moved, reconciles the companies catalogue
// derived from it. It returns whatever it managed before failing, so the caller still
// holds the resume point when a later step is the thing that failed.
func derivePass(ctx context.Context, queries *db.Queries, concurrency int64, win scanWindow) (backfillRun, int64, error) {
	pass, err := backfillPass(ctx, queries, concurrency, win)
	if err != nil {
		return pass, 0, err
	}

	// A slug rewrite re-keys jobs.company_slug; reconcile the derived companies
	// catalogue to match (and drop rows orphaned by the change) so company pages
	// resolve. Skip the whole-table sync when no slug moved.
	if pass.SlugsMoved == 0 {
		return pass, 0, nil
	}
	if err := queries.SyncCompaniesFromJobs(ctx); err != nil {
		return pass, 0, fmt.Errorf("sync companies: %w", err)
	}
	orphaned, err := queries.DeleteOrphanCompanies(ctx)
	if err != nil {
		return pass, 0, fmt.Errorf("delete orphan companies: %w", err)
	}
	return pass, orphaned, nil
}

// runOutcome turns what a run managed into the one line it prints and the code it exits
// with. Pure, so every ending is a table in a test rather than a branch only production
// can reach.
//
// The resume point is the difference between "that was all" and "most of the table is
// still stale", and the pass gives no other sign of which one happened — so it is said
// plainly whenever there is one, whether or not the run also failed.
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
		// No resume point and a failure means the run never got far enough to have one —
		// a registry that would not load, say. Claiming `done` there would be a lie about
		// the whole table.
		return fmt.Sprintf("backfill-derive stopped before it could derive anything: scanned=%d", pass.Scanned), exit
	default:
		return fmt.Sprintf("backfill-derive done: scanned=%d updated=%d slugs_moved=%d companies_orphaned=%d (follow with a reindex)",
			pass.Scanned, pass.Updated, pass.SlugsMoved, orphaned), exit
	}
}

// deriveRow re-derives a job's facets, role_fingerprint, and slugs, and reports
// whether the derived values differ from what is stored (changed → a write is needed)
// and whether specifically a slug moved (slugMoved → the companies catalogue needs
// re-keying). The fingerprint is computed from the freshly derived company_slug, so
// the result matches what cmd/ingest/store.go writes for the same raw fields. Pure —
// safe to call concurrently.
func deriveRow(j db.Job, canon map[string]string) (params db.UpdateJobDerivedParams, changed, slugMoved bool) {
	d := jobderive.Derive(jobderive.Input{
		Title:       j.Title,
		Company:     j.Company,
		Source:      j.Source,
		ExternalID:  j.ExternalID,
		Location:    j.Location,
		Description: j.Description,
		WorkMode:    j.WorkMode, // preserves a set work_mode (jobderive precedence)
		// Recovers the ingest-time IsTechHint from the stored row, which never re-crawls
		// to learn it fresh: without this, a Profession itdev/itops row whose title the
		// dictionary cannot resolve would have is_tech silently rederived back to
		// unknown on every pass (issue #2601).
		IsTechHint: sources.ProfessionConfirmsTech(j.Source, j.ExternalID),
	})
	// jobderive is pure, so it re-derives the spelling the SOURCE used. Resolving through the
	// alias registry here is what stops a backfill silently undoing every merge — and it must
	// happen before the fingerprint, which is computed from the company slug.
	if canonical, ok := canon[normalize.FoldSlug(d.CompanySlug)]; ok {
		d.CompanySlug = canonical
	}
	fingerprint := jobhash.RoleFingerprint(db.UpsertJobParams{
		CompanySlug: d.CompanySlug,
		Title:       j.Title,
		Description: j.Description,
	})
	experience := pgconv.Int4(d.ExperienceYearsMin)
	isTech := pgconv.Bool(d.IsTech)
	requiresClearance := pgconv.Bool(d.RequiresClearance)

	facetsMoved := !slices.Equal(d.Countries, j.Countries) || !slices.Equal(d.Regions, j.Regions) || !slices.Equal(d.Cities, j.Cities) ||
		d.WorkMode != j.WorkMode || !slices.Equal(d.Skills, j.Skills) ||
		d.Seniority != j.Seniority ||
		d.Category != j.Category ||
		isTech != j.IsTech ||
		requiresClearance != j.RequiresClearance ||
		d.PostingLanguage != j.PostingLanguage ||
		d.EmploymentType != j.EmploymentType ||
		d.EducationLevel != j.EducationLevel ||
		d.EnglishLevel != j.EnglishLevel ||
		experience != j.ExperienceYearsMin
	fingerprintMoved := fingerprint != j.RoleFingerprint.String
	slugMoved = d.PublicSlug != j.PublicSlug || d.CompanySlug != j.CompanySlug

	return db.UpdateJobDerivedParams{
		ID:                 j.ID,
		Countries:          d.Countries,
		Regions:            d.Regions,
		Cities:             d.Cities,
		WorkMode:           d.WorkMode,
		Skills:             d.Skills,
		Seniority:          d.Seniority,
		Category:           d.Category,
		IsTech:             isTech,
		RequiresClearance:  requiresClearance,
		PostingLanguage:    d.PostingLanguage,
		EmploymentType:     d.EmploymentType,
		EducationLevel:     d.EducationLevel,
		EnglishLevel:       d.EnglishLevel,
		ExperienceYearsMin: experience,
		RoleFingerprint:    pgtype.Text{String: fingerprint, Valid: true},
		PublicSlug:         d.PublicSlug,
		CompanySlug:        d.CompanySlug,
	}, facetsMoved || fingerprintMoved || slugMoved, slugMoved
}

// scanWindow bounds one run of the pass.
//
// It exists because the pass walks 12.7M rows and de-TOASTs a description for each,
// which outlasts any timeout it is sensible to give a unit — so a run has to be able to
// stop and be continued. See AGENTS.md for the nightly arrangement that could not.
type scanWindow struct {
	// fromID is the first id a run must DO, inclusive — the same sense as the sibling
	// pass's BACKFILL_REQUIREMENTS_FROM_ID. Two knobs sharing a name and a shape while
	// disagreeing about whether the id is done or to-do lose exactly one row per hop for
	// an operator who chains them.
	fromID int64
	// maxRows bounds how many rows one run scans. Zero is unbounded — never a literal
	// 0 from an operator, since worker.EnvInt64 refuses anything but a positive value,
	// so "unset" is the only way to ask for the whole table.
	maxRows int64
	// closedSince narrows the scan to the rows that can still reach the catalogue: open
	// postings, plus ones closed at or after it. The zero time means the whole table,
	// which is the default and the only behaviour this pass had before.
	//
	// Why a cutoff and not `closed_at IS NULL`: a closed posting reopens without its
	// facets being rewritten (ingest's Toucher), so one skipped on "closed" drifts back
	// into the catalogue carrying whatever the old dictionary gave it. The cutoff is the
	// operator's statement of how far back a posting might still come from.
	closedSince time.Time
}

// pendingRows are the rows handed to the worker pool that have not finished.
//
// It exists because the reader legitimately runs AHEAD of the pool: jobsCh is buffered at
// backfillBatchSize, so the reader can hand over a whole page — and advance its own keyset
// cursor past it — before a worker has written any of it. Resuming from the reader's cursor
// therefore skips however much of the backlog was still queued when the run stopped, and a
// skipped row keeps stale facets for good with nothing downstream reporting it. Measured on
// the review's own test: the reader's cursor said 500 while id 5 had not been written.
//
// So the resume point comes from the OLDEST row still unfinished, not from the newest row
// read. A row is added when the reader hands it over and removed when a worker is done with
// it — including the common case of deciding it needs no write. A row whose write FAILED
// stays, which is what makes the resume point fall before it.
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

// oldest is the lowest id still unfinished. Read once, after the pool has joined, so the
// linear scan over at most backfillBatchSize+concurrency entries costs nothing.
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

// backfillRun is what one bounded pass did, and where the next one starts.
type backfillRun struct {
	Scanned, Updated, SlugsMoved int
	// ResumeID is the id a follow-up run must be given as fromID — the first row this
	// one did not finish. Zero means the pass reached the end of the table with nothing
	// outstanding: an operator needs to tell "there is more" from "that was all" without
	// counting rows themselves, and must be able to trust the answer either way.
	ResumeID int64
}

// backfillPass re-derives every job's facets, fingerprint, and slugs within win and
// rewrites the rows whose derived values differ from what is stored. A single reader pages
// by keyset (id > last seen) so concurrent writes cannot skip or repeat rows, and a pool of
// `concurrency` workers derives and writes in parallel (order-independent). It reports how
// many rows were written (Updated) and how many of those moved a slug (SlugsMoved), so the
// caller knows whether to reconcile companies, and where a follow-up run must start. The
// first store error cancels the run and is returned.
//
// This is backfillWindow with the production progress log attached, and the entry point
// main uses — so it is also the one the tests exercise. It is not "bounded": win may be
// empty, and then it is the whole table.
func backfillPass(ctx context.Context, store deriveStore, concurrency int64, win scanWindow) (backfillRun, error) {
	start := time.Now()
	return backfillWindow(ctx, store, concurrency, win, progressEvery, func(scanned, updated, slugs int64) {
		log.Printf("backfill-derive: scanned %d, updated %d, slugs_moved %d, %s elapsed",
			scanned, updated, slugs, time.Since(start).Round(time.Second))
	})
}

// backfillProgress is backfillAll with the reporting cadence and sink injected, so the
// cadence is a tested property rather than something only prod can demonstrate.
//
// report fires ON each multiple of every, from whichever worker happens to cross it —
// the counter is atomic, so exactly one goroutine observes each multiple whatever the
// concurrency. It runs on the worker's goroutine, so it must stay cheap.
// loadAliasRegistry reads company_slug_aliases into a folded-key lookup.
//
// A backfill that could not read it must FAIL rather than proceed: an empty registry looks
// exactly like a catalogue with no merges, and proceeding would rewrite every merged posting
// back to its source spelling — the one failure mode this guard exists for.
func loadAliasRegistry(ctx context.Context, store deriveStore) (map[string]string, error) {
	rows, err := store.ListCompanySlugAliases(ctx)
	if err != nil {
		return nil, fmt.Errorf("load company slug aliases: %w", err)
	}
	canon := make(map[string]string, len(rows))
	for _, r := range rows {
		if _, seen := canon[r.FoldedKey]; !seen {
			canon[r.FoldedKey] = r.CanonicalSlug
		}
	}
	log.Printf("backfill-derive: company alias registry loaded (%d folded keys)", len(canon))
	return canon, nil
}

func backfillWindow(ctx context.Context, store deriveStore, concurrency int64, win scanWindow, every int64, report func(scanned, updated, slugsMoved int64)) (backfillRun, error) {
	if concurrency < 1 {
		concurrency = 1
	}

	// Loaded ONCE for the whole run rather than per row: the registry is small (one entry per
	// retired slug) and the pool re-derives millions of rows. Read before any worker starts,
	// so every row in a run resolves against the same registry.
	canon, err := loadAliasRegistry(ctx, store)
	if err != nil {
		return backfillRun{}, err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var scannedN, updatedN, slugsN int64
	var errOnce sync.Once
	var runErr error
	fail := func(e error) {
		errOnce.Do(func() { runErr = e; cancel() })
	}

	jobsCh := make(chan db.Job, backfillBatchSize)

	// Reader (producer): pages the table by keyset and feeds the channel.
	//
	// It reads through worker.ResilientPage because this pass has no resume point: a row
	// whose TOAST is damaged fails the whole wide SELECT, and an aborting scan would
	// re-fail at the same id on every later run, leaving every derived column past it
	// stale for good. The helper degrades to reading the faulting window row by row and
	// skips only what is genuinely unreadable.
	// A bounded window reads only what can still surface; the zero cutoff keeps the
	// whole-table reader this pass has always used. Both satisfy the same PageReader, so
	// the corruption-degrade path below is identical either way.
	reader := worker.NewFullScanReader(store)
	if !win.closedSince.IsZero() {
		reader = worker.NewLiveScanReader(store, win.closedSince)
	}
	// Where the reader's own cursor was when it gave up, written by the reader goroutine
	// alone and read only after readerWG.Wait(). Zero means it reached the end of the
	// table. It is a CEILING on the resume point, not the resume point itself — what the
	// run actually finished is what pending knows.
	var stoppedAt int64
	pending := newPendingRows()
	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		defer close(jobsCh)
		// Owned by this goroutine alone, so a plain counter is enough. Reported once at
		// the end rather than per page: an operator needs the total, and each skipped id
		// is already logged where it was met.
		var skipped int
		defer func() {
			if skipped > 0 {
				log.Printf("backfill-derive: skipped %d corrupted row(s); their derived columns are unchanged", skipped)
			}
		}()
		// The reader's keyset is strictly-greater, while fromID names the first id to DO,
		// so the cursor starts one below it. fromID 0 means "from the beginning", and
		// there is no id 0 to lose.
		afterID := win.fromID - 1
		if win.fromID == 0 {
			afterID = 0
		}
		var fed int64
		for {
			// Narrow the last page to what is left of the budget, so a run stops AT
			// maxRows rather than up to one batch past it. Checking only between pages
			// would overshoot by backfillBatchSize, which makes the knob approximate —
			// and the knob's whole job is to fit the run inside a unit's timeout.
			batch := int32(backfillBatchSize)
			if win.maxRows > 0 {
				left := win.maxRows - fed
				if left <= 0 {
					// The budget is spent, but that is not yet news: a budget that lands
					// exactly on the last row means the table IS fully derived, and
					// reporting "not fully derived" there sends an operator back for a
					// pass with nothing in it. One indexed row settles which it was.
					rest, _, _, e := worker.ResilientPage(ctx, reader, afterID, 1)
					if e != nil {
						fail(e)
						return
					}
					if len(rest) > 0 {
						stoppedAt = afterID
					}
					return
				}
				if left < int64(batch) {
					batch = int32(left)
				}
			}

			jobs, lastID, corrupted, e := worker.ResilientPage(ctx, reader, afterID, batch)
			if e != nil {
				stoppedAt = afterID
				fail(e)
				return
			}
			skipped += len(corrupted)
			for i := range jobs {
				// Registered BEFORE the hand-over, so a row sitting in the channel's
				// buffer counts as unfinished. Registering it in the worker instead would
				// leave the whole buffered backlog invisible, which is the gap this
				// bookkeeping exists to close.
				pending.add(jobs[i].ID)
				select {
				case jobsCh <- jobs[i]:
				case <-ctx.Done():
					stoppedAt = afterID
					return
				}
			}
			fed += int64(len(jobs))
			// Keyset progress is the exhaustion signal. A "< batchSize" test would end
			// the scan at the first corrupted row, because the degrade path returns a
			// legitimately short page whenever it skips one.
			if lastID == afterID {
				return
			}
			afterID = lastID
		}
	}()

	// Workers (consumers): derive + write in parallel.
	var workerWG sync.WaitGroup
	for i := int64(0); i < concurrency; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for j := range jobsCh {
				n := atomic.AddInt64(&scannedN, 1)
				if every > 0 && n%every == 0 && report != nil {
					report(n, atomic.LoadInt64(&updatedN), atomic.LoadInt64(&slugsN))
				}
				params, changed, slugMoved := deriveRow(j, canon)
				if !changed {
					// Finished: a row whose derived values already match needs no write,
					// and holding it pending would drag the resume point back over work
					// that is genuinely done.
					pending.done(j.ID)
					continue
				}
				if e := store.UpdateJobDerived(ctx, params); e != nil {
					// Deliberately NOT marked done — this row is the reason the run is
					// stopping, so the resume point must land before it.
					fail(e)
					return
				}
				pending.done(j.ID)
				atomic.AddInt64(&updatedN, 1)
				if slugMoved {
					atomic.AddInt64(&slugsN, 1)
				}
			}
		}()
	}

	workerWG.Wait()
	readerWG.Wait()

	// Both are read only here, after the reader has returned and the pool has joined, so
	// no synchronisation beyond those joins is needed.
	//
	// The oldest unfinished row wins over the reader's cursor whenever there is one. A
	// run that stopped cleanly at the end of the table can still hold pending rows — the
	// pool abandons its backlog the moment one write fails — and reporting `done` there
	// would leave them stale for good.
	resumeID := stoppedAt + 1
	if stoppedAt == 0 {
		resumeID = 0
	}
	if oldest, ok := pending.oldest(); ok && (resumeID == 0 || oldest < resumeID) {
		resumeID = oldest
	}
	return backfillRun{
		Scanned:    int(scannedN),
		Updated:    int(updatedN),
		SlugsMoved: int(slugsN),
		ResumeID:   resumeID,
	}, runErr
}
