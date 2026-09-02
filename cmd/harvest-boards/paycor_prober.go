package main

import (
	"context"
	"fmt"
	"net/url"
	"path"
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

// paycorJobAction is the last path segment of a posting page, the shape enumeration matches
// on. It mirrors internal/ingest/sources/paycor.go, whose constants are unexported — this
// tool lives outside that package.
const paycorJobAction = "JobIntroduction.action"

// dedupKey folds a Paycor client id to lower case. The id is hex and the portal serves the
// same employer in either case, so an upper-case spelling of a board the file already holds
// is the same board — without this it probes live and is appended a second time.
func (paycorProber) dedupKey(clientID string) string { return strings.ToLower(clientID) }

func (p paycorProber) probe(ctx context.Context, c httpClient, clientID string) (string, int, error) {
	ids, err := p.postingIDs(ctx, c, clientID)
	if err != nil {
		// A missing or closed board 404s and the client surfaces it as an error. For harvest
		// that just means "not a live paycor board" — skip silently, do not propagate.
		return "", 0, nil
	}
	return "", len(ids), nil
}

// postingIDs lists the board's live posting ids. The listing renders every open posting in
// one page, so an id it does not carry is genuinely not on the board.
func (paycorProber) postingIDs(ctx context.Context, c httpClient, clientID string) ([]string, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf(
		"https://recruitingbypaycor.com/career/CareerHome.action?clientId=%s", clientID))
	if err != nil {
		return nil, err
	}
	var ids []string
	seen := map[string]bool{}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key != "href" {
					continue
				}
				if id := paycorPostingID(a.Val, clientID); id != "" && !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			visit(ch)
		}
	}
	visit(root)
	return ids, nil
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
