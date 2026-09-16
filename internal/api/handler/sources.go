package handler

import (
	"maps"
	"slices"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/ingest/sourcestats"
	"github.com/strelov1/freehire/internal/platform/db"
)

// sourceEntry is the public, sanitized entry for one source on the /sources page.
//
// Like statusProvider beside it, it is sanitized BY CONSTRUCTION: there is no field for a
// board identifier, a crawl error, or a specific posting URL, so none of them can leak by
// somebody forgetting to omit one.
//
// The two optional blocks are optional for different reasons, and both absences are real
// answers rather than missing data:
//
//   - Jobs is absent when the snapshot has never been taken for this source. Its figures
//     must not degrade to zeros, which would state on a public page that a live source
//     carries nothing.
//   - Health is absent when the source has no board_health record at all — which is the
//     normal state for a source that is not a crawl adapter (an extraction pipeline, a
//     manual intake). Deriving a status from no evidence would publish a verdict about
//     something nothing measured.
type sourceEntry struct {
	Source string `json:"source"`
	Kind   string `json:"kind"`
	// No logo field. A source's brand mark is resolved client-side from its DISPLAY NAME,
	// which the SPA already owns. Publishing a host was tried and served the WRONG brand:
	// an ATS posting's URL is usually on the employer's own domain, so the sample taken for
	// greenhouse was bankrate.com and the one for successfactors was a staffing agency's.
	Jobs   *sourceJobs   `json:"jobs"`
	Health *sourceHealth `json:"health"`
}

// sourceJobs is what the catalogue currently holds under one source.
type sourceJobs struct {
	// Open is the raw count of open postings — every copy, duplicates included. It is the
	// denominator the overlap figures below are arithmetic on.
	Open int64 `json:"open"`
	// Browsable is the de-duplicated count Meilisearch holds: what /jobs?source=<key>
	// actually shows, and therefore the figure the page displays. Absent (never zero)
	// when Meilisearch could not be measured.
	Browsable *int64 `json:"browsable"`
	// ATSMatched and ATSUnmatched are carried for AGGREGATORS ONLY.
	//
	// "Unmatched" means the dedup pass found no first-party ATS posting to pair this one
	// with. That is the absence of evidence, not evidence of absence — the fuzzy and role
	// passes both miss — so the field is not called `exclusive`, here or on the page. A
	// name like that converts an unmeasured gap into a claim the first time it is read.
	//
	// They are omitted entirely for other kinds rather than sent as zeros: the
	// aggregator-suppression pass only ever marks an aggregator row, so a first-party ATS
	// source's 0 is arithmetic, and published it would read as "fully exclusive".
	ATSMatched   *int64 `json:"ats_matched,omitempty"`
	ATSUnmatched *int64 `json:"ats_unmatched,omitempty"`
	MeasuredAt   string `json:"measured_at"`
}

// sourceHealth is the crawl-fleet verdict for one source, derived exactly the way the
// status page derives it so the two pages can never disagree about whether a source is up.
type sourceHealth struct {
	Status        providerStatus `json:"status"`
	TotalBoards   int64          `json:"total_boards"`
	HealthyBoards int64          `json:"healthy_boards"`
	CooledBoards  int64          `json:"cooled_boards"`
	LastRun       *string        `json:"last_run"`
	LastSuccess   *string        `json:"last_success"`
	IngestedTotal int64          `json:"ingested_total"`
}

// Sources serves the public source catalogue the /sources page renders: every source the
// catalogue is built from, with what it currently holds and how its crawl is doing.
//
// Unauthenticated like the other public reads, and aggregate-only — per-source counts and
// one host, no record-level data.
func (h *statsHandlers) Sources(c *fiber.Ctx) error {
	health, err := h.queries.ProviderHealthRollup(c.Context())
	if err != nil {
		return err
	}
	snapshot, err := h.queries.ListSourceStats(c.Context())
	if err != nil {
		return err
	}

	// Taxonomy, not a crawl registry: in a crawl registry a keyed adapter is absent
	// wherever its credential is unset, and would be misclassified. The API host holds no
	// ingest credentials at all.
	entries := buildSourceEntries(sources.Taxonomy(), health, snapshot, time.Now())

	return c.JSON(fiber.Map{"data": entries})
}

// buildSourceEntries folds the adapter registry, the fleet health rollup and the per-source
// snapshot into the wire entries, in source order. Which sources exist is decided by
// sourcestats.Union, which argues the case once for both this endpoint and the rollup.
func buildSourceEntries(reg map[string]sources.Source, health []db.ProviderHealthRollupRow, snapshot []db.SourceStat, now time.Time) []sourceEntry {
	byHealth := make(map[string]db.ProviderHealthRollupRow, len(health))
	for _, r := range health {
		byHealth[r.Provider] = r
	}
	bySnapshot := make(map[string]db.SourceStat, len(snapshot))
	for _, r := range snapshot {
		bySnapshot[r.Source] = r
	}

	// The same Union the rollup builds its snapshot from, so the endpoint and the worker can
	// never disagree about which sources exist. It is re-derived here rather than trusted
	// from the snapshot alone because the snapshot may not have run yet, and a page that
	// lists nothing until a cron fires is worse than one that lists every adapter with
	// absent figures.
	names := sourcestats.Union(slices.Collect(maps.Keys(reg)), slices.Collect(maps.Keys(bySnapshot)))

	entries := make([]sourceEntry, 0, len(names))
	for _, name := range names {
		entry := sourceEntry{Source: name, Kind: sources.ProviderKind(reg, name)}
		if row, ok := bySnapshot[name]; ok {
			entry.Jobs = jobsBlock(row, entry.Kind)
		}
		if row, ok := byHealth[name]; ok {
			entry.Health = healthBlock(row, now)
		}
		entries = append(entries, entry)
	}
	return entries
}

// jobsBlock renders one snapshot row, attaching the overlap figures only where they mean
// anything. Unmatched is computed here rather than stored, so it cannot drift from the
// raw count it is a remainder of.
func jobsBlock(row db.SourceStat, kind string) *sourceJobs {
	jobs := &sourceJobs{Open: row.OpenJobs, MeasuredAt: row.MeasuredAt.Time.Format(time.RFC3339)}
	if row.BrowsableJobs.Valid {
		n := row.BrowsableJobs.Int64
		jobs.Browsable = &n
	}
	if kind == sources.KindAggregator {
		matched := row.AtsMatchedJobs
		unmatched := row.OpenJobs - matched
		jobs.ATSMatched = &matched
		jobs.ATSUnmatched = &unmatched
	}
	return jobs
}

// healthBlock renders one fleet-health rollup row, deriving the status through the same
// deriveStatus the /status page uses.
func healthBlock(row db.ProviderHealthRollupRow, now time.Time) *sourceHealth {
	return &sourceHealth{
		Status: deriveStatus(providerRollup{
			total:       row.TotalBoards,
			healthy:     row.HealthyBoards,
			lastSuccess: tsTime(row.LastSuccessAt),
		}, now),
		TotalBoards:   row.TotalBoards,
		HealthyBoards: row.HealthyBoards,
		CooledBoards:  row.CooledBoards,
		LastRun:       isoOrNil(row.LastRunAt),
		LastSuccess:   isoOrNil(row.LastSuccessAt),
		IngestedTotal: row.IngestedTotal,
	}
}
