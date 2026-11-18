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

// jobtech (Arbetsförmedlingen) closes its own postings from JobStream's change feed: each run reads the last 72h of changes and closes every ad reported removed. A removal the crawl fails to apply inside those 72h is never sent again, and nothing else closes the ad: the source is self-closing, so the unseen sweep skips it, and a live unchanged ad is not re-sent. This pass is the backstop for a removal the feed carried and the crawl missed.
//
// The evidence is JobSearch's per-ad endpoint, keyed on the ad id: a live ad answers `"removed": false`, a recent removal `"removed": true`, and one removed long ago 404. The stored url is Platsbanken's client-rendered page, which answers 200 for any id.

// jobtechAdAPI is JobSearch's per-ad endpoint. Keyless (open government data).
const jobtechAdAPI = "https://jobsearch.api.jobtechdev.se/ad/%s"

const (
	// jobtechProbeInterval spaces jobtech probes at about three a second; the API took back-to-back requests without a refusal.
	jobtechProbeInterval = 200 * time.Millisecond
	// jobtechMaxProbesPerRun bounds one run to ~45 minutes.
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
