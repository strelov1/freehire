package main

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// homerunProber validates a Homerun tenant (the <board> of douglas.homerun.co) by reading the
// Atom feed the adapter lists from, feed.homerun.co/<board>, and counting its entries. It mirrors
// internal/ingest/sources/homerun.go, which is unexported.
//
// The feed's title is the career site's, which is the employer's name or a heading built on it
// ("Vacatures - Werken bij Douglas NL", "Werken bij Belsimpel"); the heading words are cut. An
// unknown or switched-off tenant's feed redirects to 404.homerun.co, which answers 404: a skip.
//
// Homerun refuses by rate and then blocks the address (see homerunRequestInterval in
// internal/ingest/sources/pacer.go): run it with -pace 2 or slower.
type homerunProber struct{}

var homerunBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// homerunTitleHeading matches the career-site heading Homerun tenants put before their name.
var homerunTitleHeading = regexp.MustCompile(`(?i)^(vacatures\s*[-–|:]\s*)?(werken bij|working at|jobs at|careers at)\s+`)

func (homerunProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !homerunBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	var feed struct {
		XMLName xml.Name   `xml:"feed"`
		Title   string     `xml:"title"`
		Entries []struct{} `xml:"entry"`
	}
	if err := c.GetXML(ctx, "https://feed.homerun.co/"+board, &feed); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	name := homerunTitleHeading.ReplaceAllString(strings.TrimSpace(feed.Title), "")
	return strings.TrimSpace(name), len(feed.Entries), nil
}

// discover enumerates candidate tenants from Common Crawl's index of *.homerun.co. Homerun's DNS
// answers every label, so a sweep proves nothing; the crawl's tenant hosts are the seed (874
// labels in five snapshots on 2026-10-25, 623 with a feed).
func (homerunProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.homerun.co", recognizedBoard("homerun"))
}
