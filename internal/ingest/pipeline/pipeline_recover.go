package pipeline

import (
	"context"
	"log"
	"sort"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// maxRecoveryProbes bounds how many cooled boards a provider is probed with before a
// run: more than one so a single genuinely-dead board among the cooled set does not mask
// a recovered provider, few enough that the probe stays cheap.
const maxRecoveryProbes = 3

// boardKey identifies one board within a run: the pair a CompanyEntry, a Claimed
// cooldown, and a recovery probe all key on.
type boardKey struct{ provider, board, region string }

// recoverProviders is the provider-level circuit breaker's half-open transition. Before
// the main crawl, for each provider with cooled boards it probes up to maxRecoveryProbes
// of them through the adapter; the first probe that succeeds proves the provider is
// reachable again, so it clears the provider's cooldowns and the run crawls the rest this
// cycle rather than leaving each board to ride out its per-board backoff (up to a day)
// after a resolved provider-wide outage. Every probe failing leaves the provider cooled,
// so a still-down provider is never stampeded. A nil BoardHealth port disables it entirely.
//
// It returns the Stats of any board the probe itself fully ingested (see probeProvider):
// for such a board the caller must not run ingestBoard again this cycle, or "a few cheap
// probes" becomes a full second crawl of the one board that happened to answer.
func (r Runner) recoverProviders(ctx context.Context, entries []sources.CompanyEntry) map[boardKey]Stats {
	handled := make(map[boardKey]Stats)
	if r.BoardHealth == nil {
		return handled
	}
	byProvider := entriesByProvider(entries)
	for _, provider := range sortedProviders(byProvider) {
		src, ok := r.Registry[provider]
		if !ok {
			continue
		}
		// The probe only pays off with leverage — a few cheap probes clearing many boards.
		// A lone board has none: probing it crawls the whole provider (for a boardless
		// aggregator, its entire dataset) only for the main loop to crawl it again. Skip it;
		// the normal cooldown gate recovers a single board in one crawl at cooldown expiry.
		if len(byProvider[provider]) < 2 {
			continue
		}
		boards, err := r.BoardHealth.CooledBoards(ctx, provider, maxRecoveryProbes)
		if err != nil {
			log.Printf("ingest: recovery probe %s: list cooled boards: %v", provider, err)
			continue
		}
		e, st, reused, answered := r.probeProvider(ctx, src, boards, byProvider[provider])
		if !answered {
			continue
		}
		cleared, err := r.BoardHealth.ClearCooldowns(ctx, provider)
		if err != nil {
			log.Printf("ingest: recovery probe %s: clear cooldowns: %v", provider, err)
			continue
		}
		log.Printf("ingest: %s answered a recovery probe — cleared %d cooled board(s) to crawl this cycle", provider, cleared)
		if reused {
			handled[boardKey{e.Provider, e.Board, e.Region}] = st
		}
	}
	return handled
}

// probeProvider fetches each candidate board through the adapter until one succeeds,
// reporting the entry that answered. For a non-streaming board — the common case, and
// the only one fetchBoard's result actually corresponds to — that same fetch is finished
// into a full ingest (reused=true, st holds its Stats) instead of being thrown away, so
// the board is not crawled a second time by the main loop; the finding this fixes was a
// HydratingSource's FetchNew (a full paginated re-crawl plus per-posting hydration, not a
// cheap reachability check) run once to answer the probe and once more moments later. A
// streaming adapter's real ingest path is ingestStream, which this fetch does not
// exercise, so reused is false for it and the main loop still runs it normally. A board
// absent from this run's entries is skipped (it cannot be probed without its entry), and
// a cancelled run stops early. answered=false when nothing responded.
func (r Runner) probeProvider(ctx context.Context, src sources.Source, boards []CooledBoard, entries map[entryKey]sources.CompanyEntry) (e sources.CompanyEntry, st Stats, reused, answered bool) {
	_, streaming := src.(sources.StreamingSource)
	for _, board := range boards {
		if ctx.Err() != nil {
			return sources.CompanyEntry{}, Stats{}, false, false
		}
		candidate, ok := entries[entryKey{board.Board, board.Region}]
		if !ok {
			continue
		}
		fetched, err := r.fetchBoard(ctx, candidate, src)
		if err != nil {
			continue
		}
		if streaming {
			return candidate, Stats{}, false, true
		}
		return candidate, r.ingestFetched(ctx, candidate, fetched), true, true
	}
	return sources.CompanyEntry{}, Stats{}, false, false
}

// entryKey is a board's identity within one provider: board alone collides when a provider's
// board id repeats across independent regional slices (e.g. Adzuna's "it-jobs" once per
// country), so region joins it. A region-invariant provider's entries all carry Region == "".
type entryKey struct{ board, region string }

// entriesByProvider indexes the run's entries as provider → entryKey → entry, so the
// recovery probe can find the CompanyEntry for a cooled (board, region) pair.
func entriesByProvider(entries []sources.CompanyEntry) map[string]map[entryKey]sources.CompanyEntry {
	byProvider := make(map[string]map[entryKey]sources.CompanyEntry)
	for _, e := range entries {
		boards := byProvider[e.Provider]
		if boards == nil {
			boards = make(map[entryKey]sources.CompanyEntry)
			byProvider[e.Provider] = boards
		}
		boards[entryKey{e.Board, e.Region}] = e
	}
	return byProvider
}

// sortedProviders returns the run's providers deterministically, so recovery probes (and
// tests) are order-stable.
func sortedProviders(byProvider map[string]map[entryKey]sources.CompanyEntry) []string {
	providers := make([]string, 0, len(byProvider))
	for p := range byProvider {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	return providers
}
