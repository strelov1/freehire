package main

import (
	"context"
	"fmt"
	"strings"
)

// trakstarProber probes the Trakstar Hire per-subdomain RSS feed. A non-empty channel of
// items is a live board, and the channel title names the employer, so the board can be gated
// against the name the seed expected instead of accepted on liveness alone.
type trakstarProber struct{}

func (trakstarProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var feed struct {
		Title string     `xml:"channel>title"`
		Items []struct{} `xml:"channel>item"`
	}
	if err := c.GetXML(ctx, fmt.Sprintf("https://%s.hire.trakstar.com/jobfeeds/%s", slug, slug), &feed); err != nil {
		return "", 0, nil
	}
	if len(feed.Items) == 0 {
		return "", 0, nil
	}
	return trakstarEmployer(feed.Title), len(feed.Items), nil
}

// trakstarEmployer pulls the employer from a feed title "Jobs at <Company>"; anything else yields "".
func trakstarEmployer(title string) string {
	name, ok := strings.CutPrefix(strings.TrimSpace(title), "Jobs at ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(name)
}
