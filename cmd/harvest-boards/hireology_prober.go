package main

import (
	"context"
	"fmt"
	"strings"
)

// hireologyProber probes a careers.hireology.com tenant (slug = board) via the public
// api.hireology.com/v1/careers/<slug> JSON:API; a non-zero count of Open postings means a
// live board. The employer name comes from the seed.
type hireologyProber struct{}

func (hireologyProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Data []struct {
			Attributes struct {
				Status string `json:"status"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://api.hireology.com/v1/careers/%s", slug), &resp); err != nil {
		return "", 0, nil
	}
	open := 0
	for _, d := range resp.Data {
		if strings.EqualFold(d.Attributes.Status, "Open") {
			open++
		}
	}
	return "", open, nil
}
