// Command backfill-derive re-derives the six deterministic dictionary facets —
// countries, regions, work_mode, skills, seniority, category — on existing jobs in
// a single pass, replacing the three separate per-facet backfill commands
// (backfill-geo/-skills/-class). Ingest fills these on every crawl via
// jobderive.Derive; rows that predate a dictionary change — and closed jobs that
// never re-crawl — keep the stale values until this one-off worker rewrites them.
// It pages the whole table and exits. Idempotent for jobs whose facets come from
// the title/location/description dictionaries alone: re-deriving is a pure function
// of those fields, so a second run rewrites nothing.
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
// Slugs are deliberately not touched (re-slugging stays cmd/reslug). work_mode is
// preserved when already set: jobderive keeps a row's existing (possibly
// adapter-structured) work_mode over the parsed-location hint. The other
// structured-source facets are NOT preserved: an adapter that emits a grade,
// category, skills, or required-experience directly (e.g. getmatch) supplies those
// only at ingest, and this command re-derives seniority/category/skills/
// experience_years_min from the stored description columns — so running it
// overwrites such structured values with the dictionary's. This is intentional:
// the command's job is to propagate dictionary changes, which must keep updating
// those facets for the dictionary-derived majority. A boardless adapter like
// getmatch re-supplies the structured facets on its next full crawl.
package main

import (
	"context"
	"log"
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/worker"
)

// toInt4 maps an optional int (experience_years_min) to the pgtype the generated
// params expect; a nil pointer becomes SQL NULL.
func toInt4(n *int) pgtype.Int4 {
	if n == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*n), Valid: true}
}

// backfillBatchSize bounds how many jobs are read per keyset page.
const backfillBatchSize = 500

// facetStore is the slice of the data layer the backfill needs: page the table by
// keyset and rewrite a row's six facet columns. *db.Queries satisfies it; tests
// use a fake. UpdateJobFacets is called concurrently by the worker pool, and
// pgxpool hands each goroutine its own connection, so the store must be safe for
// concurrent use.
type facetStore interface {
	ListJobsByIDAfter(ctx context.Context, arg db.ListJobsByIDAfterParams) ([]db.Job, error)
	UpdateJobFacets(ctx context.Context, arg db.UpdateJobFacetsParams) error
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

	concurrency := backfillConcurrency()
	log.Printf("backfill-derive starting: concurrency=%d", concurrency)
	scanned, updated, err := backfillAll(ctx, db.New(pool), concurrency)
	if err != nil {
		log.Printf("backfill-derive: %v", err)
		return 1
	}
	log.Printf("backfill-derive done: scanned=%d updated=%d", scanned, updated)
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

// deriveFacets re-derives a job's facet columns and reports whether they differ
// from what is stored (i.e. a write is needed). Pure — safe to call concurrently.
func deriveFacets(j db.Job) (db.UpdateJobFacetsParams, bool) {
	d := jobderive.Derive(jobderive.Input{
		Title:       j.Title,
		Company:     j.Company,
		Source:      j.Source,
		ExternalID:  j.ExternalID,
		Location:    j.Location,
		Description: j.Description,
		WorkMode:    j.WorkMode, // preserves a set work_mode (jobderive precedence)
	})
	experience := toInt4(d.ExperienceYearsMin)
	changed := !(slices.Equal(d.Countries, j.Countries) &&
		slices.Equal(d.Regions, j.Regions) &&
		slices.Equal(d.Cities, j.Cities) &&
		d.WorkMode == j.WorkMode &&
		slices.Equal(d.Skills, j.Skills) &&
		d.Seniority == j.Seniority &&
		d.Category == j.Category &&
		d.PostingLanguage == j.PostingLanguage &&
		d.EmploymentType == j.EmploymentType &&
		d.EducationLevel == j.EducationLevel &&
		d.EnglishLevel == j.EnglishLevel &&
		experience == j.ExperienceYearsMin)
	return db.UpdateJobFacetsParams{
		ID:                 j.ID,
		Countries:          d.Countries,
		Regions:            d.Regions,
		Cities:             d.Cities,
		WorkMode:           d.WorkMode,
		Skills:             d.Skills,
		Seniority:          d.Seniority,
		Category:           d.Category,
		PostingLanguage:    d.PostingLanguage,
		EmploymentType:     d.EmploymentType,
		EducationLevel:     d.EducationLevel,
		EnglishLevel:       d.EnglishLevel,
		ExperienceYearsMin: experience,
	}, changed
}

// backfillAll re-derives every job's facet columns and rewrites the rows whose
// derived facets differ from what is stored. A single reader pages by keyset
// (id > last seen) so concurrent writes cannot skip or repeat rows, and a pool of
// `concurrency` workers derives and writes in parallel (order-independent). The
// first store error cancels the run and is returned.
func backfillAll(ctx context.Context, store facetStore, concurrency int) (scanned, updated int, err error) {
	if concurrency < 1 {
		concurrency = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var scannedN, updatedN int64
	var errOnce sync.Once
	var runErr error
	fail := func(e error) {
		errOnce.Do(func() { runErr = e; cancel() })
	}

	jobsCh := make(chan db.Job, backfillBatchSize)

	// Reader (producer): pages the table by keyset and feeds the channel.
	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		defer close(jobsCh)
		var afterID int64
		for {
			jobs, e := store.ListJobsByIDAfter(ctx, db.ListJobsByIDAfterParams{
				AfterID:   afterID,
				BatchSize: backfillBatchSize,
			})
			if e != nil {
				fail(e)
				return
			}
			if len(jobs) == 0 {
				return
			}
			afterID = jobs[len(jobs)-1].ID
			for i := range jobs {
				select {
				case jobsCh <- jobs[i]:
				case <-ctx.Done():
					return
				}
			}
			if len(jobs) < backfillBatchSize {
				return
			}
		}
	}()

	// Workers (consumers): derive + write in parallel.
	var workerWG sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for j := range jobsCh {
				atomic.AddInt64(&scannedN, 1)
				params, changed := deriveFacets(j)
				if !changed {
					continue
				}
				if e := store.UpdateJobFacets(ctx, params); e != nil {
					fail(e)
					return
				}
				atomic.AddInt64(&updatedN, 1)
			}
		}()
	}

	workerWG.Wait()
	readerWG.Wait()
	return int(scannedN), int(updatedN), runErr
}
