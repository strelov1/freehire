// Package atsdetect detects a supported ATS board (provider + slug) from a page's
// HTML. It is the discovery core of the domain-following harvest: a company's
// careers-page HTML is scanned for the known ATS URL shapes, and the resolved
// board feeds the existing harvest-boards validator. It performs no I/O.
package atsdetect

import "regexp"

// matchers are tried in order, so the first provider listed wins when a page links
// several ATSes. Each regex captures the board slug. Greenhouse has two shapes —
// the embed script (slug in the `for=` query param) and the direct board URL — and
// the embed is listed first so a `/embed/...` URL never falls through to the direct
// matcher (which would capture the path word "embed").
var matchers = []struct {
	provider string
	re       *regexp.Regexp
}{
	{"greenhouse", regexp.MustCompile(`(?:boards|job-boards)\.greenhouse\.io/embed/job_board(?:/js)?\?for=([a-z0-9][a-z0-9-]*)`)},
	{"greenhouse", regexp.MustCompile(`(?:boards|job-boards)\.greenhouse\.io/([a-z0-9][a-z0-9-]*)`)},
	{"lever", regexp.MustCompile(`jobs\.lever\.co/([a-z0-9][a-z0-9-]*)`)},
	{"ashby", regexp.MustCompile(`jobs\.ashbyhq\.com/([a-z0-9][a-z0-9-]*)`)},
}

// reserved are path words a direct-URL matcher can capture that are not real board
// slugs (e.g. `boards.greenhouse.io/embed/...` with no `for=` param).
var reserved = map[string]bool{"embed": true}

// absURLRe extracts absolute http(s) URLs from arbitrary markup (href/src attributes
// or bare URLs in inline scripts), stopping at the first quote, angle bracket, or
// whitespace. It is the second-tier feed into FromURL.
var absURLRe = regexp.MustCompile(`https?://[^\s"'<>)\\]+`)

// Detect returns the first supported ATS board found in html. It first tries the
// ordered slug matchers (whose order encodes provider precedence and covers the
// greenhouse embed shape FromURL can't parse), then falls back to scanning every
// absolute URL through FromURL — so any board shape FromURL understands is detected
// on a careers page without duplicating its host parsing here. ok is false when
// nothing resolves.
func Detect(html string) (provider, slug string, ok bool) {
	for _, m := range matchers {
		for _, sub := range m.re.FindAllStringSubmatch(html, -1) {
			if s := sub[1]; !reserved[s] {
				return m.provider, s, true
			}
		}
	}
	for _, u := range absURLRe.FindAllString(html, -1) {
		if p, b, ok := FromURL(u); ok {
			return p, b, true
		}
	}
	return "", "", false
}
