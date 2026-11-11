package main

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// paycorProber validates a Paycor Recruiting clientId by counting the postings its
// CareerHome listing links. One request settles liveness — an unknown clientId 404s, and the
// listing renders the board's whole open catalogue with no pagination — and because that list
// is complete the prober can also answer the expected-id check.
//
// It publishes no employer name. A tenant themes its listing with its own website chrome, so
// the page title is marketing copy rather than a name, and the seed's company is what labels
// the entry; the expected-id check is what ties that name to the board. See
// internal/ingest/sources/paycor.go for the board format.
type paycorProber struct{}

// Reject only when the complete listing has at least ten distinct placeholder-title postings
// and those postings reach 2% of the catalogue. The absolute floor protects small employers;
// the share floor protects large employers with a few incidental QA or test roles.
const (
	paycorPlaceholderTitleMinimum = 10
	paycorPlaceholderTitlePercent = 2
)

var paycorPlaceholderTitle = regexp.MustCompile(strings.Join([]string{
	`(?i)\badvertise\d*\b`,
	`\b(?:edge|incognito)test\d*\b`,
	`\bstandardnewdemo\d*\b`,
	`\btest[-_\s]*(?:job|posting|listing|position|role)(?:\b|_)`,
	`^test$`,
	`[([][^)\]]*\b(?:test|demo)\b[^)\]]*[)]]`,
	`\b(?:immediate|maya|revi)\s+test\b`,
	`\btest\s+\d`,
	`\b(?:demo|sample)[-_\s]*(?:job|posting|listing|position|role)\b`,
}, "|"))

// paycorJobAction is the last path segment of a posting page, the shape enumeration matches
// on. It mirrors internal/ingest/sources/paycor.go, whose constants are unexported — this
// tool lives outside that package.
const paycorJobAction = "JobIntroduction.action"

// dedupKey folds a Paycor client id to lower case. The id is hex and the portal serves the
// same employer in either case, so an upper-case spelling of a board the file already holds
// is the same board — without this it probes live and is appended a second time.
func (paycorProber) dedupKey(clientID string) string { return strings.ToLower(clientID) }

func (p paycorProber) probe(ctx context.Context, c httpClient, clientID string) (string, int, error) {
	ids, placeholderTitles, err := p.postingIDsAndPlaceholderTitleCount(ctx, c, clientID)
	if err != nil {
		// A missing or closed board 404s and the client surfaces it as an error. For harvest
		// that just means "not a live paycor board" — skip silently, do not propagate.
		return "", 0, nil
	}
	if len(ids) > 0 && placeholderTitles >= paycorPlaceholderTitleMinimum &&
		placeholderTitles*100 >= len(ids)*paycorPlaceholderTitlePercent {
		return "", 0, nil
	}
	return "", len(ids), nil
}

// postingIDs lists the board's live posting ids. The listing renders every open posting in
// one page, so an id it does not carry is genuinely not on the board.
func (paycorProber) postingIDs(ctx context.Context, c httpClient, clientID string) ([]string, error) {
	ids, _, err := (paycorProber{}).postingIDsAndPlaceholderTitleCount(ctx, c, clientID)
	return ids, err
}

// postingIDsAndPlaceholderTitleCount reads the complete listing and counts unique postings
// whose title signals a vendor placeholder, even when an earlier link for that posting is an
// untitled Apply link.
func (paycorProber) postingIDsAndPlaceholderTitleCount(ctx context.Context, c httpClient, clientID string) ([]string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf(
		"https://recruitingbypaycor.com/career/CareerHome.action?clientId=%s", clientID))
	if err != nil {
		return nil, 0, err
	}
	var ids []string
	seen := map[string]bool{}
	placeholderSeen := map[string]bool{}
	placeholderTitles := 0
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key != "href" {
					continue
				}
				id := paycorPostingID(a.Val, clientID)
				if id == "" {
					continue
				}
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
				if !placeholderSeen[id] && paycorPlaceholderTitle.MatchString(paycorListingTitle(n)) {
					placeholderSeen[id] = true
					placeholderTitles++
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			visit(ch)
		}
	}
	visit(root)
	return ids, placeholderTitles, nil
}

// paycorListingTitle prefers Paycor's title-bearing ns-qa attribute, then uses visible link text.
func paycorListingTitle(n *html.Node) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, "ns-qa") && strings.TrimSpace(a.Val) != "" {
			return strings.TrimSpace(a.Val)
		}
	}
	var title strings.Builder
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.TextNode {
			title.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(n)
	return strings.TrimSpace(title.String())
}

// paycorPostingID returns the posting id an href addresses on the given board, or "" when it
// addresses no posting on it. It restates the adapter's own paycorPostingID, whose comment
// argues which hrefs are excluded and why; the two must agree, or the harvest validates a
// board the crawl then reads differently.
func paycorPostingID(href, clientID string) string {
	u, err := url.Parse(href)
	if err != nil || path.Base(u.Path) != paycorJobAction {
		return ""
	}
	q := u.Query()
	if !strings.EqualFold(q.Get("clientId"), clientID) {
		return ""
	}
	return q.Get("id")
}
