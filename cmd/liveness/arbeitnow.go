package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// arbeitnow's job-board API names only the postings of the last week, so an older posting is never listed again and the company-scoped sweep cannot close it once its company drops out of the week's postings. This is that leak's evidence path.
//
// The stored url is arbeitnow's own page, and the site states a removal in the status (410 gone, 200 live). The verdict is therefore read from the status ALONE, not through liveness.Classify's body heuristics: a live page carries the employer's whole description, which can match a closed-posting phrase and read as expired.
//
// The crawl reads the API from the same host, which answers a burst with 429, so the pass goes one page a second and stops at the first 403 or 429 (see applyPaced).

const (
	// arbeitnowProbeInterval spaces arbeitnow probes; one a second drew no refusal.
	arbeitnowProbeInterval = time.Second
	// arbeitnowMaxProbesPerRun bounds one run to ~40 minutes, enough to work off the dead backlog in a few days at two runs a day.
	arbeitnowMaxProbesPerRun = 2500
)

// arbeitnowPass is arbeitnow's paced pass, one page at a time.
func arbeitnowPass(client *http.Client) pacedPass {
	return pacedPass{
		source:    "arbeitnow",
		interval:  arbeitnowProbeInterval,
		maxPerRun: arbeitnowMaxProbesPerRun,
		check: func(ctx context.Context, c candidate) pacedVerdict {
			return checkArbeitnowLive(ctx, client, c)
		},
	}
}

// checkArbeitnowLive judges one arbeitnow page by its status: 404/410 is gone, a 2xx is live, a
// 403 or 429 is the site metering the host and ends the pass, anything else says nothing. The
// body is never read: the status is the whole answer, and a body cut short must not turn a
// refusal into a failed probe that lets the pass carry on.
func checkArbeitnowLive(ctx context.Context, client *http.Client, c candidate) pacedVerdict {
	status, err := fetchStatus(ctx, client, c.url)
	switch {
	case err != nil:
		log.Printf("liveness: probe %s failed: %v", c.publicSlug, err)
		return pacedVerdict{verdict: liveness.Uncertain}
	case status == http.StatusNotFound || status == http.StatusGone:
		return pacedVerdict{verdict: liveness.Expired, reason: "http_gone"}
	case status == http.StatusForbidden || status == http.StatusTooManyRequests:
		return pacedVerdict{verdict: liveness.Uncertain, refused: true}
	case status >= 200 && status < 300:
		return pacedVerdict{verdict: liveness.Live}
	default:
		return pacedVerdict{verdict: liveness.Uncertain}
	}
}

// fetchStatus GETs url and returns the final status, following redirects, without reading
// the body.
func fetchStatus(ctx context.Context, client *http.Client, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", liveness.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}
