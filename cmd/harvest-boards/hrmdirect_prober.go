package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// hrmdirectProber validates an HRM Direct board "<slug>" by asking ClearCompany's career-site
// API how many postings the tenant lists. HRM Direct tenants are served by ClearCompany since
// ~2026-10-21: the hosted career page ("apply.clearcompany.com/<slug>") renders no posting and
// names the tenant's career-site id in its widget script, and the API answers a page of
// postings with the total. This mirrors the adapter (internal/ingest/sources/hrmdirect.go),
// which explains the move.
//
// It reports no company name. The career page's title is free text, not an employer field, so
// the seed's own name labels the board instead — the same reading careerplugProber takes.
type hrmdirectProber struct{}

func (hrmdirectProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	root, err := c.GetHTML(ctx, "https://apply.clearcompany.com/"+url.PathEscape(slug))
	if err != nil {
		return "", 0, nil
	}
	siteID := hrmdirectSiteID(root)
	if siteID == "" {
		return "", 0, nil
	}
	var resp struct {
		TotalCount int `json:"totalCount"`
	}
	u := fmt.Sprintf("https://careers-api.clearcompany.com/v1/%s?pageIndex=0&pageSize=1", url.PathEscape(siteID))
	if err := c.GetJSON(ctx, u, &resp); err != nil {
		return "", 0, nil
	}
	return "", resp.TotalCount, nil
}

// hrmdirectSiteID returns the first data-site-id attribute in the page, or "".
func hrmdirectSiteID(n *html.Node) string {
	if n.Type == html.ElementNode {
		for _, a := range n.Attr {
			if a.Key == "data-site-id" {
				if id := strings.TrimSpace(a.Val); id != "" {
					return id
				}
			}
		}
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if id := hrmdirectSiteID(ch); id != "" {
			return id
		}
	}
	return ""
}
