package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// arbeitnow's job-board API names only the postings of the last week: a full walk on
// 2026-10-30 read 21 pages, 4,282 distinct postings, none created before 2026-10-23, while the
// catalogue held 12.7k open arbeitnow rows. An older posting is never listed again however long
// it stays up, and the company-scoped sweep cannot close it once its company drops out of the
// week's postings — 88% of the open rows had gone unseen for more than a week. This is that
// leak's evidence path.
//
// The stored url is arbeitnow's own page (on .com, .co.uk, .nl, .ch, .fr), and the site states
// a removal in the status: measured over 100 postings unseen past the sweep window, 64 answered
// 410 and 36 answered 200 with their JobPosting block. The verdict is therefore read from the
// status ALONE, not through liveness.Classify's body heuristics: a live page carries the
// employer's whole description, and one live .ch posting's ("due to Brexit we are no longer
// accepting applications from the UK") matched a closed-posting phrase and read as expired.
//
// The crawl reads the API from the same host and the API answers a burst with 429, so the pass
// goes one page a second and stops at the first 403 or 429 (see applyPaced).

const (
	// arbeitnowProbeInterval spaces arbeitnow probes. 100 sequential page reads at one a
	// second drew no refusal (2026-10-30).
	arbeitnowProbeInterval = time.Second
	// arbeitnowMaxProbesPerRun bounds one run to ~40 minutes. ~11k arbeitnow postings were past
	// the sweep window when this landed, ~7k of them dead; at two runs a day, and two reads per
	// close, the backlog is worked off within about four days.
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
// 403 or 429 is the site metering the host and ends the pass, anything else says nothing.
func checkArbeitnowLive(ctx context.Context, client *http.Client, c candidate) pacedVerdict {
	status, _, _, err := liveness.Fetch(ctx, client, c.url)
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
