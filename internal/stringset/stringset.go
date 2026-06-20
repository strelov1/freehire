// Package stringset holds small helpers for working with string sets used by the
// deterministic dictionaries (location, skilltag) that collect canonical facet
// values into a set before serializing.
package stringset

import "sort"

// Sorted returns the set's keys ascending, or nil when empty so an absent facet
// omits cleanly (and matches a text[] column's '{}' default).
func Sorted(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
