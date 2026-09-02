package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/platform/db"
)

// coverageFreshness is how recently a non-aggregator posting must have been SEEN for its
// company to count as covered — i.e. for ingest to discard an aggregator's posting for that
// company unsaved.
//
// It is deliberately NOT sources.DefaultSweepGrace (48h), because the sweep and this gate ask
// different questions. The sweep asks whether a posting is still listed on its board; the gate
// asks whether the catalogue still crawls this employer at all. A board goes legitimately
// uncrawled for far longer than a posting goes legitimately unlisted — the fleet skips runs.
//
// Measured on prod 2026-09-02 (1% sample of open postings), share of rows seen within a window:
//
//	source           ≤48h    ≤7d    ≤14d    ≤30d
//	workday          73.6%  85.1%  92.8%   99.5%   (largest by volume)
//	smartrecruiters  74.2%  90.5%  90.5%   91.8%
//	greenhouse       98.5%  99.2%  99.2%   99.4%
//	lever/ashby/…    96-99%   ~98%   ~98%   ~99%
//
// At 48h the largest provider loses a quarter of its live rows to crawl jitter and the gate
// stops crediting real coverage on 43,649 slugs. At 14 days every provider is at or above
// 92.8%, so what falls outside the window is a board that has genuinely stopped being crawled
// rather than one that is merely between runs — which is exactly the 22,022 slugs held covered
// by rows nobody had seen in a fortnight (issue #2315).
//
// Not configurable. The number is a property of the fleet's crawl cadence, not of how a
// particular cron was invoked, and a per-run override would make the gate's behaviour depend
// on the latter. Re-measure and change it here.
const coverageFreshness = 14 * 24 * time.Hour

// coverageBatchSize bounds how many companies one statement asks about. The query costs one
// index search per company (measured on prod: ~11 open rows touched each), so the bound is
// about keeping a single statement's work predictable rather than about a payload limit.
const coverageBatchSize = 500

// coverage answers pipeline.CoverageLookup from Postgres: which of these companies still have
// an OPEN, RECENTLY SEEN posting from a source that is not an aggregator.
//
// Postgres and not the search index, because the index cannot hold the answer. The incremental
// drain pushes a document only when its content_hash moves, and last_seen_at is not in that
// hash — worse, the write that stamps it on the common path (RefreshUnchangedJob) deliberately
// writes that column alone and enqueues nothing. An indexed copy would therefore freeze at the
// last content change and be most wrong for exactly the actively-crawled rows the gate must
// credit.
//
// ask is a field rather than a direct call so the fold-and-credit mapping either side of the
// query is testable without a database.
type coverage struct {
	ask func(ctx context.Context, folded, aggregators []string, seenAfter time.Time) ([]string, error)
}

// newCoverage wires the gate to the ingest pool, alongside newDBStore and newBoardHealth.
func newCoverage(pool *pgxpool.Pool) *coverage {
	q := db.New(pool)
	return &coverage{
		ask: func(ctx context.Context, folded, aggregators []string, seenAfter time.Time) ([]string, error) {
			return q.CompaniesWithFreshNonAggregatorCoverage(ctx, db.CompaniesWithFreshNonAggregatorCoverageParams{
				FoldedCompanies: folded,
				Aggregators:     aggregators,
				SeenAfter:       pgtype.Timestamptz{Time: seenAfter, Valid: true},
			})
		},
	}
}

// NonAggregatorCompanies implements pipeline.CoverageLookup. companySlugs arrive as the
// alias-resolved slugs the upsert would store, and the answer is keyed by those same values —
// the fold is this type's business and never leaks out of it.
func (c *coverage) NonAggregatorCompanies(ctx context.Context, companySlugs, aggregators []string) (map[string]bool, error) {
	covered := make(map[string]bool)
	seenAfter := time.Now().Add(-coverageFreshness)
	folded, owners := foldedBatch(companySlugs)
	for start := 0; start < len(folded); start += coverageBatchSize {
		end := min(start+coverageBatchSize, len(folded))
		answer, err := c.ask(ctx, folded[start:end], aggregators, seenAfter)
		if err != nil {
			return nil, fmt.Errorf("ingest: aggregator coverage lookup: %w", err)
		}
		// Credited through owners, never straight from the answer: one folded value can own
		// several spellings, and a value the caller never asked about must not become a key.
		for _, f := range answer {
			for _, slug := range owners[f] {
				covered[slug] = true
			}
		}
	}
	return covered, nil
}

// foldedBatch returns the distinct folded spellings to ask about, plus the reverse index that
// credits a folded answer back to every slug that folds to it ("cfo-insights" and "cfoinsights"
// both fold to "cfoinsights"). A slug folding to "" — nothing but hyphens — is left out: as a
// filter value it would match rows whose own fold is empty, which is coverage for an employer
// nobody named.
func foldedBatch(companySlugs []string) (folded []string, owners map[string][]string) {
	owners = make(map[string][]string, len(companySlugs))
	for _, slug := range companySlugs {
		f := normalize.FoldSlug(slug)
		if f == "" {
			continue
		}
		if _, seen := owners[f]; !seen {
			folded = append(folded, f)
		}
		owners[f] = append(owners[f], slug)
	}
	return folded, owners
}
