package main

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// erecruiterProber validates an eRecruiter board (the 32-hex cfg of skk.erecruiter.pl) with the
// adapter's own first list page and names the employer from the first offer — two requests, where
// running the adapter would read every offer's detail. A board's cfg cannot be guessed, so the
// seed is the discovery (Common Crawl's Offer.aspx URLs carry it); see cmd/harvest-erecruiter for
// the careers-page route to the same token.
type erecruiterProber struct{}

var erecruiterCfgPattern = regexp.MustCompile(`^[0-9A-Fa-f]{32}$`)

func (erecruiterProber) probe(ctx context.Context, c httpClient, cfg string) (string, int, error) {
	if !erecruiterCfgPattern.MatchString(cfg) {
		return "", 0, nil
	}
	return sources.ProbeErecruiterBoard(ctx, c, cfg)
}

// maxRequestsPerSecond keeps a harvest polite toward skk.erecruiter.pl, the one host every
// company's board is served from.
func (erecruiterProber) maxRequestsPerSecond() float64 { return 1 }

// recruitifyProber validates a Recruitify portal (the <board> of linkgroup.recruitify.ai) against
// the API the adapter reads: the portal's config names the company, and the first listing page
// proves it has openings. It mirrors internal/ingest/sources/recruitify.go, which is unexported.
// The API tells portals apart by Origin alone, and answers an unknown one 404.
type recruitifyProber struct{}

var recruitifyBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (recruitifyProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !recruitifyBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	origin := "https://" + board + ".recruitify.ai"
	headers := map[string]string{"Origin": origin, "Referer": origin + "/"}
	var cfg struct {
		CompanyName string `json:"companyName"`
	}
	if err := c.GetJSONWithHeaders(ctx, "https://api.recruitify.ai/apply/config", headers, &cfg); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	var page []struct {
		GUID string `json:"guid"`
	}
	if err := c.GetJSONWithHeaders(ctx, "https://api.recruitify.ai/apply/project?page=1", headers, &page); err != nil {
		return "", 0, err
	}
	return strings.TrimSpace(cfg.CompanyName), len(page), nil
}

// maxRequestsPerSecond holds a harvest to the crawl's own pace (recruitifyRequestInterval): one
// API host answers every portal.
func (recruitifyProber) maxRequestsPerSecond() float64 { return 2 }

// discover enumerates candidate portals from Common Crawl's index of *.recruitify.ai.
func (recruitifyProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.recruitify.ai", recognizedBoard("recruitify"))
}
