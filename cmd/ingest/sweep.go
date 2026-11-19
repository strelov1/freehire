package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	sweeppolicy "github.com/strelov1/freehire/internal/ingest/sweep"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// crawlReachedNothing reports a run that tried and wrote nothing: boards or saves failed and
// no posting was saved or refreshed (host blocked, database unreachable, schema drift). A few
// failing boards among thousands are not that and must not turn a clean crawl into exit 1;
// those are board_health's to report.
func crawlReachedNothing(total pipeline.Stats) bool {
	return total.Ingested == 0 && total.Refreshed == 0 && (total.Failed > 0 || total.Skipped > 0)
}

// sweep runs the post-crawl stale-job closes for every provider in runStats and returns how many
// of them failed. ctx must be the sweep's own context (runBudget.sweepContext), never the crawl's.
// truncated narrows every provider to the board scope — see the comment at its call site.
func sweep(ctx context.Context, queries *db.Queries, registry map[string]sources.Source, runStats pipeline.RunStats, crawled *crawledSet, crossShardAmbiguousBoards map[string]bool, truncated, recent bool) int {
	// A recent-only listing omits older postings by construction, so it proves nothing about
	// what is gone: no scope at all.
	if recent {
		return 0
	}
	var failed int
	now := time.Now()
	// A slice-crawled source (e.g. whatjobs) declares a window wider than staleAfter: its crawl
	// reaches only a keyword's first pages, so deeper postings read as unseen and would be
	// closed and reopened. A full-board source with a tight cadence (e.g. gem) may declare a
	// narrower one, since its unseen reading is positive evidence.
	grace := sources.SweepGraceWindows(registry)
	// A self-closing source (e.g. jobtech) manages its own closes; it re-reports only changed
	// ads, so the unseen sweep would wrongly close every untouched open ad.
	sweepExempt := make(map[string]bool)
	for _, p := range sources.UnseenSweepExemptProviders(registry) {
		sweepExempt[p] = true
	}
	// A fullCatalog source (e.g. habr_career) lists its whole catalogue each run, so a clean run
	// may close its unseen jobs by source alone, retiring a vanished company that the company
	// scope leaks. Gated on a zero-Failed run below, since a truncated crawl must not
	// source-close what it never reached.
	fullCatalog := make(map[string]bool)
	for _, p := range sources.FullCatalogProviders(registry) {
		fullCatalog[p] = true
	}
	// A fullBoardListing provider's adapter is registered as proving it lists a board to
	// completion, so the sweep may close within its boards; see sweeppolicy.SweepableBoards for
	// the full gate.
	fullBoardListing := sources.FullBoardListingProviders(registry)
	// A sliceBoardListing provider (e.g. whatjobs) reads each board only to a page budget, so it
	// cannot prove a full listing — but its grace window absorbs that, so its finished boards
	// close within themselves on that window (see sweepableBoards).
	sliceBoardListing := sources.SliceBoardListingProviders(registry)
	// An emptyBoardListing provider's adapter answers an emptied board distinguishably from a
	// failed fetch, so a finished board that listed nothing joins its board scope too.
	emptyBoardListing := sources.EmptyBoardListingProviders(registry)
	for _, provider := range sweeppolicy.SweepableProviders(runStats) {
		if sweepExempt[provider] {
			continue
		}
		_, hasGrace := grace[provider]
		window := sweeppolicy.SweepWindowFor(grace, provider)
		cutoff := pgconv.Instant(now.Add(-window))
		// Board evidence admits this provider to dispatch without granting the broader
		// company/source scope: that still requires an ingested or refreshed posting.
		if !truncated && sweeppolicy.ShouldSweep(runStats[provider]) {
			bySource := sweeppolicy.SweepBySource(runStats[provider], fullCatalog[provider])
			companySlugs := sweeppolicy.SweepableCompanies(crawled.slugs(provider), runStats[provider])
			closed, skipped, err := sweepProvider(ctx, queries, provider, cutoff, companySlugs, runStats[provider].SkippedBoards, bySource)
			if err != nil {
				// Count and continue: one provider's sweep failure must not skip the rest,
				// but the run still exits non-zero.
				failed++
				log.Printf("close stale jobs (%s): %v", provider, err)
				continue
			}
			if skipped > 0 {
				failed++
				log.Printf("close stale jobs (%s): closed %d, skipped %d unclosable row(s) — see preceding lines for their ids", provider, closed, skipped)
			}
			log.Printf("closed %d stale %s jobs (unseen for %s)", closed, provider, window)
		}

		// Board-scoped close (job-lifecycle spec): the company scope leaks a company whose LAST
		// posting drops off a board this run still crawled, since its slug never re-enters
		// companySlugs. This closes per board the run proved it covered
		// (runStats[provider].QualifyingBoards); see sweeppolicy.SweepableBoards for the gate.
		// Uses the provider's own cutoff: the default for a complete listing, the grace window
		// for a slice.
		boards := sweeppolicy.SweepableBoards(runStats[provider], sweeppolicy.BoardScope{
			HasGrace:          hasGrace,
			FullCatalog:       fullCatalog[provider],
			FullBoardListing:  fullBoardListing[provider],
			SliceBoardListing: sliceBoardListing[provider],
			EmptyBoardListing: emptyBoardListing[provider],
			RegionScoped:      sources.ScopesBoardsByRegion(registry[provider]),
		}, crossShardAmbiguousBoards)
		if len(boards) > 0 {
			boardClosed, boardFailed := sweepBoards(ctx, queries, provider, cutoff, boards, registry[provider])
			log.Printf("closed %d stale %s jobs across %d finished board(s) (unseen for %s)", boardClosed, provider, len(boards), window)
			if boardFailed > 0 {
				failed++
			}
		}
	}
	return failed
}

// sweepBoards closes every qualifying board's unseen jobs in one statement per url scope, one
// scan of the provider's open rows rather than a round trip per board. Every provider but a
// region-scoped one has a single (empty) scope; a region-scoped one (jobstreet) gets one statement
// per region, narrowed to its url prefix (sources.BoardURLPrefix). sweeppolicy.SweepableBoards
// has already cleared the region of every board of an unscoped provider.
func sweepBoards(ctx context.Context, queries *db.Queries, provider string, cutoff pgtype.Timestamptz, boards []pipeline.BoardRef, src sources.Source) (closed int64, failed int) {
	byScope := make(map[string][]string)
	var scopes []string
	for _, b := range boards {
		urlPattern := ""
		if b.Region != "" {
			prefix, ok := sources.BoardURLPrefix(src, b.Region)
			if !ok {
				// The crawl itself fails on a region its adapter does not know, so this board
				// cannot have qualified; refuse rather than close it unscoped.
				failed++
				log.Printf("close stale jobs (%s board %q): no url scope for region %q", provider, b.Board, b.Region)
				continue
			}
			urlPattern = externalid.LikePrefix(prefix)
		}
		if _, seen := byScope[urlPattern]; !seen {
			scopes = append(scopes, urlPattern)
		}
		byScope[urlPattern] = append(byScope[urlPattern], b.Board)
	}
	for _, urlPattern := range scopes {
		n, f := sweepBoardScope(ctx, queries, provider, cutoff, byScope[urlPattern], urlPattern)
		closed += n
		failed += f
	}
	return closed, failed
}

// sweepBoardScope closes the unseen jobs of boards sharing one url scope in one statement. If
// that fails it falls back to one statement per board (see sweepRowByRow), so an unwritable row
// blocks only its own board. failed counts the boards whose close failed.
func sweepBoardScope(ctx context.Context, queries *db.Queries, provider string, cutoff pgtype.Timestamptz, boards []string, urlPattern string) (closed int64, failed int) {
	patterns := make([]string, len(boards))
	for i, board := range boards {
		patterns[i] = externalid.BoardPattern(board)
	}
	closed, err := queries.CloseUnseenJobsForBoards(ctx, db.CloseUnseenJobsForBoardsParams{
		Source:        provider,
		Cutoff:        cutoff,
		BoardPatterns: patterns,
		UrlPattern:    urlPattern,
	})
	if err == nil {
		return closed, 0
	}
	// An ended context would fail every per-board statement the same way; say so once.
	if ctx.Err() != nil {
		log.Printf("close stale jobs (%s, %d boards): %v", provider, len(boards), err)
		return 0, len(boards)
	}
	log.Printf("close stale jobs (%s, %d boards): batched close failed (%v), falling back to one board at a time", provider, len(boards), err)
	for i, board := range boards {
		n, err := queries.CloseUnseenJobsForBoard(ctx, db.CloseUnseenJobsForBoardParams{
			Source:       provider,
			Cutoff:       cutoff,
			BoardPattern: patterns[i],
			UrlPattern:   urlPattern,
		})
		if err != nil {
			failed++
			log.Printf("close stale jobs (%s board %q): %v", provider, board, err)
			continue
		}
		closed += n
	}
	return closed, failed
}

// sweepProvider closes one provider's unseen jobs: the bulk UPDATE (CloseUnseenJobs or
// CloseUnseenJobsBySource) is the fast path; on error it falls back to sweepRowByRow so one
// unwritable row doesn't block the provider. skipped is 0 unless the fallback ran.
// skippedBoards (pipeline.Stats.SkippedBoards) are left out of either scope: the run did not
// look at them, so their rows' age says nothing about whether they are still listed.
func sweepProvider(ctx context.Context, queries *db.Queries, provider string, cutoff pgtype.Timestamptz, companySlugs, skippedBoards []string, bySource bool) (closed int64, skipped int, err error) {
	if bySource {
		closed, err = queries.CloseUnseenJobsBySource(ctx, db.CloseUnseenJobsBySourceParams{
			Source:               provider,
			Cutoff:               cutoff,
			SkippedBoardPatterns: skippedBoards,
		})
	} else {
		closed, err = queries.CloseUnseenJobs(ctx, db.CloseUnseenJobsParams{
			Source:               provider,
			Cutoff:               cutoff,
			CompanySlugs:         companySlugs,
			SkippedBoardPatterns: skippedBoards,
		})
	}
	if err == nil {
		return closed, 0, nil
	}
	log.Printf("close stale jobs (%s): bulk close failed (%v), falling back to row-by-row", provider, err)
	closed, skipped, err = sweepRowByRow(ctx, queries, provider, cutoff, companySlugs, skippedBoards, bySource)
	if err != nil {
		return 0, 0, fmt.Errorf("row-by-row fallback failed: %w", err)
	}
	return closed, skipped, nil
}

// sweepRowByRow is the bulk sweep's fallback when the single-statement UPDATE fails: it
// fetches the same candidate set and closes each row in its own statement, so one row Postgres
// can't write (e.g. a corrupted jobs_pkey value) is skipped by itself instead of aborting the
// provider. Slower than the bulk path; it only runs after the fast path failed.
func sweepRowByRow(ctx context.Context, queries *db.Queries, provider string, cutoff pgtype.Timestamptz, companySlugs, skippedBoards []string, bySource bool) (closed int64, skipped int, err error) {
	var ids []int64
	if bySource {
		ids, err = queries.UnseenJobIDsBySource(ctx, db.UnseenJobIDsBySourceParams{Source: provider, Cutoff: cutoff, SkippedBoardPatterns: skippedBoards})
	} else {
		ids, err = queries.UnseenJobIDs(ctx, db.UnseenJobIDsParams{Source: provider, Cutoff: cutoff, CompanySlugs: companySlugs, SkippedBoardPatterns: skippedBoards})
	}
	if err != nil {
		return 0, 0, fmt.Errorf("list candidates: %w", err)
	}
	for _, id := range ids {
		n, err := queries.CloseUnseenJobByID(ctx, db.CloseUnseenJobByIDParams{ID: id, Cutoff: cutoff})
		if err != nil {
			skipped++
			log.Printf("close stale jobs (%s): skipping id=%d, still unclosable: %v", provider, id, err)
			continue
		}
		closed += n
	}
	return closed, skipped, nil
}
