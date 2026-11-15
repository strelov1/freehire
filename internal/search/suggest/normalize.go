// Package suggest builds and serves the search box's suggestion dictionary: the
// posting titles the catalogue actually carries, mined offline and offered as
// completions the moment someone types.
//
// It exists because the facet dictionaries cannot answer what people type: the role
// vocabulary has no "Product Owner", only an alias folding it into "Product Manager".
// Titles are the vocabulary the market actually uses.
package suggest

import "strings"

// separators end the part of a title that names the job; what follows qualifies it
// (location, gender notice, employer). The hyphen is not one ("front-end"); spaced
// dashes are handled separately.
const separators = "|(),[]/"

// spacedDashes are the dash forms that separate; the surrounding spaces distinguish
// them from the hyphen in "front-end".
var spacedDashes = []string{" - ", " – ", " — "}

// atSeparators name the employer ("Staff Engineer at Google"). Spaced, so "Data
// Analyst" is untouched.
var atSeparators = []string{" at ", " @ "}

// Title reduces a posting title to the lowercased phrase that names the job. The builder
// and the query path both use it, so a typed query and the title it names share a key.
func Title(s string) string {
	out := strings.ToLower(s)

	// Newlines and tabs are feed whitespace, not structure.
	out = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, out)

	for _, sep := range spacedDashes {
		if i := strings.Index(out, sep); i >= 0 {
			out = out[:i]
		}
	}
	for _, sep := range atSeparators {
		if i := strings.Index(out, sep); i >= 0 {
			out = out[:i]
		}
	}
	if i := strings.IndexAny(out, separators); i >= 0 {
		out = out[:i]
	}

	// A dash can still open a title ("- Data Engineer").
	out = strings.TrimLeft(out, " -–—")

	// Collapse whitespace runs; Fields also trims the ends.
	return strings.Join(strings.Fields(out), " ")
}
