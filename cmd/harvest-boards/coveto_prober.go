package main

import (
	"context"
	"encoding/xml"
	"errors"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// covetoProber validates a coveto tenant (the k<number> label of k60360.coveto.de) by reading
// the RSS feed the adapter lists from, and counting its items. It mirrors
// internal/ingest/sources/coveto.go, which is unexported.
//
// The feed's channel title names the employer ("Aktuelle Jobs bei Stadtmission Nürnberg
// e.V."). A switched-off tenant serves an HTML notice in place of the feed, and a host that does
// not resolve is a customer number nobody holds; both are a skip.
type covetoProber struct{}

var covetoBoardPattern = regexp.MustCompile(`^k[0-9]{3,7}$`)

func (covetoProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !covetoBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	var feed struct {
		XMLName xml.Name `xml:"rss"`
		Channel *struct {
			Title string     `xml:"title"`
			Items []struct{} `xml:"item"`
		} `xml:"channel"`
	}
	if err := c.GetXML(ctx, "https://"+board+".coveto.de/public/rss/", &feed); err != nil {
		// A status is the platform answering something other than a feed, which is worth
		// seeing. Anything else is the HTML notice failing the decode, or a customer number
		// nobody holds failing to resolve — an absent board either way.
		var se *sources.StatusError
		if errors.As(err, &se) {
			return "", 0, err
		}
		return "", 0, nil
	}
	if feed.Channel == nil {
		return "", 0, nil
	}
	name := strings.TrimSpace(feed.Channel.Title)
	if _, after, ok := strings.Cut(name, " bei "); ok {
		name = strings.TrimSpace(after)
	} else {
		name = ""
	}
	return name, len(feed.Channel.Items), nil
}

// discover enumerates candidate tenants from Common Crawl's index of *.coveto.de. Common Crawl
// sees a small share of them: a tenant's label is its customer number and coveto's DNS has no
// wildcard, so a sweep of the number range resolving k<n>.coveto.de (1,191 hosts on 2026-10-25,
// against 100 in six Common Crawl snapshots) is the fuller seed, passed as a file.
func (covetoProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.coveto.de", recognizedBoard("coveto"))
}
