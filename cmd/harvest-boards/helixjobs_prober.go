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

// helixjobsProber validates a helixjobs iid (the <board> of helixjobs.com/_/joblist?iid=146) by
// reading page one of the list the adapter walks, which states the iid's number of postings and
// names the employer on every line. It mirrors internal/ingest/sources/helixjobs.go, which is
// unexported. The portal answers an unknown iid exactly as one with no posting — "Keine
// Ergebnisse gefunden" — so both are skipped.
type helixjobsProber struct{}

const helixjobsListURL = "https://helixjobs.com/_/joblist?iid=%s&page=%d"

var (
	helixjobsBoardPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
	helixjobsStatedCount  = regexp.MustCompile(`(?s)class="result-text"[^>]*>.*?<strong>\s*([0-9][0-9.]*)\s*</strong>\s*Treffer`)
	// helixjobsLineCompany reads the employer a list line names.
	helixjobsLineCompany = regexp.MustCompile(`(?s)<li class="company-name">.*?<span>([^<]+)</span>`)
	// helixjobsLineIID reads the iid prefix of every posting a page links.
	helixjobsLineIID = regexp.MustCompile(`/_/jobad\?prj=([1-9][0-9]*)[Pp][0-9]+`)
)

// helixjobsDiscoveryMaxPages bounds the portal walk discover makes: the whole portal measured
// 1,001 pages (25,021 postings, 2026-10-25).
const helixjobsDiscoveryMaxPages = 2000

func (helixjobsProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !helixjobsBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	body, err := c.GetText(ctx, fmt.Sprintf(helixjobsListURL, board, 1))
	if err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	m := helixjobsStatedCount.FindStringSubmatch(body)
	if m == nil {
		return "", 0, nil // no posting, or no such iid: the portal does not tell them apart
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m[1], ".", ""))
	if err != nil {
		return "", 0, nil
	}
	name := ""
	if k := helixjobsLineCompany.FindStringSubmatch(body); k != nil {
		name = strings.Join(strings.Fields(html.UnescapeString(k[1])), " ")
	}
	return name, n, nil
}

// discover enumerates candidate iids from the portal's own list of every posting: each line's
// posting id opens with its iid. The walk ends at the first page that links no posting.
func (helixjobsProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for page := 1; page <= helixjobsDiscoveryMaxPages; page++ {
		body, err := c.GetText(ctx, fmt.Sprintf("https://helixjobs.com/_/joblist?page=%d", page))
		if err != nil {
			return nil, fmt.Errorf("helixjobs discover: page %d: %w", page, err)
		}
		ms := helixjobsLineIID.FindAllStringSubmatch(body, -1)
		if len(ms) == 0 {
			return out, nil
		}
		for _, m := range ms {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return nil, fmt.Errorf("helixjobs discover: %d pages without reaching the end", helixjobsDiscoveryMaxPages)
}
