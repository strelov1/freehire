package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// djinni's listing names only the postings of roughly the last month, so an older posting is never listed again and the company-scoped sweep cannot close it once its company drops out of the crawled set. This is that leak's evidence path.
//
// The stored url is djinni's own page, and the shared plain-GET probe reads it correctly: a removed posting answers 200 with the alert "The job ad is no longer active" (a liveness.Classify closed-posting phrase) or a plain 404.
//
// It cannot share the fan-out: djinni answers a fast burst from a datacenter IP with 403, and the crawl reads its listing from the same host. So the pass goes one page a second and stops at the first 403 or 429 (see applyPaced).

const (
	// djinniProbeInterval spaces djinni probes; one a second drew no refusal (the crawl pages at 600ms).
	djinniProbeInterval = time.Second
	// djinniMaxProbesPerRun bounds one run to ~25 minutes.
	djinniMaxProbesPerRun = 1500
)

// djinniPass is djinni's paced pass: the shared Fetch + Classify, one page at a time.
func djinniPass(client *http.Client) pacedPass {
	return pacedPass{
		source:    "djinni",
		interval:  djinniProbeInterval,
		maxPerRun: djinniMaxProbesPerRun,
		check: func(ctx context.Context, c candidate) pacedVerdict {
			return checkDjinniLive(ctx, client, c)
		},
	}
}

// checkDjinniLive classifies one djinni page. A 403 or 429 is djinni metering the host, not a
// statement about the posting, and ends the pass.
func checkDjinniLive(ctx context.Context, client *http.Client, c candidate) pacedVerdict {
	status, finalURL, body, err := liveness.Fetch(ctx, client, c.url)
	if err != nil {
		log.Printf("liveness: probe %s failed: %v", c.publicSlug, err)
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	if status == http.StatusForbidden || status == http.StatusTooManyRequests {
		return pacedVerdict{verdict: liveness.Uncertain, refused: true}
	}
	verdict, reason := liveness.Classify(status, finalURL, body)
	return pacedVerdict{verdict: verdict, reason: reason}
}
