package handler

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/ingest/sourcestats"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The wire entry for one source on the /sources page is apigen.SourceEntry.
//
// Like the provider entries beside it, it is sanitized BY CONSTRUCTION: there is no field
// for a board identifier, a crawl error, or a specific posting URL, so none of them can
// leak by somebody forgetting to omit one.
//
// The two optional blocks are optional for different reasons, and both absences are real
// answers rather than missing data:
//
//   - Jobs is null when the snapshot has never been taken for this source. Its figures
//     must not degrade to zeros, which would state on a public page that a live source
//     carries nothing.
//   - Health is null when the source has no board_health record at all — which is the
//     normal state for a source that is not a crawl adapter (an extraction pipeline, a
//     manual intake). Deriving a status from no evidence would publish a verdict about
//     something nothing measured.
//
// Adapter is whether the source is a crawl adapter at all. Kind no longer answers it: the
// Telegram pipeline is social and employer-published vacancies are company, yet neither
// has a board to crawl, so a missing health record means "nothing to crawl" for them and
// "not crawled yet" for an adapter.
//
// There is no logo field. A source's brand mark is resolved client-side from its DISPLAY
// NAME, which the SPA already owns. Publishing a host was tried and served the WRONG
// brand: an ATS posting's URL is usually on the employer's own domain, so the sample taken
// for greenhouse was bankrate.com and the one for successfactors was a staffing agency's.
//
// Jobs.Browsable is the de-duplicated count Meilisearch holds: what /jobs?source=<key>
// actually shows, and therefore the figure the page displays. Null (never zero) when
// Meilisearch could not be measured. Open is the raw count of open postings, duplicates
// included — the denominator the overlap figures are arithmetic on.
//
// ATSMatched and ATSUnmatched are carried for AGGREGATORS ONLY. "Unmatched" means the
// dedup pass found no first-party ATS posting to pair this one with. That is the absence
// of evidence, not evidence of absence — the fuzzy and role passes both miss — so the field
// is not called `exclusive`, here or on the page. They are omitted entirely for other kinds
// rather than sent as zeros: the aggregator-suppression pass only ever marks an aggregator
// row, so a first-party ATS source's 0 is arithmetic, and published it would read as
// "fully exclusive".

// Sources serves the public source catalogue the /sources page renders: every source the
// catalogue is built from, with what it currently holds and how its crawl is doing.
//
// Unauthenticated like the other public reads, and aggregate-only — per-source counts and
// one host, no record-level data.
func (h *statsHandlers) ListSources(ctx context.Context) (*apigen.ListSourcesOK, error) {
	health, err := h.queries.ProviderHealthRollup(ctx)
	if err != nil {
		return nil, err
	}
	snapshot, err := h.queries.ListSourceStats(ctx)
	if err != nil {
		return nil, err
	}

	// Taxonomy, not a crawl registry: in a crawl registry a keyed adapter is absent
	// wherever its credential is unset, and would be misclassified. The API host holds no
	// ingest credentials at all.
	entries := buildSourceEntries(sources.Taxonomy(), health, snapshot, time.Now())

	return &apigen.ListSourcesOK{Data: entries}, nil
}

// buildSourceEntries folds the adapter registry, the fleet health rollup and the per-source
// snapshot into the wire entries, in source order. Which sources exist is decided by
// sourcestats.Union, which argues the case once for both this endpoint and the rollup.
func buildSourceEntries(reg map[string]sources.Source, health []db.ProviderHealthRollupRow, snapshot []db.ListSourceStatsRow, now time.Time) []apigen.SourceEntry {
	byHealth := make(map[string]db.ProviderHealthRollupRow, len(health))
	for _, r := range health {
		byHealth[r.Provider] = r
	}
	bySnapshot := make(map[string]db.ListSourceStatsRow, len(snapshot))
	for _, r := range snapshot {
		bySnapshot[r.Source] = r
	}

	// The same Union the rollup builds its snapshot from, so the endpoint and the worker can
	// never disagree about which sources exist. It is re-derived here rather than trusted
	// from the snapshot alone because the snapshot may not have run yet, and a page that
	// lists nothing until a cron fires is worse than one that lists every adapter with
	// absent figures.
	names := sourcestats.Union(slices.Collect(maps.Keys(reg)), slices.Collect(maps.Keys(bySnapshot)))

	entries := make([]apigen.SourceEntry, 0, len(names))
	for _, name := range names {
		_, adapter := reg[name]
		entry := apigen.SourceEntry{Source: name, Kind: sources.ProviderKind(reg, name), Adapter: adapter,
			Jobs: apigen.NilSourceJobs{Null: true}, Health: apigen.NilSourceHealth{Null: true}}
		if row, ok := bySnapshot[name]; ok {
			entry.Jobs = apigen.NewNilSourceJobs(jobsBlock(row, sources.IsAggregator(reg, name)))
		}
		if row, ok := byHealth[name]; ok {
			entry.Health = apigen.NewNilSourceHealth(healthBlock(row, now))
		}
		entries = append(entries, entry)
	}
	return entries
}

// jobsBlock renders one snapshot row, attaching the overlap figures only where they mean
// anything. Unmatched is computed here rather than stored, so it cannot drift from the
// raw count it is a remainder of.
func jobsBlock(row db.ListSourceStatsRow, aggregator bool) apigen.SourceJobs {
	jobs := apigen.SourceJobs{Open: row.OpenJobs, MeasuredAt: row.MeasuredAt.Time.Format(time.RFC3339), Browsable: apigen.NilInt64{Null: true}}
	if row.BrowsableJobs.Valid {
		jobs.Browsable = apigen.NewNilInt64(row.BrowsableJobs.Int64)
	}
	if aggregator {
		matched := row.AtsMatchedJobs
		unmatched := row.OpenJobs - matched
		jobs.AtsMatched = apigen.NewOptInt64(matched)
		jobs.AtsUnmatched = apigen.NewOptInt64(unmatched)
	}
	return jobs
}

// healthBlock renders one fleet-health rollup row, deriving the status through the same
// deriveStatus the /status page uses.
func healthBlock(row db.ProviderHealthRollupRow, now time.Time) apigen.SourceHealth {
	return apigen.SourceHealth{
		Status: string(deriveStatus(providerRollup{
			total:       row.TotalBoards,
			healthy:     row.HealthyBoards,
			lastSuccess: tsTime(row.LastSuccessAt),
		}, now)),
		TotalBoards:   row.TotalBoards,
		HealthyBoards: row.HealthyBoards,
		CooledBoards:  row.CooledBoards,
		LastRun:       isoOrNil(row.LastRunAt),
		LastSuccess:   isoOrNil(row.LastSuccessAt),
		IngestedTotal: row.IngestedTotal,
	}
}
