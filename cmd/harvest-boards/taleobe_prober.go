package main

import (
	"context"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// taleobeProber probes a Taleo Business Edition career site ("<ORG>:<cws>") through the adapter's
// RSS request. A non-empty channel is a live board and its title names the employer. An unknown or
// deactivated org, or a missing career-site number, redirects to HTML and fails the decode.
type taleobeProber struct{}

func (taleobeProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	feedURL, err := sources.TaleoBEFeedURL(board)
	if err != nil {
		return "", 0, nil
	}
	var feed struct {
		Title string     `xml:"channel>title"`
		Items []struct{} `xml:"channel>item"`
	}
	if err := c.GetXML(ctx, feedURL, &feed); err != nil {
		return "", 0, nil
	}
	if len(feed.Items) == 0 {
		return "", 0, nil
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(feed.Title), " Job Feed")), len(feed.Items), nil
}
