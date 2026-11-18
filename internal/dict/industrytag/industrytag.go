// Package industrytag resolves free-text industry labels to a curated canonical
// vocabulary for companies.industries.
//
// It is the finer level beneath vocab.DomainValues, whose coarse "other" bucket
// swallows many companies; this dictionary names what they actually do.
//
// Dict-only, like internal/dict/skilltag and internal/dict/location: an unknown
// label produces nothing. Reshaping it into canonical form would add a third
// spelling to a column that already mixes two.
package industrytag

import (
	"regexp"
	"slices"
	"strings"
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// normalize folds a raw label to the lookup key the alias map is written in:
// lowercase, "&" spelled out, every other run of punctuation or whitespace
// collapsed to one hyphen.
//
// "&" becomes " and " with spaces: scraped labels write it unspaced ("Oil&Gas"),
// and a bare "and" would yield "oilandgas", a key no canonical matches.
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
		// Only canonicals leave this function: an already-canonical input passes (so
		// re-normalizing output is a no-op), and an alias with a non-canonical target
		// is refused so a dictionary typo cannot write an unfilterable value.
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
