package dictgap

import (
	"sort"

	"github.com/strelov1/freehire/internal/dict/classify"
)

// TitleCount is one distinct job title and how many postings carried it. The SQL
// groups per id chunk, so the same title arrives once per chunk it appears in — the
// same shape ListTitlesForClassifyDrift produces, for the same reason: an aggregate
// carries no single id to resume a partial chunk from.
type TitleCount struct {
	Title string
	Count int
}

// UnclassifiedTitles ranks the titles NEITHER dictionary places — no tech-title
// signal from classify.IsTech and no category from classify.Parse — by how many
// postings carry them.
//
// This population is invisible to the other two reports by construction rather than
// by omission. Enrichment is gated on is_tech IS TRUE, so a title no dictionary
// places is never enriched, has no model opinion recorded against it, and can
// therefore never appear in a drift report (which compares against that opinion) or
// a skill-gap report (which mines it). Measured on prod 2026-09-23, that blind spot
// held 2,232,773 open canonical postings, 71,314 of them on titles reading as
// software or IT outright.
//
// Like ClassifyDriftCandidates it RECOMPUTES, and here that is a requirement rather
// than an implementation note: a dictionary change reaches stored rows only through
// cmd/backfill-derive, measured at ~171 rows/s over 12.7M rows, so for most of a day
// after terms ship, jobs.is_tech still states what the OLD dictionary said. A report
// reading that column would rank gaps already closed and hide gaps the new terms
// opened — at exactly the moment a curator is most likely to run it.
//
// BOTH dictionaries must fail for a title to be a gap. A technical category already
// yields is_tech through jobderive's derivation, so a title classify.Parse places
// would send a curator after a term that changes nothing.
func UnclassifiedTitles(rows []TitleCount) []TitleCount {
	totals := make(map[string]int, len(rows))
	for _, r := range rows {
		// An empty title names no role, so no entry could ever place it.
		if r.Title == "" {
			continue
		}
		if classify.IsTech(r.Title) || classify.Parse(r.Title).Category != "" {
			continue
		}
		totals[r.Title] += r.Count
	}
	out := make([]TitleCount, 0, len(totals))
	for title, count := range totals {
		out = append(out, TitleCount{Title: title, Count: count})
	}
	// Count descending, then title, so two runs over the same data print the same
	// list — a report a curator diffs against last week's is worth more than one they
	// cannot.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Title < out[j].Title
	})
	return out
}
