package main

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// workstreamProber validates a Workstream board (an employer's eight-hex career-site id) by counting
// the postings its positions listing links. It discovers its own candidates: the platform has no
// directory and its sitemap.xml carries no "/j/" URL, so Common Crawl is the only enumeration.
//
// It reports the employer name the site titles itself with, which is safe to gate on: the title is
// the account's own display name, not a heading a tenant typed.
type workstreamProber struct{}

// workstreamJobHost is the path prefix career sites are served under (a path, not a bare host,
// because the marketing site owns the domain's root).
const workstreamJobHost = "www.workstream.us/j"

// workstreamBoardID matches a board id: the eight lower-case hex characters a canonical career site
// URL opens with. The legacy "/j/<employer-slug>" spelling 301-redirects to one of these and is
// dropped: resolving it costs a request each and found only 4 boards against 63 redirects.
var workstreamBoardID = regexp.MustCompile(`^[0-9a-f]{8}$`)

// workstreamPositionLink matches a posting permalink's path, mirroring the adapter's own rule
// (internal/ingest/sources/workstream.go): "/j/<board>/<brand>/<store>/<title>-<id>".
var workstreamPositionLink = regexp.MustCompile(`^/j/[0-9a-f]{8}/[^/]+/[^/]+/[^/]+-([0-9a-f]{8})$`)

// workstreamSearchBase captures the positions-listing URL a career-site page states for itself. A
// single-brand employer redirects "/j/<board>/positions" to a brand root that lists LOCATIONS, so the
// count must come from wherever this variable points.
var workstreamSearchBase = regexp.MustCompile(`var searchBaseUrl = '([^']+)'`)

// workstreamTitleSuffix is what a career site appends to the employer's name in its <title>. Only
// its opening is matched, because a long title is TRUNCATED mid-suffix and trimming the whole phrase
// would leave debris in the employer name.
const workstreamTitleSuffix = " Careers and"

// discover enumerates candidate boards from Common Crawl's index of www.workstream.us/j/*,
// keeping the eight-hex employer ids.
func (workstreamProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, workstreamJobHost+"/*", workstreamCandidate)
}

// workstreamCandidate slices a crawled career-site URL to its board id (the segment after "/j/"),
// reporting ok=false for anything else (legacy slug spelling, "/j/share", "/j/css", a bare "/j/").
func workstreamCandidate(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "j" || !workstreamBoardID.MatchString(parts[1]) {
		return "", false
	}
	return parts[1], true
}

// probe counts the postings on the board's first listing page only: one page settles liveness, and
// paging a whole frontline board would spend hundreds of requests on a rate-metered platform. The
// count is a floor, not a total.
func (workstreamProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	root, err := c.GetHTML(ctx, "https://"+workstreamJobHost+"/"+board+"/positions")
	if err != nil {
		return "", 0, nil // absent (404), retired (410) or unreachable — not a live board
	}
	base := firstSubmatch(workstreamSearchBase, workstreamScriptText(root))
	if base == "" {
		return "", 0, nil // not a career site
	}
	if base != "https://"+workstreamJobHost+"/"+board+"/positions" {
		if root, err = c.GetHTML(ctx, base); err != nil {
			return "", 0, nil
		}
	}
	n := workstreamPostingCount(root)
	if n == 0 {
		return "", 0, nil
	}
	return workstreamEmployer(pageTitle(root)), n, nil
}

// workstreamEmployer reads the employer name out of a career site's <title>, cutting the
// platform's own suffix off whether the site rendered it whole or truncated.
func workstreamEmployer(title string) string {
	if i := strings.Index(title, workstreamTitleSuffix); i >= 0 {
		return strings.TrimSpace(title[:i])
	}
	return title
}

// workstreamPostingCount counts the distinct postings a listing page links. A card links the same
// posting from its title, its row and its arrow, so counting links would treble the figure.
func workstreamPostingCount(root *html.Node) int {
	ids := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key != "href" {
					continue
				}
				if u, err := url.Parse(a.Val); err == nil {
					if m := workstreamPositionLink.FindStringSubmatch(u.Path); m != nil {
						ids[m[1]] = true
					}
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return len(ids)
}

// workstreamScriptText concatenates a page's inline script text, where the site states its
// listing URL. The block carries no id, so the blocks are read together.
func workstreamScriptText(root *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" {
			for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
				if ch.Type == html.TextNode {
					b.WriteString(ch.Data)
				}
			}
			b.WriteByte('\n')
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return b.String()
}
