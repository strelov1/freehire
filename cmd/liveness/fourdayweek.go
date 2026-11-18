package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/liveness"
)

// 4dayweek's list feed names its whole catalogue, but the adapter is not fullCatalog, so its sweep is company-scoped and the last posting of a company that leaves the feed stays open. This is that leak's evidence path.
//
// It reads the per-posting API keyed on the slug: /api/v2/jobs/<slug> answers 200 while 4dayweek has the posting and 404 once it is gone. That is the path robots.txt allows, and ~7 KB against the page's ~250 KB; the page offers no better signal. The external id (a UUID) is NOT a key this API accepts (it answers 404 for a live posting), so the slug comes from the stored url.
//
// The site sits behind Cloudflare, which refuses every self-identifying client (see sources.BrowserUserAgent) and meters by IP. The crawl reads the list from the same host, so the pass goes one request at a time and stops at the first 403 or 429 (see applyPaced).

// fourDayWeekJobAPI is the per-posting API, keyed by slug.
const fourDayWeekJobAPI = "https://4dayweek.io/api/v2/jobs/%s"

const (
	// fourDayWeekProbeInterval spaces 4dayweek probes. One API request a second drew no refusal; the job PAGE at that pace drew 429s, and concurrent pages drew them on most requests.
	fourDayWeekProbeInterval = time.Second
	// fourDayWeekMaxProbesPerRun bounds one run to 50 minutes. A row read Expired once is re-read first on the next run (see applyPaced).
	fourDayWeekMaxProbesPerRun = 3000
)

// fourDayWeekPass is 4dayweek's paced pass over apiTemplate (fourDayWeekJobAPI, or a stub in a
// test).
func fourDayWeekPass(client *http.Client, apiTemplate string) pacedPass {
	return pacedPass{
		source:    "4dayweek",
		interval:  fourDayWeekProbeInterval,
		maxPerRun: fourDayWeekMaxProbesPerRun,
		check: func(ctx context.Context, c candidate) pacedVerdict {
			slug, ok := fourDayWeekSlug(c.url)
			if !ok {
				log.Printf("liveness: 4dayweek %s has no job slug in url %q — skipping", c.publicSlug, c.url)
				return pacedVerdict{verdict: liveness.Uncertain}
			}
			return checkFourDayWeekLiveAt(ctx, client, apiTemplate, slug)
		},
	}
}

// fourDayWeekSlug recovers the posting's slug from its stored url, https://4dayweek.io/job/<slug>.
func fourDayWeekSlug(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host != "4dayweek.io" {
		return "", false
	}
	slug, ok := strings.CutPrefix(u.Path, "/job/")
	if !ok || slug == "" || strings.Contains(slug, "/") {
		return "", false
	}
	return slug, true
}

// checkFourDayWeekLiveAt asks the per-posting API about one slug: 200 is Live, 404 or 410 is
// Expired, a 403 (the Cloudflare challenge) or 429 ends the pass, and anything else is
// Uncertain.
func checkFourDayWeekLiveAt(ctx context.Context, client *http.Client, apiTemplate, slug string) pacedVerdict {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(apiTemplate, url.PathEscape(slug)), nil)
	if err != nil {
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	req.Header.Set("User-Agent", sources.BrowserUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	defer resp.Body.Close()
	// Drain so the transport can reuse the connection across a pass of many requests.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	switch resp.StatusCode {
	case http.StatusOK:
		return pacedVerdict{verdict: liveness.Live}
	case http.StatusNotFound, http.StatusGone:
		return pacedVerdict{verdict: liveness.Expired, reason: "4dayweek_job_gone"}
	case http.StatusForbidden, http.StatusTooManyRequests:
		return pacedVerdict{verdict: liveness.Uncertain, refused: true}
	default:
		return pacedVerdict{verdict: liveness.Uncertain}
	}
}
