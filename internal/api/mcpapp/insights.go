package mcpapp

import (
	"cmp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Bounds for the three market tools.
const (
	defaultSimilar = 6
	maxSimilar     = 20
	// marketTopValues is how many values of each facet job_market_stats keeps, so ten facets fit a turn.
	marketTopValues = 10
)

// marketFacets are the distributions job_market_stats publishes. Cities are left out: their
// long tail is thousands of entries; countries and regions answer "where".
var marketFacets = []string{
	"category", "seniority", "skills", "work_mode", "countries", "regions",
	"employment_type", "english_level", "visa_sponsorship", "relocation",
}

// MarketStats projects a facet result: the total and the top values of each published facet,
// highest count first, ties broken by value so the answer is the same on every read.
func (p Projector) MarketStats(total int64, counts map[string]map[string]int64) MarketStatsResult {
	out := MarketStatsResult{Total: int(total), Facets: make(map[string][]FacetCount, len(marketFacets))}
	for _, facet := range marketFacets {
		values := counts[facet]
		if len(values) == 0 {
			continue
		}
		top := make([]FacetCount, 0, len(values))
		for value, count := range values {
			top = append(top, FacetCount{Value: value, Count: int(count)})
		}
		slices.SortFunc(top, func(a, b FacetCount) int {
			return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Value, b.Value))
		})
		out.Facets[facet] = top[:min(len(top), marketTopValues)]
	}
	return out
}

// Scope is the salary scope this tool can honour: known vocabulary values, lowercased, and
// "" for anything it cannot — which Unsupported names back.
func (in SalaryInput) Scope() (category, seniority, country string) {
	if known(vocab.CategoryValues, in.Category) {
		category = strings.ToLower(strings.TrimSpace(in.Category))
	}
	if known(vocab.SeniorityValues, in.Seniority) {
		seniority = strings.ToLower(strings.TrimSpace(in.Seniority))
	}
	if isCountryCode(in.Country) {
		country = strings.ToLower(strings.TrimSpace(in.Country))
	}
	return category, seniority, country
}

// Unsupported names the scope values that were dropped, as search_jobs does: a salary for
// "backend in Germany" answered as "backend everywhere" must say so.
func (in SalaryInput) Unsupported() []string {
	var out []string
	if in.Category != "" && !known(vocab.CategoryValues, in.Category) {
		out = append(out, "category="+in.Category)
	}
	if in.Seniority != "" && !known(vocab.SeniorityValues, in.Seniority) {
		out = append(out, "seniority="+in.Seniority)
	}
	if in.Country != "" && !isCountryCode(in.Country) {
		out = append(out, "country="+in.Country)
	}
	slices.Sort(out)
	return out
}

// similarLimit bounds a similar_jobs page.
func similarLimit(limit int) int {
	if limit <= 0 {
		return defaultSimilar
	}
	return min(limit, maxSimilar)
}
