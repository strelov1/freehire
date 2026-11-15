package normalize

import (
	"regexp"
	"strings"
)

// Some ATS boards jam raw coordinates into the free-text location field
// (e.g. "Gaston, SC 29053 | 33.8316 | -81.1126"), which then surface in the UI
// and in the JobPosting addressLocality. This matches a trailing run of one or
// more pipe-delimited numeric segments — the coordinate tail — while leaving a
// legitimate "City | Country" split (no numeric tail) intact.
var coordTail = regexp.MustCompile(`\s*(\|\s*-?\d+(\.\d+)?\s*)+$`)

// Some postings carry the form label inside the value ("Locations: San Francisco",
// "Location Preferences: Minneapolis, MN"). The geography parser reads the label as
// part of the first place, so a single-place location resolved to nothing.
var labelHead = regexp.MustCompile(`(?i)^\s*locations?(\s+preferences?)?\s*:\s*`)

// CleanLocation strips a source's coordinate tail and a leading "Location:" label
// from a free-text location and trims surrounding whitespace. Plain places and
// "City | Country" splits pass through unchanged.
func CleanLocation(s string) string {
	s = labelHead.ReplaceAllString(s, "")
	return strings.TrimSpace(coordTail.ReplaceAllString(s, ""))
}
