package dictgap

import (
	"sort"

	"github.com/strelov1/freehire/internal/dict/classify"
)

// TitleClassification is one distinct job title's enrichment facts, aggregated
// across every job that carries it: how many jobs shared the title, and what LLM
// enrichment recorded for seniority/category on those jobs. Empty enrichment
// fields mean the model stated no opinion for that facet.
type TitleClassification struct {
	Title               string
	Count               int
	EnrichmentSeniority string
	EnrichmentCategory  string
}

// DriftCandidate is one title where the title-classification dictionary's answer
// for one facet (seniority or category) disagrees with what LLM enrichment
// recorded for jobs carrying that title.
type DriftCandidate struct {
	Title           string
	Count           int
	DictionaryValue string
	EnrichmentValue string
}

// DriftReport ranks the titles where internal/dict/classify.Parse disagrees with
// LLM enrichment, tracked separately per facet.
type DriftReport struct {
	Seniority []DriftCandidate
	Category  []DriftCandidate
}

// ClassifyDriftCandidates recomputes internal/dict/classify.Parse for every given
// title — reflecting the dictionary as it stands today, not whatever version
// derived the stored jobs.seniority/jobs.category columns — and reports every
// title where that recomputed value disagrees with what LLM enrichment recorded,
// for seniority and for category independently. A title is skipped for a facet
// when enrichment recorded no opinion (empty) or when the dictionary agrees with
// it. Each list is sorted by count descending, then by title for a deterministic
// order among ties.
func ClassifyDriftCandidates(rows []TitleClassification) DriftReport {
	var report DriftReport
	for _, row := range rows {
		dict := classify.Parse(row.Title)
		if row.EnrichmentSeniority != "" && row.EnrichmentSeniority != dict.Seniority {
			report.Seniority = append(report.Seniority, DriftCandidate{
				Title:           row.Title,
				Count:           row.Count,
				DictionaryValue: dict.Seniority,
				EnrichmentValue: row.EnrichmentSeniority,
			})
		}
		if row.EnrichmentCategory != "" && row.EnrichmentCategory != dict.Category {
			report.Category = append(report.Category, DriftCandidate{
				Title:           row.Title,
				Count:           row.Count,
				DictionaryValue: dict.Category,
				EnrichmentValue: row.EnrichmentCategory,
			})
		}
	}
	sortDriftCandidates(report.Seniority)
	sortDriftCandidates(report.Category)
	return report
}

func sortDriftCandidates(candidates []DriftCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Count != candidates[j].Count {
			return candidates[i].Count > candidates[j].Count
		}
		return candidates[i].Title < candidates[j].Title
	})
}
