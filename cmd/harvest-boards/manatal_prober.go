package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// manatalProber validates a Manatal board "<tenant>" by counting the postings linked from its
// hosted career page ("careers-page.com/<tenant>") and reading the tenant's own name off that
// page's title.
//
// The name matters more here than on most platforms. Manatal is heavily used by recruitment
// AGENCIES, so a posting's hiring company is routinely NOT the board's owner — a seed built
// from an aggregator's hiringOrganization labelled three different agency boards with the same
// client, and the adapter stamps every posting with the entry's company. The career page names
// the tenant that owns the board, which is the only correct label.
//
// Without this the provider fell through to adapterProber, which runs a whole crawl per
// candidate and publishes no name at all.
type manatalProber struct{}

// manatalJobLink captures a posting's id from a Manatal career-page job link
// (/<tenant>/job/<id>), so the title and apply links to one posting count once.
var manatalJobLink = regexp.MustCompile(`/job/([A-Za-z0-9]+)`)

// manatalTitleSuffix is what Manatal appends to every hosted career page's title. The tenant
// name is whatever precedes it.
const manatalTitleSuffix = "| Career Page"

func (manatalProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://careers-page.com/%s", board))
	if err != nil {
		return "", 0, nil
	}
	ids := map[string]bool{}
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "a":
				for _, a := range n.Attr {
					if a.Key == "href" {
						if m := manatalJobLink.FindStringSubmatch(a.Val); m != nil {
							ids[m[1]] = true
						}
					}
				}
			case "title":
				if title == "" && n.FirstChild != nil {
					title = n.FirstChild.Data
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	if len(ids) == 0 {
		return "", 0, nil
	}
	return manatalTenantName(title), len(ids), nil
}

// manatalTenantName pulls the tenant out of a Manatal career-page title, which reads
// " - <tenant> | Career Page". It cuts at the LAST suffix occurrence so a tenant whose own
// name contains a pipe survives, and returns "" when the title carries no name — the caller
// then falls back rather than storing a fragment of boilerplate as a company.
func manatalTenantName(title string) string {
	i := strings.LastIndex(title, manatalTitleSuffix)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(title[:i]), "-"))
}
