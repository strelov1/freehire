package main

import (
	"context"
	"strings"
)

// phenomProber validates a Phenom career site (board = the site host) with one refineSearch
// request at page size 1, whose totalHits is the site's open-posting count. The adapter prober
// lists every page and fetches each posting's detail (minutes per board), which a count never
// needs. The response exposes no employer name, so the seed's company labels the board.
type phenomProber struct{}

func (phenomProber) dedupKey(board string) string { return strings.ToLower(board) }

func (phenomProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	host := strings.ToLower(strings.TrimSpace(board))
	if host == "" {
		return "", 0, nil
	}
	body := map[string]any{
		"deviceType": "desktop",
		"ddoKey":     "refineSearch",
		"jobs":       true,
		"from":       0,
		"size":       1,
	}
	var resp struct {
		RefineSearch struct {
			TotalHits int `json:"totalHits"`
			Data      struct {
				Jobs []struct {
					JobSeqNo string `json:"jobSeqNo"`
				} `json:"jobs"`
			} `json:"data"`
		} `json:"refineSearch"`
	}
	if err := c.PostJSON(ctx, "https://"+host+"/widgets", body, &resp); err != nil {
		// A site that is not a Phenom tenant (any more) answers 404; anything else is the
		// probe failing, not the board being empty.
		if boardGone(err) {
			return "", 0, nil
		}
		return "", 0, err
	}
	n := resp.RefineSearch.TotalHits
	if n == 0 {
		// totalHits is absent on some tenants; the page itself still says whether one exists.
		n = len(resp.RefineSearch.Data.Jobs)
	}
	return "", n, nil
}
