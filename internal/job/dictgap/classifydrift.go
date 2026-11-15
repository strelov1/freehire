package dictgap

import (
	"sort"

	"github.com/strelov1/freehire/internal/dict/classify"
)

// TitleClassification is one distinct job title's enrichment facts, aggregated across
// its jobs. An empty enrichment field means the model stated no opinion for that facet.
type TitleClassification struct {
	Title               string
	Count               int
	EnrichmentSeniority string
	EnrichmentCategory  string
}

// DriftCandidate is one title where the dictionary's answer for a facet (seniority or
// category) disagrees with what LLM enrichment recorded.
type DriftCandidate struct {
	Title           string
	Count           int
	DictionaryValue string
	EnrichmentValue string
}

// DriftReport ranks the titles where classify.Parse disagrees with LLM enrichment, per facet.
type DriftReport struct {
	Seniority []DriftCandidate
	Category  []DriftCandidate
}

// ClassifyDriftCandidates recomputes classify.Parse for every title, so it reflects the
// dictionary as it stands today rather than the version that derived the stored columns.
// A facet is skipped when enrichment recorded no opinion or agrees. Lists are sorted by
// count descending, then title.
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
