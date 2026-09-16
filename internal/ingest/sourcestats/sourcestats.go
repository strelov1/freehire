// Package sourcestats folds one day's per-source measurement into the rows the
// source_stats snapshot holds: the grouped scan over open postings, the adapter
// registry that decides which sources exist at all, and Meilisearch's de-duplicated
// per-source distribution.
//
// It is the shape internal/search/facetsnapshot has — db rows in, db params out, no
// transport and no clock of its own — so the arithmetic that decides what a public page
// says about a source is a pure function a test can pin down.
package sourcestats

import (
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// BrowsableCounts is Meilisearch's de-duplicated open-posting count per source — what
// /jobs?source=<key> would actually show — together with whether it was measured at all.
//
// The two states are carried in a type rather than in a nilable map because they are not
// the same answer and the difference is invisible at a call site: a run that could not
// reach Meilisearch must leave the figure ABSENT, while a run that reached it and found a
// source missing from the distribution has measured a real zero. Written as a bare map,
// those two are one typo apart, and the mistake surfaces as a public page stating that a
// live source carries no jobs.
type BrowsableCounts struct {
	counts   map[string]int64
	measured bool
}

// Measured wraps a distribution Meilisearch actually answered with. A source absent from
// it has a measured count of zero.
func Measured(counts map[string]int64) BrowsableCounts {
	return BrowsableCounts{counts: counts, measured: true}
}

// Unmeasured is the distribution of a run that could not reach Meilisearch. Every
// source's de-duplicated count comes out absent, never zero.
func Unmeasured() BrowsableCounts { return BrowsableCounts{} }

// lookup answers the de-duplicated count for one source, and whether there is an answer.
func (b BrowsableCounts) lookup(source string) (int64, bool) {
	if !b.measured {
		return 0, false
	}
	return b.counts[source], true
}

// Union returns the sorted, distinct union of the given source-name sets — the single
// definition of "which sources exist" that both the rollup and the public endpoint use, so
// the two can never disagree about who belongs on the page.
//
// No one set is enough, and each covers a hole the others leave:
//
//   - The adapter REGISTRY alone drops a source that has postings but no crawl adapter.
//     `telegram` is exactly that — an extraction pipeline, absent from sources.Taxonomy(),
//     carrying real postings, with its own display label and its own figure on the
//     transparency page. A page claiming to list every source cannot be built on a list
//     that omits one, and the omission is invisible precisely because it is an omission.
//   - The SCAN alone drops every registered adapter whose postings have all closed,
//     leaving a reader unable to tell "we read this source and it currently carries
//     nothing" from "we have never measured this source". The page says different things
//     about the two.
//   - The PREVIOUS SNAPSHOT alone is nobody's whole answer, but without it the first two
//     still drop a source that is in neither: a non-registry source whose last posting has
//     just closed vanishes the moment it goes empty — which is the same silent drop, one
//     closure later.
func Union(sets ...[]string) []string {
	union := map[string]struct{}{}
	for _, set := range sets {
		for _, name := range set {
			union[name] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(union))
}

// Rows builds the snapshot rows for one run: one row per source, in source order.
//
// `previous` is the snapshot this run replaces. It is read for its KEYS only — every
// figure is re-measured — so a source that has gone quiet keeps reporting a measured zero
// instead of disappearing. See Union for why all three sets are needed.
func Rows(registry []string, agg []db.AggregateOpenJobsBySourceRow, previous []db.SourceStat, browsable BrowsableCounts, measuredAt time.Time) []db.InsertSourceStatParams {
	scanned := make(map[string]db.AggregateOpenJobsBySourceRow, len(agg))
	for _, r := range agg {
		scanned[r.Source] = r
	}

	names := Union(registry, slices.Collect(maps.Keys(scanned)), snapshotNames(previous))

	rows := make([]db.InsertSourceStatParams, 0, len(names))
	for _, source := range names {
		got := scanned[source]
		row := db.InsertSourceStatParams{
			Source:         source,
			OpenJobs:       got.OpenJobs,
			AtsMatchedJobs: got.AtsMatchedJobs,
			MeasuredAt:     pgtype.Timestamptz{Time: measuredAt, Valid: true},
		}
		if count, ok := browsable.lookup(source); ok {
			row.BrowsableJobs = pgtype.Int8{Int64: count, Valid: true}
		}
		rows = append(rows, row)
	}
	return rows
}

// snapshotNames is the source keys a stored snapshot holds.
func snapshotNames(rows []db.SourceStat) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Source
	}
	return out
}
