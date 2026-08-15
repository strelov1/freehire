// Package industrytag resolves free-text industry labels to a curated canonical
// vocabulary for companies.industries.
//
// It is the finer level beneath vocab.DomainValues: domains names ~20 coarse
// verticals derived from job enrichment, and its "other" bucket swallows 42% of
// the companies it tags. This dictionary names what those companies actually do.
//
// Dict-only, like internal/skilltag and internal/location: a label outside the
// dictionary produces nothing. Guessing — including mechanically reshaping an
// unknown label into canonical form — would add a third spelling to a column that
// already mixes two.
package industrytag

import (
	"regexp"
	"slices"
	"strings"
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// normalize folds a raw label to the lookup key the alias map is written in:
// lowercase, "&" spelled out, and every other run of punctuation or whitespace
// collapsed to a single hyphen. So "Food & Beverage", "Food-and-Beverage" and
// "food and beverage" all share one key, and the dictionary needs one entry
// rather than one per spelling.
//
// "&" becomes " and " with spaces, not "and": scraped labels write it unspaced
// ("Food&Beverage", "Oil&Gas"), and substituting without separators would yield
// "foodandbeverage" — a key no canonical can ever match. The added spaces cost
// nothing, since the hyphen collapse below removes them.
func normalize(label string) string {
	s := strings.ToLower(strings.TrimSpace(label))
	s = strings.ReplaceAll(s, "&", " and ")
	return strings.Trim(nonAlnum.ReplaceAllString(s, "-"), "-")
}

// Canonicalize maps raw industry labels to canonical slugs, dropping every label
// the dictionary does not know. The result is sorted and de-duplicated so a caller
// can write it straight into a text[] column and compare two runs for equality.
//
// It never returns nil: an empty result is an empty slice, so a caller storing the
// result into a NOT NULL array column does not have to special-case it.
func Canonicalize(labels []string) []string {
	return resolve(labels, aliases, displayNames)
}

// resolve is Canonicalize with its two tables injected, so a test can drive a
// deliberately broken dictionary through the real code path without mutating the
// package's own maps.
func resolve(labels []string, aliases, canonical map[string]string) []string {
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		key := normalize(label)
		if key == "" {
			continue
		}
		if canonical, ok := aliases[key]; ok {
			key = canonical
		}
		// displayNames is the canonical set, and this lookup is the only way a value
		// leaves this function. It admits an already-canonical input — which is what
		// makes running the normalization worker over its own output a no-op — and it
		// refuses an alias whose target is not a canonical, so one typo in the
		// dictionary cannot write a value the facet could never offer.
		if _, ok := canonical[key]; ok {
			seen[key] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for canonical := range seen {
		out = append(out, canonical)
	}
	slices.Sort(out)
	return out
}
