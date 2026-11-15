package main

import (
	"context"
)

// gemProbeQuery is the minimal list query (extId only) the Gem prober uses for liveness — a
// trimmed sibling of the adapter's gemListQuery.
const gemProbeQuery = `query JobBoardList($boardId: String!) { oatsExternalJobPostings(boardId: $boardId) { jobPostings { extId } } }`

// gemProber probes the Gem public job-board GraphQL API. A non-empty jobPostings list is a
// live board; the query exposes no company name, so it falls back to the slug.
type gemProber struct{}

func (gemProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	body := map[string]any{
		"operationName": "JobBoardList",
		"query":         gemProbeQuery,
		"variables":     map[string]any{"boardId": slug},
	}
	var resp struct {
		Data struct {
			Postings struct {
				JobPostings []struct {
					ExtID string `json:"extId"`
				} `json:"jobPostings"`
			} `json:"oatsExternalJobPostings"`
		} `json:"data"`
	}
	if err := c.PostJSON(ctx, "https://jobs.gem.com/api/public/graphql", body, &resp); err != nil {
		return "", 0, nil
	}
	n := len(resp.Data.Postings.JobPostings)
	if n == 0 {
		return "", 0, nil
	}
	return "", n, nil
}
