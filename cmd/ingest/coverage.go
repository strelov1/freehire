package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// coverageFreshness is how recently a non-aggregator posting must have been SEEN for its
// company to count as covered — i.e. for ingest to discard an aggregator's posting for that
// company unsaved.
//
// It is deliberately NOT sources.DefaultSweepGrace (48h): the sweep asks whether a posting is
// still listed on its board, the gate asks whether the catalogue still crawls this employer at
// all, and the fleet legitimately skips runs. At 48h the largest provider (workday) loses about
// a quarter of its live rows to crawl jitter; at 14 days every provider is above 92%, so what
// falls outside is a board that has genuinely stopped being crawled.
//
// Not configurable: it is a property of the fleet's crawl cadence, and a per-run override
// would make the gate depend on how a cron was invoked.
const coverageFreshness = 14 * 24 * time.Hour

// coverageBatchSize bounds how many companies one statement asks about; the query costs one
// index search per company, so this keeps a statement's work predictable.
const coverageBatchSize = 500

// coverage answers the LinkedIn native-board discovery lookup from Postgres: which of these companies still have
// an OPEN, RECENTLY SEEN posting from a source that is not an aggregator.
//
// Postgres and not the search index: the drain pushes a document only when content_hash
// moves, last_seen_at is not in that hash, and RefreshUnchangedJob writes that column alone
// and enqueues nothing, so an indexed copy would freeze on exactly the actively-crawled rows.
//
// ask is a field so the fold-and-credit mapping is testable without a database.
type coverage struct {
	ask func(ctx context.Context, folded, aggregators, countries []string, seenAfter time.Time) ([]string, error)
}

// newCoverage wires the gate to the ingest pool, alongside newDBStore and newBoardHealth.
func newCoverage(pool *pgxpool.Pool) *coverage {
	q := db.New(pool)
	return &coverage{
		ask: func(ctx context.Context, folded, aggregators, countries []string, seenAfter time.Time) ([]string, error) {
			return q.CompaniesWithFreshNonAggregatorCoverage(ctx, db.CompaniesWithFreshNonAggregatorCoverageParams{
				FoldedCompanies: folded,
				Aggregators:     aggregators,
				Countries:       countries,
				SeenAfter:       pgconv.Instant(seenAfter),
			})
		},
	}
}

// NonAggregatorCompanies implements the LinkedIn native-board discovery lookup. companySlugs are the
// alias-resolved slugs the upsert would store, and the answer is keyed by those same values.
func (c *coverage) NonAggregatorCompanies(ctx context.Context, companySlugs, aggregators, countries []string) (map[string]bool, error) {
	covered := make(map[string]bool)
	seenAfter := time.Now().Add(-coverageFreshness)
	folded, owners := foldedBatch(companySlugs)
	for start := 0; start < len(folded); start += coverageBatchSize {
		end := min(start+coverageBatchSize, len(folded))
		answer, err := c.ask(ctx, folded[start:end], aggregators, countries, seenAfter)
		if err != nil {
			return nil, fmt.Errorf("ingest: aggregator coverage lookup: %w", err)
		}
		// Credit through owners, not the raw answer: one folded value can own several spellings,
		// and a value the caller never asked about must not become a key.
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
// both fold to "cfoinsights"). A slug folding to "" (only hyphens) is left out: as a filter it
// would match rows whose own fold is empty, crediting an employer nobody named.
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
