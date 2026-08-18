// Command backfill-derive re-derives, in a single keyset pass over the whole jobs
// table, every column that ingest computes as a pure function of a row's own raw
// fields: the deterministic dictionary facets (countries, regions, cities, work_mode,
// skills, seniority, category, is_tech, and the synthetic enrichment facets
// posting_language, employment_type, education_level, english_level,
// experience_years_min — all from jobderive.Derive), the repost-identity
// role_fingerprint (internal/jobhash), and the public_slug/company_slug
// (internal/normalize, via jobderive). It replaces the three former one-shots
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
// a moderator- or submission-authored vacancy may STATE them (internal/moderation passes them as
// authoritative structured geography), and this command re-derives both from the free-text
// location — which for such a row usually yields nothing. Unlike getmatch, a manual job is never
// crawled again, so nothing restores them.
//
// That is deliberate rather than an oversight, and it is not this command's decision to make
// alone: internal/moderation's own edit path already re-derives every facet from content and
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
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/worker"
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

	queries := db.New(pool)
	concurrency := backfillConcurrency()
	log.Printf("backfill-derive starting: concurrency=%d", concurrency)
	scanned, updated, slugsMoved, err := backfillAll(ctx, queries, concurrency)
	if err != nil {
		log.Printf("backfill-derive: %v", err)
		return 1
	}

	// A slug rewrite re-keys jobs.company_slug; reconcile the derived companies
	// catalogue to match (and drop rows orphaned by the change) so company pages
	// resolve. Skip the whole-table sync when no slug moved.
	var orphaned int64
	if slugsMoved > 0 {
		if err := queries.SyncCompaniesFromJobs(ctx); err != nil {
			log.Printf("backfill-derive: sync companies: %v", err)
			return 1
		}
		orphaned, err = queries.DeleteOrphanCompanies(ctx)
		if err != nil {
			log.Printf("backfill-derive: delete orphan companies: %v", err)
			return 1
		}
	}

	log.Printf("backfill-derive done: scanned=%d updated=%d slugs_moved=%d companies_orphaned=%d (follow with a reindex)",
		scanned, updated, slugsMoved, orphaned)
	return 0
}

// backfillConcurrency reads the worker-pool size from BACKFILL_CONCURRENCY,
// defaulting to 1 (the original single-threaded pass) for any unset/invalid value.
func backfillConcurrency() int {
	if n, err := strconv.Atoi(os.Getenv("BACKFILL_CONCURRENCY")); err == nil && n > 0 {
		return n
	}
	return 1
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

	facetsMoved := !slices.Equal(d.Countries, j.Countries) || !slices.Equal(d.Regions, j.Regions) || !slices.Equal(d.Cities, j.Cities) ||
		d.WorkMode != j.WorkMode || !slices.Equal(d.Skills, j.Skills) ||
		d.Seniority != j.Seniority ||
		d.Category != j.Category ||
		isTech != j.IsTech ||
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

// backfillAll re-derives every job's facets, fingerprint, and slugs and rewrites the
// rows whose derived values differ from what is stored. A single reader pages by keyset
// (id > last seen) so concurrent writes cannot skip or repeat rows, and a pool of
// `concurrency` workers derives and writes in parallel (order-independent). It reports
// how many rows were written (updated) and how many of those moved a slug (slugsMoved),
// so the caller knows whether to reconcile companies. The first store error cancels the
// run and is returned.
func backfillAll(ctx context.Context, store deriveStore, concurrency int) (scanned, updated, slugsMoved int, err error) {
	start := time.Now()
	return backfillProgress(ctx, store, concurrency, progressEvery, func(scanned, updated, slugs int64) {
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

func backfillProgress(ctx context.Context, store deriveStore, concurrency int, every int64, report func(scanned, updated, slugsMoved int64)) (scanned, updated, slugsMoved int, err error) {
	if concurrency < 1 {
		concurrency = 1
	}

	// Loaded ONCE for the whole run rather than per row: the registry is small (one entry per
	// retired slug) and the pool re-derives millions of rows. Read before any worker starts,
	// so every row in a run resolves against the same registry.
	canon, err := loadAliasRegistry(ctx, store)
	if err != nil {
		return 0, 0, 0, err
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
	reader := worker.NewFullScanReader(store)
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
		var afterID int64
		for {
			jobs, lastID, corrupted, e := worker.ResilientPage(ctx, reader, afterID, backfillBatchSize)
			if e != nil {
				fail(e)
				return
			}
			skipped += len(corrupted)
			for i := range jobs {
				select {
				case jobsCh <- jobs[i]:
				case <-ctx.Done():
					return
				}
			}
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
	for i := 0; i < concurrency; i++ {
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
					continue
				}
				if e := store.UpdateJobDerived(ctx, params); e != nil {
					fail(e)
					return
				}
				atomic.AddInt64(&updatedN, 1)
				if slugMoved {
					atomic.AddInt64(&slugsN, 1)
				}
			}
		}()
	}

	workerWG.Wait()
	readerWG.Wait()
	return int(scannedN), int(updatedN), int(slugsN), runErr
}
