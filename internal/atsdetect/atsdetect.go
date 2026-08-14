// Package atsdetect detects a supported ATS board (provider + slug) from a page's
// HTML. It is the discovery core of the domain-following harvest: a company's
// careers-page HTML is scanned for the known ATS URL shapes, and the resolved
// board feeds the existing harvest-boards validator. It performs no I/O.
package atsdetect

import (
	"regexp"
	"strings"
)

// matchers are tried in order, so the first provider listed wins when a page links
// several ATSes. Each regex captures the board slug. Greenhouse has two shapes —
// the embed script (slug in the `for=` query param) and the direct board URL — and
// the embed is listed first so a `/embed/...` URL never falls through to the direct
// matcher (which would capture the path word "embed").
//
// Every pattern opens with a `(?:^|[^A-Za-z0-9])` boundary: without it, a host is
// matched wherever it appears as a substring, so a look-alike label like
// "evil-boards.greenhouse.io" or "notjobs.lever.co" would satisfy the same regex as
// the real vendor host. The boundary is non-capturing, so it does not shift the slug
// capture group's index.
var matchers = []struct {
	provider string
	re       *regexp.Regexp
}{
	{"greenhouse", regexp.MustCompile(`(?:^|[^A-Za-z0-9])(?:boards|job-boards)\.greenhouse\.io/embed/job_board(?:/js)?\?for=([a-z0-9][a-z0-9-]*)`)},
	{"greenhouse", regexp.MustCompile(`(?:^|[^A-Za-z0-9])(?:boards|job-boards)\.greenhouse\.io/([a-z0-9][a-z0-9-]*)`)},
	{"lever", regexp.MustCompile(`(?:^|[^A-Za-z0-9])jobs\.lever\.co/([a-z0-9][a-z0-9-]*)`)},
	{"ashby", regexp.MustCompile(`(?:^|[^A-Za-z0-9])jobs\.ashbyhq\.com/([a-z0-9][a-z0-9-]*)`)},
}

// reserved are path words a direct-URL matcher can capture that are not real board
// slugs (e.g. `boards.greenhouse.io/embed/...` with no `for=` param).
var reserved = map[string]bool{"embed": true}

// selfHosted fingerprints the ATS platforms whose tenants serve from the EMPLOYER's own domain,
// where the board IS that host (jobs.intuit.com, careers.arrive.com) exactly as the ingest
// adapters and their board files store it. Nothing in such a URL names the platform, and the
// page need not link to any ATS host either — so neither atsboard.Recognize nor a scan of the
// page's links can resolve one. The vendor's own bundle in the markup is the only tell.
//
// Each marker is the vendor's product name as it appears in asset paths and inline config, and
// they do not overlap: sampled across the boards already in radancy.yml / phenom.yml / jibe.yml
// / teamtailor.yml, every page carried its own marker and none carried another's. Greenhouse,
// Lever, Ashby, Workable and plain non-ATS careers pages carry none.
var selfHosted = []struct {
	provider string
	marker   *regexp.Regexp
}{
	{"radancy", regexp.MustCompile(`(?i)talentbrew`)},
	{"phenom", regexp.MustCompile(`(?i)phenompeople`)},
	{"jibe", regexp.MustCompile(`(?i)jibeapply`)},
	{"teamtailor", regexp.MustCompile(`(?i)teamtailor`)},
}

// vendorDomains are the ATS vendors' own domains. Their marketing sites naturally carry their
// own marker, and a tenant hosted on the vendor's domain is URL-derivable without fetching
// anything — either way the fetched host is not an employer board.
var vendorDomains = []string{
	"radancy.com", "talentbrew.com",
	"phenom.com", "phenompeople.com",
	"jibe.com", "jibeapply.com",
	"teamtailor.com",
}

// DetectSelfHosted reports the board for a career site served from the employer's own domain:
// the provider comes from the platform's fingerprint in html, the board is host itself. It is
// the last resort, after both the URL rules and the page's links have come up empty, and it
// performs no I/O.
func DetectSelfHosted(html, host string) (provider, board string, ok bool) {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	if host == "" {
		return "", "", false
	}
	for _, d := range vendorDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return "", "", false
		}
	}
	for _, s := range selfHosted {
		if s.marker.MatchString(html) {
			return s.provider, host, true
		}
	}
	return "", "", false
}

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
