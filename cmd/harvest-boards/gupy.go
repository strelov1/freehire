package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
)

// gupyFeedURL is Gupy's public portal jobs API (mirrors sources.gupyBaseURL, which is
// unexported; this tool lives outside the sources package). Keyed by companyId it lists one
// company's jobs; unkeyed it is a global feed across all employers (see discover).
const gupyFeedURL = "https://employability-portal.gupy.io/api/v1/jobs"

const (
	// gupyPageSize is the discovery page size for the global feed.
	gupyPageSize = 100
	// gupyMaxOffset bounds the discovery sweep so a feed that never empties cannot loop
	// forever; it sits well above Gupy's live-job count, and the empty-page check ends the
	// sweep sooner in practice.
	gupyMaxOffset = 100000
)

// gupyProber probes and discovers Gupy boards. A Gupy board is a numeric companyId; unlike
// the ATS providers there is no public seed list of ids, so the prober also discovers its
// own candidates from the global feed (see discover).
type gupyProber struct{}

// gupyResponse is the portal feed envelope: a page of postings plus the pagination total.
type gupyResponse struct {
	Data []struct {
		CompanyID      int64  `json:"companyId"`
		CareerPageName string `json:"careerPageName"`
	} `json:"data"`
	Pagination struct {
		Total int `json:"total"`
	} `json:"pagination"`
}

// probe queries one company's feed for its open-job count and career-page name. A missing
// company (getter error) or one with no open jobs yields ("", 0, nil) — a skip — and the
// name falls back to the companyId when the feed reports none.
func (gupyProber) probe(ctx context.Context, c httpClient, companyID string) (string, int, error) {
	var resp gupyResponse
	if err := c.GetJSON(ctx, fmt.Sprintf("%s?companyId=%s&limit=1", gupyFeedURL, companyID), &resp); err != nil {
		return "", 0, nil
	}
	// Liveness is "the feed returned a posting", like every other prober — not the
	// pagination total, which the adapter documents as unreliable when limit==page size.
	if len(resp.Data) == 0 {
		return "", 0, nil
	}
	name := companyID
	if len(resp.Data) > 0 && resp.Data[0].CareerPageName != "" {
		name = resp.Data[0].CareerPageName
	}
	return name, resp.Pagination.Total, nil
}

// discover pages the global feed (across all companies, no job-category filter) collecting
// each posting's distinct companyId in first-seen order, until a page returns no postings or
// the offset reaches gupyMaxOffset. A page that fails to fetch ends the sweep with the ids
// gathered so far, so one bad page truncates rather than aborts the harvest.
func (gupyProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	var ids []string
	seen := make(map[int64]bool)
	for offset := 0; offset < gupyMaxOffset; offset += gupyPageSize {
		var resp gupyResponse
		url := fmt.Sprintf("%s?limit=%d&offset=%d", gupyFeedURL, gupyPageSize, offset)
		if err := c.GetJSON(ctx, url, &resp); err != nil {
			// A page failing mid-sweep truncates rather than aborts; log so a short harvest
			// is not mistaken for an exhausted feed.
			log.Printf("gupy discover: offset %d: %v (returning %d ids so far)", offset, err, len(ids))
			break
		}
		if len(resp.Data) == 0 {
			break
		}
		for _, j := range resp.Data {
			if j.CompanyID != 0 && !seen[j.CompanyID] {
				seen[j.CompanyID] = true
				ids = append(ids, strconv.FormatInt(j.CompanyID, 10))
			}
		}
	}
	return ids, nil
}
