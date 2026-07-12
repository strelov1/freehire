package main

import (
	"context"
	"fmt"
)

// pageupProber validates a PageUp board (a numeric institution id) by reading the open-job
// count off the canonical search endpoint. PageUp exposes no employer name (the board is an
// opaque instID), so it returns an empty name and leans on the seed-supplied company; the XHR
// header is required or the endpoint returns the HTML page instead of the JSON envelope.
type pageupProber struct{}

func (pageupProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	url := fmt.Sprintf("https://careers.pageuppeople.com/%s/cw/en/search/?page-items=1", board)
	var env struct {
		Count int `json:"count"`
	}
	if err := c.GetJSONWithHeaders(ctx, url, map[string]string{"X-Requested-With": "XMLHttpRequest"}, &env); err != nil {
		return "", 0, nil
	}
	return "", env.Count, nil
}
