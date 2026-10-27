package mcpapp

import (
	"cmp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// list_filter_values answers "which values does this filter take?" before a search is
// spent on a guess. Measured against production on 2026-10-27: search_jobs had no way to
// learn the skill vocabulary, so a model wrote "golangg", got zero postings, and told a
// person freehire had no Go jobs. Reporting the unknown value afterwards is half the fix;
// this is the other half — the account server's facets tool, narrowed to one filter at a
// time, because the full distribution (1,200 skills alone) is a context window spent on a
// list the model needs one line of.

// Bounds for a list_filter_values page.
const (
	defaultFilterValues = 50
	maxFilterValues     = 200
)

// filterValueFacets maps each search_jobs input field this tool can list to the facet the
// catalogue counts it under. The keys are the TOOL's field names (sources, not source),
// because that is what the model will write next. company_slugs is absent: an employer is
// found by name through the company search, and the slug distribution is millions long.
var filterValueFacets = map[string]string{
	"category":        "category",
	"seniority":       "seniority",
	"skills":          "skills",
	"work_mode":       "work_mode",
	"regions":         "regions",
	"countries":       "countries",
	"cities":          "cities",
	"sources":         "source",
	"employment_type": "employment_type",
	"english_level":   "english_level",
	"relocation":      "relocation",
}

// FilterValuesInput is list_filter_values' input.
type FilterValuesInput struct {
	Filter   string `json:"filter" jsonschema:"a job-search filter: one of category, seniority, skills, work_mode, regions, countries, cities, sources, employment_type, english_level, relocation"`
	Contains string `json:"contains,omitempty" jsonschema:"keep only values containing this text, e.g. kube; for skills a known alias also finds its value, e.g. golang finds go"`
	Limit    int    `json:"limit,omitempty" jsonschema:"how many values to return, up to 200"`
}

// FilterValuesResult is what list_filter_values answers.
type FilterValuesResult struct {
	Filter        string       `json:"filter"`
	Values        []FacetCount `json:"values" jsonschema:"values carried by open postings, most common first, with how many carry each"`
	More          int          `json:"more,omitempty" jsonschema:"how many further values matched beyond this page"`
	IgnoredParams []string     `json:"ignored_params,omitempty" jsonschema:"inputs that were not understood"`
}

// FilterValues projects the catalogue's facet distributions (keyed by facet param) into
// one filter's values: those matching contains, most common first, bounded by limit.
func FilterValues(in FilterValuesInput, counts map[string]map[string]int64) FilterValuesResult {
	out := FilterValuesResult{Filter: in.Filter, Values: []FacetCount{}}
	facet, ok := filterValueFacets[strings.ToLower(strings.TrimSpace(in.Filter))]
	if !ok {
		out.IgnoredParams = []string{"filter=" + in.Filter}
		return out
	}

	match := containsMatcher(facet, in.Contains)
	for value, count := range counts[facet] {
		if match(value) {
			out.Values = append(out.Values, FacetCount{Value: value, Count: int(count)})
		}
	}
	slices.SortFunc(out.Values, func(a, b FacetCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Value, b.Value))
	})

	limit := defaultFilterValues
	if in.Limit > 0 {
		limit = min(in.Limit, maxFilterValues)
	}
	if len(out.Values) > limit {
		out.More = len(out.Values) - limit
		out.Values = out.Values[:limit]
	}
	return out
}

// containsMatcher keeps a value when it contains the text, case-folded. For skills it also
// keeps the canonical the text resolves to: the search reads "golang" as "go", and a
// model that asked about golang must be shown the value it will find postings under — a
// substring match alone would offer "golang-migrate" and hide "go".
func containsMatcher(facet, contains string) func(string) bool {
	needle := strings.ToLower(strings.TrimSpace(contains))
	if needle == "" {
		return func(string) bool { return true }
	}
	canonical := ""
	if facet == "skills" {
		canonical, _ = skilltag.Resolve(needle)
	}
	return func(value string) bool {
		return value == canonical || strings.Contains(strings.ToLower(value), needle)
	}
}
