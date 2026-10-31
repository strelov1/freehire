package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// jobtech (Arbetsförmedlingen) closes its own postings from JobStream's change feed: each run
// reads the last 72h of changes and closes every ad the feed reports removed. A removal the
// crawl fails to apply inside those 72h is never sent again, and nothing else closes the ad —
// the source is self-closing, so the unseen sweep skips it, and a live ad that does not change
// is not re-sent either, so going unseen says nothing. From 2026-10-27 the crawl applied no
// removal at all (its read of the feed ended before them; see sources/jobtech.go), and on
// 2026-10-31 32.7k of its 46.5k open rows had gone unseen for a week. This pass is the backstop
// for a removal the feed carried and the crawl missed.
//
// The evidence is JobSearch's per-ad endpoint, the same service's API keyed on the ad id: it
// answers a live ad with `"removed": false`, a recent removal with `"removed": true`, and an ad
// removed long enough ago with 404. The stored url is Platsbanken's page, a client-rendered app
// that answers 200 for any id. Of 200 stale rows sampled on 2026-10-31, 53 answered 404 and the
// rest 200; the removed ones among those were removals of the last two days.

// jobtechAdAPI is JobSearch's per-ad endpoint. Keyless (open government data).
const jobtechAdAPI = "https://jobsearch.api.jobtechdev.se/ad/%s"

const (
	// jobtechProbeInterval spaces jobtech probes. The API answered 40 back-to-back requests
	// from the crawl host in ~115 ms each without a refusal (2026-10-31); this keeps the pass
	// near three a second.
	jobtechProbeInterval = 200 * time.Millisecond
	// jobtechMaxProbesPerRun bounds one run to ~45 minutes. ~41k open rows were unseen past the
	// sweep window when this landed, ~11k of them dead; at two runs a day and two reads per
	// close, the dead backlog is worked off in about four days.
	jobtechMaxProbesPerRun = 8000
	// jobtechReadLimit bounds how much of an answer is read. An ad is a few KB of JSON.
	jobtechReadLimit = 1 << 20
)

// jobtechPass is jobtech's paced pass over apiTemplate (jobtechAdAPI, or a stub in a test).
func jobtechPass(client *http.Client, apiTemplate string) pacedPass {
	return pacedPass{
		source:    "jobtech",
		interval:  jobtechProbeInterval,
		maxPerRun: jobtechMaxProbesPerRun,
		check: func(ctx context.Context, c candidate) pacedVerdict {
			id := strings.TrimPrefix(c.externalID, ":")
			if !isDigits(id) {
				log.Printf("liveness: jobtech %s has no ad id in external_id %q — skipping", c.publicSlug, c.externalID)
				return pacedVerdict{verdict: liveness.Uncertain}
			}
			return checkJobtechLiveAt(ctx, client, apiTemplate, id)
		},
	}
}

// checkJobtechLiveAt asks JobSearch about one ad: 404/410 or `"removed": true` is Expired,
// `"removed": false` is Live, a 403/429 ends the pass, and anything else says nothing.
func checkJobtechLiveAt(ctx context.Context, client *http.Client, apiTemplate, id string) pacedVerdict {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(apiTemplate, id), nil)
	if err != nil {
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	req.Header.Set("User-Agent", liveness.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("liveness: jobtech probe %s failed: %v", id, err)
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return pacedVerdict{verdict: liveness.Expired, reason: "jobtech_ad_gone"}
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return pacedVerdict{verdict: liveness.Uncertain, refused: true}
	case resp.StatusCode != http.StatusOK:
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	var ad struct {
		Removed *bool `json:"removed"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, jobtechReadLimit)).Decode(&ad); err != nil || ad.Removed == nil {
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	if *ad.Removed {
		return pacedVerdict{verdict: liveness.Expired, reason: "jobtech_ad_removed"}
	}
	return pacedVerdict{verdict: liveness.Live}
}

// isDigits reports whether s is a non-empty run of ASCII digits — a JobTech ad id.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
