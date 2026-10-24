package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/ingest/sources"
)

// polymerProber validates a Polymer board (the <board> of jobs.polymer.co/<board>) with one
// request: the first page of the public listing API states the board's posting count and the
// organisation's own name, where the adapter fallback would read every posting's body to count
// a board. It mirrors the URL of internal/ingest/sources/polymer.go, which is unexported.
type polymerProber struct{}

// polymerBoardPattern is the slug shape the adapter accepts; anything else would build an API
// path the platform never serves.
var polymerBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (polymerProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !polymerBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	var page struct {
		Meta struct {
			Count            *int   `json:"count"`
			OrganizationName string `json:"organization_name"`
		} `json:"meta"`
	}
	if err := c.GetJSON(ctx, "https://api.polymer.co/v1/hire/organizations/"+board+"/jobs?page=1", &page); err != nil {
		// The platform answers an unknown or disabled board with 422 ("no careers_page found").
		// Anything else is a probe that did not happen, and is surfaced.
		var se *sources.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusUnprocessableEntity || se.Code == http.StatusNotFound) {
			return "", 0, nil
		}
		return "", 0, err
	}
	// A page without the count is a reshaped answer, not an empty board.
	if page.Meta.Count == nil {
		return "", 0, fmt.Errorf("polymer probe: %s: listing carries no count", board)
	}
	if *page.Meta.Count == 0 {
		return "", 0, nil
	}
	return strings.TrimSpace(page.Meta.OrganizationName), *page.Meta.Count, nil
}

// discover enumerates candidate boards from Common Crawl's index of jobs.polymer.co, the host
// every board and posting is served under. Recognition goes through atsboard, the same reading
// the contribution flow gives a pasted link.
func (polymerProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "jobs.polymer.co/*", recognizedBoard("polymer"))
}

// recognizedBoard slices a crawled URL to the board atsboard reads in it for provider, for a
// platform whose URL shape atsboard already knows.
func recognizedBoard(provider string) func(string) (string, bool) {
	return func(u string) (string, bool) {
		src, board, _, ok := atsboard.Recognize(strings.TrimSpace(u))
		if !ok || src != provider {
			return "", false
		}
		return board, true
	}
}
