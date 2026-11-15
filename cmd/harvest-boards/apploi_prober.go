package main

import (
	"context"
	"fmt"
	"regexp"
)

// apploiProber probes an apploi employer board (slug = numeric employer id) via the public
// api.apploi.com/v1/jobs?employer=<id> list; a non-zero count of live (published, non-archived)
// postings means a live board. The employer name comes from the seed.
type apploiProber struct{}

// apploiNumericSlugRE matches apploi's own employer-id shape: digits only. api.apploi.com
// silently ignores an `employer` value it can't parse and answers with a non-empty "live" jobs
// list, so a name-derived candidate (e.g. "101-edu") would read as a hit. Reject non-numeric
// slugs up front rather than trust the API to say no.
var apploiNumericSlugRE = regexp.MustCompile(`^\d+$`)

func (apploiProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	if !apploiNumericSlugRE.MatchString(slug) {
		return "", 0, nil
	}
	var resp struct {
		Data []struct {
			Published bool `json:"published"`
			Archived  bool `json:"archived"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://api.apploi.com/v1/jobs?employer=%s&limit=100", slug), &resp); err != nil {
		return "", 0, nil
	}
	live := 0
	for _, j := range resp.Data {
		if j.Published && !j.Archived {
			live++
		}
	}
	return "", live, nil
}
