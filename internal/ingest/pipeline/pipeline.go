// Package pipeline is the ingest write path: it dispatches each configured board to
// its source adapter, normalizes the postings, and persists them. It is independent
// of the DB layer (via Store) and of any specific platform (via the source registry).
package pipeline

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// Runner drives ingest: for each configured board it looks up the adapter, fetches,
// normalizes, and saves. Boards run concurrently up to defaultConcurrency; a board
// failure is isolated and never aborts the run.
type Runner struct {
	Registry map[string]sources.Source
	Store    Store
	// Outbound routes non-native LinkedIn discoveries into the destination ATS.
	// They are never written as public LinkedIn postings, even without a resolver.
	Outbound OutboundResolver
	// BoardHealth is optional (nil disables per-board cooldown/health recording).
	BoardHealth BoardHealth
	// Aliases is optional (nil keeps every posting on its derived company slug).
	Aliases AliasLookup
	// RecentWindow, when positive, makes this a recent-only run: each board is read through its
	// adapter's FetchRecent for postings published within the window, and a board whose adapter
	// has no date filter is skipped (Stats.RecentUnsupported) instead of crawled in full. A
	// recent listing omits every older posting by construction, so such a run records no board
	// health at all — neither success, yield nor failure — and never recovers a cooled provider.
	// Zero is the ordinary full crawl.
	RecentWindow time.Duration
	// WriteReserve, when positive, ends every board's fetch that long ahead of the run's context
	// deadline, so a board the deadline catches mid-crawl still writes what it read. Without it
	// the fetch and the writes ended together: a board too big for one run read thousands of
	// postings, saved none on the ended context, and the next run started it from zero again.
	// No effect on a context without a deadline.
	WriteReserve time.Duration
}

func (r Runner) recent() bool { return r.RecentWindow > 0 }

// fetchContext is what a board's fetch runs on: ctx, ending WriteReserve ahead of its deadline.
func (r Runner) fetchContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if dl, ok := ctx.Deadline(); ok && r.WriteReserve > 0 {
		return context.WithDeadline(ctx, dl.Add(-r.WriteReserve))
	}
	return context.WithCancel(ctx)
}

// AliasLookup resolves folded company slugs to the canonical slug a merge elected for that
// employer (company_slug_aliases). Keyed on the FOLD rather than on the slug so a spelling
// that was never itself merged still lands: "DollarTree" has no alias row, but it folds onto
// "dollartree", which does.
//
// Keys absent from the returned map have no canon and keep their derived slug. A lookup
// failure is not fatal — see companyCanonical.
type AliasLookup interface {
	CanonicalCompanySlugs(ctx context.Context, foldedKeys []string) (map[string]string, error)
}

// companyCanonical maps a derived slug to its canonical employer. Nil keeps the derived slug.
// Buffered boards resolve one batch; streaming boards memoize each company as it arrives.
type companyCanonical func(derivedSlug string) string

// resolveAll canonicalizes and deduplicates the company scope a stale sweep must withhold.
// It rewrites the caller-owned derived slice in place.
func resolveAll(c companyCanonical, derived []string) []string {
	for i, s := range derived {
		derived[i] = canonicalOf(c, s)
	}
	return slices.Compact(slices.Sorted(slices.Values(derived)))
}

// canonicalOf returns the canonical slug, retaining the derived one when aliases are unwired.
func canonicalOf(c companyCanonical, derived string) string {
	if c == nil {
		return derived
	}
	return c(derived)
}

// Run ingests every configured board and returns the stats per provider. It returns an
// error only for a context cancellation, never for a single board's failure. All boards
// run in one bounded concurrent pool regardless of provider, so a slow self-pacing
// provider occupies one slot without blocking the others.
func (r Runner) Run(ctx context.Context, entries []sources.CompanyEntry) (RunStats, error) {
	// Half-open the circuit breaker before the crawl: a provider whose boards were mass-
	// cooled by a since-resolved outage recovers this cycle instead of each board waiting
	// out its own backoff. The board that answered the probe was already fully ingested
	// from that same fetch (see probeProvider) — handled carries its Stats so the loop
	// below does not crawl it a second time.
	handled := r.recoverProviders(ctx, entries)

	// Computed once up front: a board name this run's entries share across more than one
	// region never qualifies for the board scope (see ambiguousRegionBoards) — the close
	// query's pattern cannot distinguish which region proved coverage.
	ambiguous := ambiguousRegionBoards(entries, r.Registry)

	var (
		mu     sync.Mutex
		byProv = RunStats{}
		wg     sync.WaitGroup
	)

	// Heartbeat the crawl progress: boards run concurrently and a successful board
	// logs nothing, so without this a run that stalls on one hung board is silent.
	var crawled atomic.Int64
	total := len(entries)
	stopHeartbeat := worker.Heartbeat(progressInterval, func() {
		log.Printf("ingest: progress %d/%d boards crawled", crawled.Load(), total)
	})
	defer stopHeartbeat()
	sem := make(chan struct{}, defaultConcurrency)
	// No board starts once the fetches are over: it would have nothing to read in.
	fetchCtx, cancelFetch := r.fetchContext(ctx)
	defer cancelFetch()

admit:
	for _, e := range entries {
		// Admit in catalog order so oldest-first scheduling survives a truncated run.
		select {
		case sem <- struct{}{}:
		case <-fetchCtx.Done():
			break admit
		}
		if fetchCtx.Err() != nil {
			<-sem
			break
		}
		wg.Add(1)
		go func(e sources.CompanyEntry) {
			defer wg.Done()

			defer func() { <-sem }()
			if fetchCtx.Err() != nil {
				return
			}

			boardStats, already := handled[boardKey{e.Provider, e.Board, e.Region}]
			if !already {
				boardStats = r.ingestBoard(ctx, e)
			}
			if !ambiguous[providerBoard{e.Provider, e.Board}] {
				switch {
				case boardQualifies(e, boardStats):
					boardStats.QualifyingBoards = append(boardStats.QualifyingBoards, BoardRef{e.Board, e.Region})
				case boardListedEmpty(e, boardStats):
					boardStats.EmptyBoards = append(boardStats.EmptyBoards, BoardRef{e.Board, e.Region})
				}
			}
			crawled.Add(1)

			mu.Lock()
			s := byProv[e.Provider]
			s.add(boardStats)
			byProv[e.Provider] = s
			mu.Unlock()
		}(e)
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return byProv, err
	}
	// The fetch window closing on a board, or ahead of one, is the deadline cutting the run
	// short even though the writes after it finished in time.
	if err := fetchCtx.Err(); err != nil && (byProv.Total().Interrupted > 0 || int(crawled.Load()) < total) {
		return byProv, fmt.Errorf("the fetch window closed before every board was read: %w", err)
	}
	// A board interrupted while the context is still alive met a limiter's early deadline
	// refusal (limiterOutOfRun). The run did not crawl it, so it reports itself cut short like
	// any run its deadline ended, and cmd/ingest narrows its sweep accordingly.
	if byProv.Total().Interrupted > 0 {
		return byProv, errLimiterOutOfRun
	}
	return byProv, nil
}
