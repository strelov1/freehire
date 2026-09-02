package main

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// workstreamProber validates a Workstream board — an employer's eight-hex career-site id — by
// counting the postings its positions listing links. It also discovers its own candidates: the
// platform publishes no directory of career sites, its sitemap.xml is the marketing site's and
// carries no "/j/" URL at all, so Common Crawl's record of the pages it has visited is the only
// enumeration there is.
//
// It reports the employer name the site titles itself with, which is the name the board file
// then carries. That is safe to gate on here in a way it is not on a name-less platform: the
// title is the account's own display name, not a heading a tenant typed.
type workstreamProber struct{}

// workstreamJobHost is the path prefix Workstream serves career sites under — a path, not a
// bare host, because the marketing site owns the same domain's root.
const workstreamJobHost = "www.workstream.us/j"

// workstreamBoardID matches a board id: the eight lower-case hex characters a canonical career
// site URL opens with. Common Crawl also surfaces the legacy "/j/<employer-slug>" spelling that
// 301-redirects onto one of these; those are dropped rather than followed, because resolving
// them costs a request each and measured live they yielded 4 boards the hex ids did not already
// carry, against 63 redirects to chase.
var workstreamBoardID = regexp.MustCompile(`^[0-9a-f]{8}$`)

// workstreamPositionLink matches a posting permalink's path, mirroring the adapter's own rule
// (internal/ingest/sources/workstream.go): "/j/<board>/<brand>/<store>/<title>-<id>".
var workstreamPositionLink = regexp.MustCompile(`^/j/[0-9a-f]{8}/[^/]+/[^/]+/[^/]+-([0-9a-f]{8})$`)

// workstreamSearchBase captures the positions-listing URL a career-site page states for itself.
// An employer running exactly one brand redirects "/j/<board>/positions" to that brand's root,
// which lists LOCATIONS — so the count has to come from wherever this variable points, not from
// whatever the redirect happened to land on.
var workstreamSearchBase = regexp.MustCompile(`var searchBaseUrl = '([^']+)'`)

// workstreamTitleSuffix is what a career site appends to the employer's name in its <title>
// ("FineCasual Careers and Jobs"). Only its opening is matched, because the site TRUNCATES a
// long title mid-suffix ("Good Charlie's Oyster Bar & Seafood Kitchen Careers and...") and
// trimming the whole phrase leaves that debris in the employer name the board file then carries.
const workstreamTitleSuffix = " Careers and"

// discover enumerates candidate boards from Common Crawl's index of www.workstream.us/j/*,
// keeping the eight-hex employer ids.
func (workstreamProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, workstreamJobHost, workstreamCandidate)
}

// workstreamCandidate slices a crawled career-site URL to its board id — the segment after
// "/j/" — reporting ok=false for anything that is not one (the legacy slug spelling, the
// "/j/share" and "/j/css" service paths, a bare "/j/").
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

// probe counts the postings on the board's first listing page. It is one page, not the whole
// board: liveness is what the harvest needs and one page settles it, and paging a frontline
// employer's whole board would spend hundreds of requests against a platform that meters by
// rate. So the count reported is a floor, not a total.
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
	if i := strings.Index(title, workstreamTitleSuffix); i > 0 {
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
