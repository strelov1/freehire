package dictgap

import (
	"sort"

	"github.com/strelov1/freehire/internal/dict/classify"
)

// TitleCount is one distinct job title and how many postings carried it. The SQL groups
// per id chunk, so the same title arrives once per chunk it appears in.
type TitleCount struct {
	Title string
	Count int
}

// UnclassifiedTitles ranks the titles neither dictionary places (no classify.IsTech and no
// classify.Parse category) by how many postings carry them.
//
// The other two reports cannot see these: enrichment is gated on is_tech, so such a title
// is never enriched and has no model opinion to drift from or skills to mine.
//
// It recomputes rather than reading jobs.is_tech, because a dictionary change reaches
// stored rows only through a slow cmd/backfill-derive, so the column lags the dictionary.
//
// Both dictionaries must fail for a title to be a gap: a technical category already yields
// is_tech, so a title classify.Parse places needs no new term.
func UnclassifiedTitles(rows []TitleCount) []TitleCount {
	totals := make(map[string]int, len(rows))
	for _, r := range rows {
		// An empty title names no role.
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
	// Count descending, then title, so runs over the same data print the same list.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Title < out[j].Title
	})
	return out
}
