package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// companyBatchSize bounds how many companies one RecomputeRoleDuplicatesForCompanies /
// SuppressAggregatorDuplicatesForCompanies call covers. One round trip per company made the passes run for
// hours; 500 keeps each WHERE ... = ANY(...) a cheap index probe (migration 0076) while cutting round trips ~500x.
const companyBatchSize = 500

// recomputeRoleDuplicates refreshes jobs.duplicate_of in batches of companies, returning the total rows
// re-marked. Batching avoids both a round trip per company (see companyBatchSize) and one whole-catalogue
// UPDATE whose lock window would stall concurrent ingest writes. Best-effort like every batch here.
//
// It resumes like the fuzzy pass (passCursor): measured 2026-11-21 one walk had grown past 3.5h, and
// restarted from the top every run it never finished inside the 4h unit, nor let the passes after it run.
func recomputeRoleDuplicates(ctx context.Context, q *db.Queries) (int64, error) {
	companies, err := q.CompaniesWithRoleClusters(ctx)
	if err != nil {
		return 0, err
	}
	cursor := &passCursor{store: q, pass: rolePassName}
	return forCompanyBatchesResuming(ctx, cursor, companies, q.RecomputeRoleDuplicatesForCompanies)
}

// suppressAggregatorDuplicates marks each open aggregator posting that duplicates a first-party ATS posting
// (same company, normalized title, compatible country) as a duplicate of that row, in batches of companies,
// returning the total rows re-marked. The aggregator set comes from the taxonomy registry's aggregator()
// markers so it is the same here as on the ingest host: a keyed adapter whose credential lives only where the
// crawl runs must still be classified. Best-effort, lock-scoped and resuming like recomputeRoleDuplicates.
func suppressAggregatorDuplicates(ctx context.Context, q *db.Queries) (int64, error) {
	aggregators := sources.AggregatorProviders(sources.Taxonomy())
	companies, err := q.CompaniesWithAggregatorPostings(ctx, aggregators)
	if err != nil {
		return 0, err
	}
	cursor := &passCursor{store: q, pass: aggregatorPassName}
	return forCompanyBatchesResuming(ctx, cursor, companies, func(ctx context.Context, batch []string) (int64, error) {
		return q.SuppressAggregatorDuplicatesForCompanies(ctx, db.SuppressAggregatorDuplicatesForCompaniesParams{
			FoldedCompanies: foldCompanySlugs(batch),
			Aggregators:     aggregators,
		})
	})
}

// foldCompanySlugs applies the `replace(slug, '-', ”)` fold the aggregator suppression compares on, so the
// query receives an already-folded array.
//
// It exists for the planner: folding in a SQL subquery carries no size estimate and made each batch scan the
// source index (271s per batch); a bare array parameter takes under a second.
//
// Duplicates are left in on purpose: a fold can collide ("cfo-insights", "cfoinsights") and those rows must match.
func foldCompanySlugs(slugs []string) []string {
	folded := make([]string, len(slugs))
	for i, s := range slugs {
		folded[i] = strings.ReplaceAll(s, "-", "")
	}
	return folded
}

// rolePassName and aggregatorPassName key the batched passes' resume points in marker_pass_cursors.
const (
	rolePassName       = "role"
	aggregatorPassName = "aggregator"
)

// forCompanyBatchesResuming is forCompanyBatches over the companies after cursor's saved point, saving
// the last company of every batch it gets past and clearing the point once the walk reaches the end.
func forCompanyBatchesResuming(ctx context.Context, cursor *passCursor, companies []string, fn func(context.Context, []string) (int64, error)) (int64, error) {
	companies, err := cursor.resume(ctx, companies)
	if err != nil {
		return 0, err
	}
	total, err := forCompanyBatches(ctx, companies, fn, func(point string) { cursor.save(ctx, point) })
	return total, errors.Join(err, cursor.finish(ctx))
}

// forCompanyBatches runs fn once per companyBatchSize-sized slice of companies, summing the rows it reports
// re-marked. Batches are independent, so one failure (e.g. a statement timeout) is counted and skipped and
// must not starve the rest; any failure turns the result into an aggregate error (the caller is best-effort).
// Coarser than per-company: a bad batch costs up to companyBatchSize companies their update.
//
// checkpoint receives the last company of each batch the walk gets past — completed or failed, so an
// always-failing batch cannot pin a resumed walk — and never one a cancellation cut off.
func forCompanyBatches(ctx context.Context, companies []string, fn func(context.Context, []string) (int64, error), checkpoint func(lastFinished string)) (int64, error) {
	var total int64
	var done, failures int
	var lastErr error
	for batch := range slices.Chunk(companies, companyBatchSize) {
		n, err := fn(ctx, batch)
		if err != nil {
			// A cancelled context ends the pass instead of counting a failure: the remaining batches would each fail
			// instantly against the same dead context and one deadline would read as hundreds of failed batches.
			// `done`, not `failures`: this runs BEFORE the failure is counted, and a cancellation reports how far it got.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return total, fmt.Errorf("cancelled after %d completed batches: %w", done, ctxErr)
			}
			failures++
			lastErr = fmt.Errorf("batch of %d companies (starting %q): %w", len(batch), batch[0], err)
			checkpoint(batch[len(batch)-1])
			continue
		}
		done++
		total += n
		checkpoint(batch[len(batch)-1])
	}
	if failures > 0 {
		return total, fmt.Errorf("%d batches failed out of %d companies (batch size %d); last: %w",
			failures, len(companies), companyBatchSize, lastErr)
	}
	return total, nil
}

// checkpointEvery is how many companies the finished prefix of a forEachCompany walk must grow by before it
// is checkpointed again. The run is ended by its unit's SIGTERM, so progress is saved along the way.
const checkpointEvery = 1000

// forEachCompany runs fn once per company on `workers` goroutines, summing the rows it reports re-marked.
// Unlike forCompanyBatches there is no query to batch: its only caller (collapseFuzzyDuplicates) does
// per-company work in Go that waits on Postgres far more than it computes, hence several at once. Each
// company's marker write touches only its own rows, so the workers never contend.
//
// Companies are independent, so one failure must not starve the rest — it is counted and
// stepped over, and any failure turns the pass's result into an aggregate error (the
// caller treats the whole pass as best-effort and continues with the prior markers).
//
// A cancelled context ends the walk instead: a company cut off by it is UNFINISHED, not
// failed, and nothing further is dispatched, so one deadline does not read as thousands of failures.
//
// checkpoint receives the resume point: the last company of the longest FINISHED prefix (a stepped-over
// failure counts as finished, so an always-failing company cannot pin the walk). Never the newest company
// finished: with several in flight a later one can finish before an earlier one, and resuming after it
// would skip the earlier one for good. Called from this goroutine only, every checkpointEvery companies of
// progress and once more at the end if the prefix moved.
func forEachCompany(ctx context.Context, companies []string, workers int, fn func(context.Context, string) (int64, error), checkpoint func(lastFinished string)) (int64, error) {
	type result struct {
		i   int
		n   int64
		err error
	}
	indices := make(chan int)
	results := make(chan result)

	go func() {
		defer close(indices)
		for i := range companies {
			select {
			case indices <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	var wg sync.WaitGroup
	for range min(workers, len(companies)) {
		wg.Go(func() {
			for i := range indices {
				n, err := fn(ctx, companies[i])
				results <- result{i: i, n: n, err: err}
			}
		})
	}
	go func() { wg.Wait(); close(results) }()

	var total int64
	var finished, failures int
	var lastErr error
	done := make([]bool, len(companies))
	prefix, saved := 0, 0 // companies[:prefix] are all finished; companies[:saved] were checkpointed
	for r := range results {
		if r.err != nil && ctx.Err() != nil {
			continue // cut off by the cancellation: unfinished, so the resume point stays before it
		}
		done[r.i] = true
		finished++
		if r.err != nil {
			failures++
			lastErr = fmt.Errorf("company %q: %w", companies[r.i], r.err)
		} else {
			total += r.n
		}
		for prefix < len(done) && done[prefix] {
			prefix++
		}
		if prefix-saved >= checkpointEvery {
			checkpoint(companies[prefix-1])
			saved = prefix
		}
	}
	if prefix > saved {
		checkpoint(companies[prefix-1])
	}

	if err := ctx.Err(); err != nil {
		return total, fmt.Errorf("cancelled after %d/%d companies (%d failed): %w", finished, len(companies), failures, err)
	}
	if failures > 0 {
		return total, fmt.Errorf("%d/%d companies failed; last: %w", failures, len(companies), lastErr)
	}
	return total, nil
}
