// Package suggest builds and serves the search box's suggestion dictionary: the
// posting titles the catalogue actually carries, mined offline and offered as
// completions the moment someone types.
//
// It exists because the facet dictionaries cannot answer what people type. Measured
// on the live catalogue: 8,680 postings are titled "Product Owner", and the role
// vocabulary has no such role — only an alias folding it into `product`, whose label
// is "Product Manager". So the box answered a question about real postings by
// renaming the person asking. Titles are the vocabulary the market actually uses.
package suggest

import "strings"

// separators end the part of a title that NAMES the job. Everything after the first
// one qualifies it — a location, a gender notice, a seniority aside, an employer —
// and keeping any of it produces one suggestion per employer's punctuation habits
// rather than one per job.
//
// The dash is NOT here: a hyphen inside a word is part of the name ("front-end",
// "e-commerce"), and cutting on it turns "Front-End Developer" into "front". Spaced
// dashes are handled separately below.
const separators = "|(),[]/"

// spacedDashes are the dash forms that separate rather than hyphenate. Each is
// surrounded by space in real titles ("Product Owner - Data"), which is exactly what
// distinguishes them from the hyphen in "front-end".
var spacedDashes = []string{" - ", " – ", " — "}

// atSeparators name the employer rather than the job ("Staff Engineer at Google").
// Spaced, so the "at" inside "Data Analyst" is untouched.
var atSeparators = []string{" at ", " @ "}

// Title reduces a posting title to the phrase that names the job, lowercased.
//
// It is the ONE normalisation: the builder applies it to mined titles and the query
// path applies it to what a visitor types, so a typed query and the title it names
// land on the same key. Two copies would drift, and the drift would look exactly like
// a suggestion nobody ever searches for.
func Title(s string) string {
	out := strings.ToLower(s)

	// Newlines and tabs are whitespace from whatever produced the feed, not structure.
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

	// A dash that survived the spaced forms above can still OPEN a title ("- Data
	// Engineer"), where it separates from nothing and leaves nothing behind.
	out = strings.TrimLeft(out, " -–—")

	// Collapse runs of whitespace. strings.Fields also drops the leading and trailing
	// runs, so no separate trim is needed.
	return strings.Join(strings.Fields(out), " ")
}
