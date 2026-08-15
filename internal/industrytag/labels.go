package industrytag

import "slices"

// displayNames is the canonical set and the text each value renders as. A slug
// absent from this map is not a canonical value — Canonicalize consults it to
// decide whether an unrecognized-but-slug-shaped input is a real canonical, and an
// invariant test asserts every alias target appears here.
var displayNames = map[string]string{
	"ai":                 "AI",
	"financial-services": "Financial Services",
	"food-and-beverage":  "Food & Beverage",
	"medical-devices":    "Medical Devices",
	"retail":             "Retail",
}

// Label returns the display text for a canonical slug. An unknown slug returns
// itself rather than empty, so a value stored before a dictionary edit still
// renders as something readable instead of vanishing from the UI.
func Label(canonical string) string {
	if name, ok := displayNames[canonical]; ok {
		return name
	}
	return canonical
}

// Canonicals returns every canonical slug, sorted — the source for the facet's
// option list, so the UI cannot drift from the dictionary.
func Canonicals() []string {
	out := make([]string, 0, len(displayNames))
	for canonical := range displayNames {
		out = append(out, canonical)
	}
	slices.Sort(out)
	return out
}
