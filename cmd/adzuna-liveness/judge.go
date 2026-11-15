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

// adzunaDetailsURL is Adzuna's own page for one posting. The stored jobs.url is usually the
// ad-network redirect, which answers "Access Denied" to non-browsers; /details/<id> answers
// honestly.
const adzunaDetailsURL = "https://www.adzuna.com/details/%s"

// maxBodyBytes bounds one page read; it only has to stop a runaway answer.
const maxBodyBytes = 4 << 20

// detailsURL builds the page to probe from a stored external_id, since jobs.url is the
// redirect.
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
// URL cannot be rebuilt from the id: Wellfound answers 404 for a LIVE posting asked for
// without its title slug, which would close every posting probed. So the stored url is
// accepted only when it names exactly the row's posting id; any other page's 404 would close a
// posting nobody asked about.
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

// postingID reads the posting id from a board-namespaced external_id ("it-jobs:5922558051"):
// the part after the last colon, which must be all digits. Anything else is a row this worker
// does not understand, and a URL built from it would read some other page.
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

// classify reads one answer from a source's own posting page, in three shapes:
//
//   - 404 or 410: the source says the posting is gone.
//   - 200 carrying the schema.org JobPosting block: the posting is live.
//   - anything else is not an answer. Adzuna answers 405 "Human Verification" about nine
//     requests in ten, so the caller asks again from another exit IP. A 200 without the
//     JobPosting block is not an answer either; it is the shape of a challenge or interstitial.
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

// probe asks the source about one posting up to attempts times, stopping at the first answer.
// Each request leaves through the Scrapling rotating proxy, so each attempt is a fresh exit IP
// past a challenge. A transport failure counts as an attempt with no answer and is never read
// as a verdict.
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
