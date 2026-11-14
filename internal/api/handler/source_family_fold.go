package handler

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/db"
)

// A source family (vocab.SourceFamilies) is one brand split into per-market adapters only
// because each market is its own publisher account and dedup namespace. The public pages
// show the brand once, the way the search filter already does, so the /sources and /status
// reads fold each family's rows into one row under the family name before rendering.

// foldHealthFamilies merges each family's health rows into one, keeping the order in which
// rows first appear. Board counts add up; the freshness timestamps take the latest member.
func foldHealthFamilies(rows []db.ProviderHealthRollupRow) []db.ProviderHealthRollupRow {
	out := make([]db.ProviderHealthRollupRow, 0, len(rows))
	at := make(map[string]int, len(rows))
	for _, r := range rows {
		r.Provider = vocab.SourceFamilyOf(r.Provider)
		i, seen := at[r.Provider]
		if !seen {
			at[r.Provider] = len(out)
			out = append(out, r)
			continue
		}
		f := &out[i]
		f.TotalBoards += r.TotalBoards
		f.HealthyBoards += r.HealthyBoards
		f.CooledBoards += r.CooledBoards
		f.IngestedTotal += r.IngestedTotal
		f.LastRunAt = latest(f.LastRunAt, r.LastRunAt)
		f.LastSuccessAt = latest(f.LastSuccessAt, r.LastSuccessAt)
	}
	return out
}

// foldSnapshotFamilies merges each family's snapshot rows into one. The browsable count stays
// measured only when every member's is: a sum over the measured members alone would publish
// a figure smaller than the truth as if it were one. MeasuredAt is the oldest member's, so
// the row never claims to be fresher than its stalest part.
func foldSnapshotFamilies(rows []db.ListSourceStatsRow) []db.ListSourceStatsRow {
	out := make([]db.ListSourceStatsRow, 0, len(rows))
	at := make(map[string]int, len(rows))
	for _, r := range rows {
		r.Source = vocab.SourceFamilyOf(r.Source)
		i, seen := at[r.Source]
		if !seen {
			at[r.Source] = len(out)
			out = append(out, r)
			continue
		}
		f := &out[i]
		f.OpenJobs += r.OpenJobs
		f.AtsMatchedJobs += r.AtsMatchedJobs
		f.BrowsableJobs = pgtype.Int8{Int64: f.BrowsableJobs.Int64 + r.BrowsableJobs.Int64, Valid: f.BrowsableJobs.Valid && r.BrowsableJobs.Valid}
		if r.MeasuredAt.Valid && (!f.MeasuredAt.Valid || r.MeasuredAt.Time.Before(f.MeasuredAt.Time)) {
			f.MeasuredAt = r.MeasuredAt
		}
	}
	return out
}

func latest(a, b pgtype.Timestamptz) pgtype.Timestamptz {
	if !a.Valid || (b.Valid && b.Time.After(a.Time)) {
		return b
	}
	return a
}
