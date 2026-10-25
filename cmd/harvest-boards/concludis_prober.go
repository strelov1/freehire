package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// concludisProber validates a concludis tenant (the <board> of hhla.concludis.de) by reading
// page one of the complete list the adapter walks, which states the tenant's number of open
// positions. It mirrors internal/ingest/sources/concludis.go, which is unexported.
//
// A tenant that publishes on its own site instead redirects the list there, so the page that
// comes back carries no concludis count and the candidate is skipped. The employer is the first
// entry of the list page's keywords meta, which every live tenant measured (2026-10-25, 28 of
// 28) fills with its company name (one pads it with byte-order marks) — or with its own label,
// which names nobody and is dropped.
type concludisProber struct{}

const concludisListURL = "https://%s.concludis.de/prj/lst/a181a603769c1f98ad927e7367c7aa51/GesamtlisteOffenePositionen.htm"

var (
	concludisBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	concludisStatedCount  = regexp.MustCompile(`class="stellensum">\s*([0-9]+)\s+Stellen?\s+gefunden`)
	concludisNoneStated   = regexp.MustCompile(`class="stellensum">[^<]*keine offenen Stellen`)
	concludisKeywords     = regexp.MustCompile(`<meta name="keywords" content="([^",]*)`)
)

func (concludisProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !concludisBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	body, err := c.GetText(ctx, fmt.Sprintf(concludisListURL, board))
	if err != nil {
		// An unknown label redirects onto the vendor's marketing site, which answers 404.
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code != http.StatusNotFound {
			return "", 0, err
		}
		return "", 0, nil
	}
	if concludisNoneStated.MatchString(body) {
		return "", 0, nil
	}
	m := concludisStatedCount.FindStringSubmatch(body)
	if m == nil {
		return "", 0, nil // redirected to the tenant's own site: nothing the adapter can read
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return "", 0, nil
	}
	name := ""
	if k := concludisKeywords.FindStringSubmatch(body); k != nil {
		name = strings.Join(strings.Fields(strings.ReplaceAll(html.UnescapeString(k[1]), "\ufeff", "")), " ")
		if strings.EqualFold(name, board) {
			name = ""
		}
	}
	return name, n, nil
}

// discover enumerates candidate tenants from Common Crawl's index of *.concludis.de.
func (concludisProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.concludis.de", recognizedBoard("concludis"))
}
