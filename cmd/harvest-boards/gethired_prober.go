package main

import (
	"context"
	"fmt"
	"strings"
)

// gethiredProber probes a GetHired tenant's RSS feed (<board>.easyapply.co/rss), the adapter's own
// request. A non-empty channel is a live board and its title is the employer. An unserved subdomain
// redirects to a sign-in page whose HTML fails the decode and is skipped.
type gethiredProber struct{}

func (gethiredProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var feed struct {
		Title string     `xml:"channel>title"`
		Items []struct{} `xml:"channel>item"`
	}
	if err := c.GetXML(ctx, fmt.Sprintf("https://%s.easyapply.co/rss", slug), &feed); err != nil {
		return "", 0, nil
	}
	if len(feed.Items) == 0 {
		return "", 0, nil
	}
	return strings.TrimSpace(feed.Title), len(feed.Items), nil
}
