package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// kenjoProber validates a Kenjo career site (the <board> of ict-ds.kenjo.io) by reading the
// listing the adapter reads, which names the company and carries every open position. It
// mirrors internal/ingest/sources/kenjo.go, which is unexported. Kenjo serves every label, so an
// unknown one is a 404 from the API, never a DNS failure.
type kenjoProber struct{}

var kenjoBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (kenjoProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !kenjoBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	var site struct {
		CompanyName     string      `json:"companyName"`
		ActivePositions *[]struct{} `json:"activePositions"`
	}
	u := "https://" + board + ".kenjo.io/api/controller/career-site/public/" + board + "/positions"
	if err := c.GetJSON(ctx, u, &site); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	// An answer without the key is a reshaped one, not an empty site.
	if site.ActivePositions == nil {
		return "", 0, fmt.Errorf("kenjo probe: %s: answer carries no activePositions", board)
	}
	return strings.TrimSpace(site.CompanyName), len(*site.ActivePositions), nil
}

// discover enumerates candidate sites from Common Crawl's index of *.kenjo.io.
func (kenjoProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.kenjo.io", recognizedBoard("kenjo"))
}
