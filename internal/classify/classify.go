// Package classify derives a job's seniority and role category deterministically
// from its title. It is a curated dictionary, not a model: it resolves known
// English and Russian title terms and emits nothing for what it cannot resolve
// (it never guesses). Canonical values are drawn from the same controlled
// vocabularies the enrichment contract defines (enrich.SeniorityValues /
// enrich.CategoryValues), so the parser, the enrichment payload, and the search
// facet all speak one set of values — the same doctrine as internal/location.
package classify

import (
	"strings"

	"github.com/strelov1/freehire/internal/wordmatch"
)

// Classification is the seniority and role category parsed from a job title.
// Each field is "" when the title states nothing the dictionary resolves.
type Classification struct {
	Seniority string // "" or one of enrich.SeniorityValues
	Category  string // "" or one of enrich.CategoryValues
}

// Parse resolves a job title to its seniority and category. It never guesses;
// an unresolved field is "".
func Parse(title string) Classification {
	lower := strings.ToLower(title)
	return Classification{
		Seniority: matchOrdered(lower, seniorityTable),
		Category:  matchOrdered(lower, categoryTable),
	}
}

// Categories resolves every category the text mentions — each alias that occurs as a
// whole word — distinct and in precedence order (the primary category first). Unlike
// Parse, which keeps only the single strongest category, this surfaces the full set a
// résumé spans (a backend engineer who also does ML). Empty when nothing resolves; it
// never guesses.
func Categories(text string) []string {
	return matchAllOrdered(strings.ToLower(text), categoryTable)
}

// matchAllOrdered returns the distinct canonical values of every alias (in priority
// order) that occurs as a whole word in title. Several aliases can share a canonical
// ("backend"/"back-end"), so results are deduplicated while preserving first-seen order.
func matchAllOrdered(title string, table []aliasEntry) []string {
	var out []string
	seen := map[string]bool{}
	for _, e := range table {
		if wordmatch.Contains(title, e.alias, wordmatch.UnicodeBoundary) {
			if !seen[e.canonical] {
				seen[e.canonical] = true
				out = append(out, e.canonical)
			}
		}
	}
	return out
}

// CategoryAliases maps each category canonical to the title aliases that resolve
// to it (the inverse of the internal alias table); SeniorityAliases does the same
// for grades. Exposed so the web role picker can search roles by shorthand — the
// same curated EN+RU terms used to tag titles, so "sre"/"sysadmin"/"sr" find their
// role rather than only its display label.
func CategoryAliases() map[string][]string  { return invertAliases(categoryTable) }
func SeniorityAliases() map[string][]string { return invertAliases(seniorityTable) }

func invertAliases(table []aliasEntry) map[string][]string {
	out := make(map[string][]string, len(table))
	for _, e := range table {
		out[e.canonical] = append(out[e.canonical], e.alias)
	}
	return out
}

// matchOrdered returns the canonical value of the first alias (in priority order)
// that occurs as a whole word in title, or "" if none match. Ordering encodes
// precedence: the most specific / highest-rank alias is checked first, so a title
// carrying several grade words ("Lead Senior") resolves the stronger one.
func matchOrdered(title string, table []aliasEntry) string {
	for _, e := range table {
		if wordmatch.Contains(title, e.alias, wordmatch.UnicodeBoundary) {
			return e.canonical
		}
	}
	return ""
}
