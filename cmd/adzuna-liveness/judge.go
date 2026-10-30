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

// detailsURL builds Adzuna's page to probe from a stored external_id. Adzuna's stored jobs.url
// is the ad-network redirect, so the page is rebuilt from the posting id instead.
func detailsURL(externalID string) (string, bool) {
	id, ok := postingID(externalID)
	if !ok {
		return "", false
	}
	return fmt.Sprintf(adzunaDetailsURL, id), true
}

// wellfoundJobsPrefix is where every Wellfound posting page lives: /jobs/<id>-<title slug>.
const wellfoundJobsPrefix = "https://wellfound.com/jobs/"

// wellfoundPageURL returns the stored posting page when it is provably this row's page. The
// URL cannot be rebuilt from the id the way Adzuna's can: Wellfound answers 404 for a LIVE
// posting asked for without its title slug (/jobs/4689896, measured 2026-10-30), so a page
// built from the id alone would close every posting it probed. The stored url carries the
// slug, and it is accepted only when it names exactly the row's posting id — any other page's
// 404 would close a posting nobody asked about.
func wellfoundPageURL(externalID, url string) (string, bool) {
	id, ok := postingID(externalID)
	if !ok {
		return "", false
	}
	slug, found := strings.CutPrefix(url, wellfoundJobsPrefix+id+"-")
	if !found || slug == "" || strings.ContainsAny(slug, "/?#") {
		return "", false
	}
	return url, true
}

// postingID reads the posting id from a board-namespaced external_id ("it-jobs:5922558051",
// "software-engineer:4465590"): the part after the last colon, which must be all digits —
// anything else is a row this worker does not understand, and probing a URL built from it
// would read some other page.
func postingID(externalID string) (string, bool) {
	id := externalID[strings.LastIndex(externalID, ":")+1:]
	if id == "" {
		return "", false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return id, true
}

// classify reads one answer from a source's own posting page. Both sources answer in the same
// three shapes, measured through the Scrapling tier's rotating proxy:
//
//   - 404 or 410 — the source stating the posting is gone. Adzuna (2026-10-23): every stale
//     posting that got past the challenge answered this. Wellfound (2026-10-30): 20 of 20
//     postings stranded since September answered 410 "Page no longer available".
//   - 200 carrying the posting's schema.org JobPosting block — the posting is live.
//   - anything else, which is not an answer. Adzuna answers 405 "Human Verification" about nine
//     requests in ten, so the caller asks again from another exit IP. A 200 WITHOUT the
//     JobPosting block is not an answer either, since that is the shape a challenge variant or
//     an interstitial would take.
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

// probe asks the source about one posting up to attempts times, stopping at the first answer. Each
// request leaves the Scrapling service through its rotating proxy, so each attempt is a new
// exit IP and a fresh chance past a challenge. A transport failure counts as an attempt that
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
