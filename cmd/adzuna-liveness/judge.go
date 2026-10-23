package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// adzunaDetailsURL is Adzuna's own hosted page for one posting. The stored jobs.url is usually
// the ad-network redirect (/land/ad/<id>), which answers a branded "Access Denied" to anything
// but a browser; /details/<id> is the same posting on Adzuna's own site and answers honestly.
const adzunaDetailsURL = "https://www.adzuna.com/details/%s"

// maxBodyBytes bounds one page read. A posting page measured ~50 KB; the cap only has to stop a
// runaway answer, not fit a typical one.
const maxBodyBytes = 4 << 20

// detailsURL builds the page to probe from a stored external_id. Adzuna's ids are namespaced by
// board ("it-jobs:5922558051"); the posting id is the part after the last colon, and it must be
// all digits — anything else is a row this worker does not understand, and probing a URL built
// from it would read some other page.
func detailsURL(externalID string) (string, bool) {
	id := externalID[strings.LastIndex(externalID, ":")+1:]
	if id == "" {
		return "", false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return fmt.Sprintf(adzunaDetailsURL, id), true
}

// classify reads one answer from Adzuna's hosted job page. Measured 2026-10-23 through the
// Scrapling tier's rotating proxy, the page answers in exactly three shapes:
//
//   - 405 "Human Verification" — a challenge, most of the time (~9 in 10 requests). It says
//     nothing about the posting, so the caller asks again from another exit IP.
//   - 404 or 410 "Page not found" / "no longer available" — Adzuna stating the posting is gone.
//     Of 20 stale postings sampled, every one that got past the challenge answered this.
//   - 200 carrying the posting's schema.org JobPosting block — the posting is live.
//
// Anything else is not an answer — a 200 WITHOUT the JobPosting block included, since that is
// the shape a challenge variant or an interstitial would take.
func classify(status int, body []byte) (liveness.Verdict, bool) {
	switch {
	case status == http.StatusNotFound || status == http.StatusGone:
		return liveness.Expired, true
	case status == http.StatusOK && bytes.Contains(body, []byte("JobPosting")):
		return liveness.Live, true
	default:
		return liveness.Uncertain, false
	}
}

// probe asks Adzuna about one posting up to attempts times, stopping at the first answer. Each
// request leaves the Scrapling service through its rotating proxy, so each attempt is a new
// exit IP and a fresh chance past the challenge. A transport failure counts as an attempt that
// got no answer: the service being briefly unreachable must not end the probe of a posting on
// the first try, and must never be read as a verdict.
func probe(ctx context.Context, client *http.Client, url string, attempts int) (liveness.Verdict, int, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			return liveness.Uncertain, i, ctx.Err()
		}
		status, body, err := get(ctx, client, url)
		if err != nil {
			lastErr = err
			continue
		}
		if verdict, answered := classify(status, body); answered {
			return verdict, i + 1, nil
		}
	}
	return liveness.Uncertain, attempts, lastErr
}

func get(ctx context.Context, client *http.Client, url string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}
