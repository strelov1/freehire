package main

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/atsdetect"
)

// careerPaths are the common careers/jobs locations probed on a company site, in
// addition to the homepage and a careers link discovered on it.
var careerPaths = []string{"/careers", "/jobs", "/career", "/vacancies", "/employment", "/about/careers", "/join-us"}

// careerHostPrefixes are separate careers-host guesses tried once the site's own
// pages reveal no board — universities and large orgs commonly run recruitment on
// a dedicated host (jobs.<domain>, careers.<domain>) not linked from the homepage.
var careerHostPrefixes = []string{"careers", "jobs", "hr"}

// anchorRe extracts (href, inner-text) from each <a> tag (case-insensitive, dotall
// so the text may span lines). Both quote styles are accepted — some sites write
// href='...'.
var anchorRe = regexp.MustCompile(`(?is)<a\b[^>]*\bhref=["']([^"']*)["'][^>]*>(.*?)</a>`)

// careersLink returns the first anchor on a homepage that points at a careers/jobs
// page — matched on either the href or the link text containing "career"/"job" —
// resolved to an absolute URL against base. It returns "" when none is found.
func careersLink(html, base string) string {
	return firstLink(html, base, looksCareers)
}

// deeperLink returns the first anchor on a careers page that points one level deeper
// into the recruitment section (a vacancy listing / apply / open-positions page),
// where the ATS board is often linked instead of on the careers landing page.
func deeperLink(html, base string) string {
	return firstLink(html, base, deepRe.MatchString)
}

// firstLink returns the first anchor whose lowercased href or text satisfies match,
// resolved to an absolute URL against base, or "" when none does.
func firstLink(html, base string, match func(string) bool) string {
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	for _, m := range anchorRe.FindAllStringSubmatch(html, -1) {
		href, text := m[1], strings.ToLower(stripTags(m[2]))
		if match(strings.ToLower(href)) || match(text) {
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

// deepRe matches the link words a careers landing page uses to reach its live
// vacancy listing (the page that actually embeds the ATS board).
var deepRe = regexp.MustCompile(`(?i)(vacan|opening|apply|position|current-|search.?job|job.?search|opportunit)`)

// careerHosts derives dedicated careers-host guesses (jobs.<domain>, careers.<domain>)
// from a site's base URL, dropping a leading "www." so the prefix lands on the apex.
func careerHosts(base string) []string {
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	domain := strings.TrimPrefix(u.Hostname(), "www.")
	out := make([]string, 0, len(careerHostPrefixes))
	for _, p := range careerHostPrefixes {
		out = append(out, u.Scheme+"://"+p+"."+domain)
	}
	return out
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
	candidates = append(candidates, careerHosts(base)...)
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
		// One deeper hop: a careers landing page often links to the live vacancy
		// listing (which embeds the board) rather than the board itself.
		if link := deeperLink(html, u); link != "" && !seen[link] {
			seen[link] = true
			if dh, err := fetch(link); err == nil {
				if p, s, ok := atsdetect.Detect(dh); ok {
					return p, s, true
				}
			}
		}
	}
	return "", "", false
}
