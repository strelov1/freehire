package main

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/atsdetect"
)

// careerPaths are the common careers/jobs locations probed on a company site, in
// addition to the homepage and a careers link discovered on it.
var careerPaths = []string{"/careers", "/jobs", "/career"}

// anchorRe extracts (href, inner-text) from each <a> tag (case-insensitive, dotall
// so the text may span lines). Both quote styles are accepted — some sites write
// href='...'.
var anchorRe = regexp.MustCompile(`(?is)<a\b[^>]*\bhref=["']([^"']*)["'][^>]*>(.*?)</a>`)

// careersLink returns the first anchor on a homepage that points at a careers/jobs
// page — matched on either the href or the link text containing "career"/"job" —
// resolved to an absolute URL against base. It returns "" when none is found.
func careersLink(html, base string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	for _, m := range anchorRe.FindAllStringSubmatch(html, -1) {
		href, text := m[1], strings.ToLower(stripTags(m[2]))
		if looksCareers(strings.ToLower(href)) || looksCareers(text) {
			if ref, err := url.Parse(href); err == nil {
				return b.ResolveReference(ref).String()
			}
		}
	}
	return ""
}

func looksCareers(s string) bool {
	return strings.Contains(s, "career") || strings.Contains(s, "job")
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func stripTags(s string) string { return tagRe.ReplaceAllString(s, "") }

// fetchFunc fetches a URL's body, or returns an error. Injected so resolve is
// testable without network.
type fetchFunc func(string) (string, error)

// resolve follows a company website to its ATS board: it fetches the homepage and
// a small fixed set of careers/jobs pages (plus a careers link found on the
// homepage), runs atsdetect on each, and returns the first board found. A fetch
// error on any single page is ignored (best-effort); ok is false when no page
// yields a board.
func resolve(website string, fetch fetchFunc) (provider, slug string, ok bool) {
	base := strings.TrimRight(strings.TrimSpace(website), "/")
	if base == "" {
		return "", "", false
	}

	home, err := fetch(base)
	if err != nil {
		// Homepage unreachable (dead domain, DNS failure, block). The careers paths
		// live on the same host, so probing them would only burn more timeouts —
		// give up on this company. (Dead domains dominate the unmatched long tail.)
		return "", "", false
	}
	if p, s, ok := atsdetect.Detect(home); ok {
		return p, s, true
	}

	seen := map[string]bool{base: true}
	var candidates []string
	for _, path := range careerPaths {
		candidates = append(candidates, base+path)
	}
	if link := careersLink(home, base); link != "" {
		candidates = append(candidates, link)
	}

	for _, u := range candidates {
		if seen[u] {
			continue
		}
		seen[u] = true
		html, err := fetch(u)
		if err != nil {
			continue
		}
		if p, s, ok := atsdetect.Detect(html); ok {
			return p, s, true
		}
	}
	return "", "", false
}
