package skilltag

import "maps"

// PreferredFromText reports, for each canonical skill the vacancy names, which of that
// skill's interchangeable spellings the vacancy used — "IaC" or "infrastructure as code",
// "k8s" or "Kubernetes". It is the invert path for JD-driven wording alignment; Parse's
// return shape is unchanged.
//
// Two properties the callers depend on:
//
//   - Only canonicals in interchangeableSurfaces can appear. The alias tables Parse uses
//     are many-to-one on purpose, so inverting THEM would answer "ruby" with "Ruby on
//     Rails" — a technology the candidate never claimed. Wording alignment is allowed to
//     change how a skill is spelled and nothing else.
//   - The spelling returned is the curated one, never a span cut out of the description.
//     Descriptions are raw ATS HTML, and stripMarkup leaves a space where each tag was, so
//     a slice of the source would carry double spaces and line breaks into a skill chip —
//     and a SHOUTY requirements heading would shout on the candidate's CV.
//
// Ambiguity is gated the way Parse gates it: a spelling that is ordinary English ("go"),
// or too short to be unmistakable ("ML"), counts only once the vacancy has named some
// other technology outright.
func PreferredFromText(text string) map[string]string {
	norm := normalize(text)
	if norm == "" {
		return nil
	}

	// Each canonical is resolved independently and its variants are already ordered
	// longest-first, so the answer does not depend on map iteration order: a vacancy
	// naming both "Kubernetes" and "k8s" always yields "Kubernetes".
	named, hedged := map[string]string{}, map[string]string{}
	for canonical, variants := range surfaceIndex {
		for _, v := range variants {
			if !v.matcher.matches(norm) {
				continue
			}
			if v.weak {
				hedged[canonical] = v.display
			} else {
				named[canonical] = v.display
			}
			break
		}
	}
	// Nothing named outright means nothing corroborates the hedged spellings, and a text
	// that only ever said "go" is not a vacancy talking about Go.
	if len(named) == 0 {
		return nil
	}
	maps.Copy(named, hedged)
	return named
}
