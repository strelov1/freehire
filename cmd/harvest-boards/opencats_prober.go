package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// opencatsListingPath is the portal's "show all" route, appended to a board (mirrors the
// unexported constant in the sources package; this tool lives outside it).
const opencatsListingPath = "index.php?m=careers&p=showAll"

// opencatsPostingLink matches a portal link that is a posting. It is the liveness signal: a
// portal serving at least one is a live board.
var opencatsPostingLink = regexp.MustCompile(`p=showJob`)

// opencatsDiscoveryQueries are the signatures an install leaves in a public scan index. The
// stock page title is by far the richest (66 hosts against 13 for the URL routing) because
// administrators move the portal or put it behind a rewrite far more often than they rename
// the page; the routing queries add the installs that did rename it.
var opencatsDiscoveryQueries = []string{
	`page.title:"OpenCATS"`,
	`page.url:"careers/index.php"`,
	`page.url:"m=careers"`,
}

// opencatsSearchURL builds a urlscan.io search request for one signature.
func opencatsSearchURL(q string) string {
	return "https://urlscan.io/api/v1/search/?q=" + url.QueryEscape(q) + "&size=100"
}

// opencatsMounts are the paths an install serves its portal under, in the order they are
// tried. Most installs nest it; a rewritten portal (G4S) serves it at the web root.
var opencatsMounts = []string{"/careers", ""}

// opencatsProber probes and discovers self-hosted OpenCATS portals. OpenCATS has no vendor
// API and no tenant catalogue — every install is somebody's own domain — so candidates come
// from a public index of scanned hosts rather than from a seed list or a platform feed.
type opencatsProber struct{}

// probe requests the board's portal listing and counts its postings. An unreachable host, a
// host serving no portal, and a portal with no postings are all a skip — ("", 0, nil) — so one
// dead candidate cannot abort the harvest.
func (opencatsProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	root, err := c.GetHTML(ctx, opencatsListingURL(board))
	if err != nil {
		return "", 0, nil
	}
	jobs := countMatchingLinks(root, opencatsPostingLink)
	if jobs == 0 {
		return "", 0, nil
	}
	return opencatsCompanyName(root, board), jobs, nil
}

// discover unions the signature queries into one candidate host list, then resolves each host
// to the single board it serves. Resolving here rather than in probe is deliberate: a board is
// its mount point, so a host emitted at two mounts would namespace one posting under two
// external ids, and only discovery sees the whole host.
func (p opencatsProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	seen := map[string]struct{}{}
	var hosts []string
	failures := 0
	for _, q := range opencatsDiscoveryQueries {
		var resp struct {
			Results []struct {
				Task struct {
					Domain string `json:"domain"`
				} `json:"task"`
			} `json:"results"`
		}
		if err := c.GetJSON(ctx, opencatsSearchURL(q), &resp); err != nil {
			failures++
			continue
		}
		for _, r := range resp.Results {
			h := strings.ToLower(strings.TrimSpace(r.Task.Domain))
			if h == "" || !opencatsEligibleHost(h) {
				continue
			}
			if _, ok := seen[h]; ok {
				continue
			}
			seen[h] = struct{}{}
			hosts = append(hosts, h)
		}
	}
	// Every query failing means the index is unreachable or has changed, not that no installs
	// exist. Say so, rather than returning an empty list that reads as an exhausted search.
	if failures == len(opencatsDiscoveryQueries) {
		return nil, fmt.Errorf("opencats: all %d discovery queries failed", failures)
	}

	var boards []string
	for _, h := range hosts {
		if b := opencatsResolveMount(ctx, c, h); b != "" {
			boards = append(boards, b)
		}
	}
	return boards, nil
}

// opencatsResolveMount returns the single board a host serves its portal as, or "" when the
// host serves no portal with postings. A host answering at more than one mount yields the
// first, so one host is always one board.
func opencatsResolveMount(ctx context.Context, c httpClient, host string) string {
	for _, mount := range opencatsMounts {
		board := host + mount
		root, err := c.GetHTML(ctx, opencatsListingURL(board))
		if err != nil {
			continue
		}
		if countMatchingLinks(root, opencatsPostingLink) > 0 {
			return board
		}
	}
	return ""
}

// opencatsListingURL builds the portal listing URL for a board (host plus optional mount).
func opencatsListingURL(board string) string {
	return "https://" + strings.Trim(board, "/") + "/" + opencatsListingPath
}

// opencatsIneligibleSuffixes name hosts that must never enter the board file: commercial CATS
// shares the URL scheme with its open-source descendant and is already crawled under its own
// provider — admitting it creates the same posting under two providers, which the
// (source, external_id) dedup key cannot detect — and the project's own sites are
// documentation and demos, not an employer's portal.
var opencatsIneligibleSuffixes = []string{"catsone.com", "opencats.org"}

// opencatsEligibleHost reports whether a discovered host can be a company's portal.
func opencatsEligibleHost(host string) bool {
	if net.ParseIP(host) != nil {
		return false
	}
	for _, s := range opencatsIneligibleSuffixes {
		if host == s || strings.HasSuffix(host, "."+s) {
			return false
		}
	}
	return true
}

// opencatsTitleSuffix matches the "- Careers" tail installs append to the portal title.
var opencatsTitleSuffix = regexp.MustCompile(`(?i)\s*[-–—|]\s*careers\s*$`)

// opencatsCompanyName proposes a company name from the portal page title, or "" when the
// install never set one. Deriving a name from the host instead would be indistinguishable
// from a name the portal actually published — and the name a prober returns is what the
// corroboration gate tests, so a derived one would gate a board against a token the employer
// never chose. An unnamed board falls back to the seed's name, then to the board id.
func opencatsCompanyName(root *html.Node, _ string) string {
	return strings.TrimSpace(opencatsTitleSuffix.ReplaceAllString(pageTitle(root), ""))
}
