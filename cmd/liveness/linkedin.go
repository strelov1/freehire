package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/liveness"
)

// linkedInProxyTimeout bounds one read through the Scrapling service, its exit-IP retries included.
const linkedInProxyTimeout = 60 * time.Second

// LinkedIn's guest search is only a recent slice, so absence is never a removal. The job's own explicit closed notice is evidence even though it returns HTTP 200.
//
// proxied means client leaves through the Scrapling tier's rotating proxy, the crawl's own route.
// Direct, LinkedIn refuses the crawl host after a few requests (both 2026-11-21 runs stopped at
// probe 1 while ~17k stale postings, nearly all closed, queued), so the direct pass keeps its
// small budget and stops on a refusal. Through the proxy a refusal names one exit IP, so that
// posting is skipped and the pass goes on with a budget that outruns the queue's growth.
func linkedInPass(client *http.Client, proxied bool) pacedPass {
	maxPerRun, timeout := 300, probeTimeout
	if proxied {
		maxPerRun, timeout = 1500, linkedInProxyTimeout
	}
	return pacedPass{
		source:    "linkedin",
		interval:  2 * time.Second,
		maxPerRun: maxPerRun,
		check: func(ctx context.Context, c candidate) pacedVerdict {
			unknown := pacedVerdict{verdict: liveness.Uncertain}
			id := sources.LinkedInPostingID(c.url)
			if id == "" {
				return unknown
			}
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/"+id, nil)
			if err != nil {
				return unknown
			}
			req.Header.Set("User-Agent", liveness.UserAgent)
			req.Header.Set("Accept-Language", "en-US,en;q=0.9")
			resp, err := client.Do(req)
			if err != nil {
				return unknown
			}
			defer resp.Body.Close()
			if resp.StatusCode == 403 || resp.StatusCode == 429 || resp.StatusCode == 999 {
				unknown.refused = !proxied
				return unknown
			}
			// Unlike other providers, 404/410 is not established as authoritative; keep it uncertain until a larger sample supports treating it as gone.
			if resp.StatusCode != http.StatusOK {
				return unknown
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
			if err != nil || len(body) > 2<<20 {
				return unknown
			}
			root, err := html.Parse(bytes.NewReader(body))
			if err != nil {
				return unknown
			}
			job, err := sources.LinkedInPosting(root, resp.Request.URL.String(), id)
			if err != nil {
				return unknown
			}
			if job.Removed {
				return pacedVerdict{verdict: liveness.Expired, reason: "linkedin_post_closed"}
			}
			return pacedVerdict{verdict: liveness.Live}
		},
	}
}
