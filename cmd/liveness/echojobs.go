package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// echojobsJobURL is echojobs.io's own per-posting detail page (also used for description backfill, see internal/ingest/sources/echojobs.go). Unlike jobs.url for this source (the employer's own ATS link, with wildly variable reliability), this is one site that answers a removed posting with a plain 404. The public JSON detail API was retired, so the server-rendered page's HTTP status is the liveness signal.
const echojobsJobURL = "https://echojobs.io/job/%s"

// checkEchoJobsLive queries echojobs.io's own job page for one posting's slug (its ExternalID,
// minus the boardless ":" prefix UpsertJob stores boardless external ids with — see
// internal/ingest/sources/source.go) and reports whether it is still listed.
func checkEchoJobsLive(ctx context.Context, client *http.Client, externalID string) (liveness.Verdict, string) {
	return checkEchoJobsLiveAt(ctx, client, echojobsJobURL, trimEchoJobsHandle(externalID))
}

// trimEchoJobsHandle strips the boardless ":" prefix UpsertJob stores boardless
// external ids with, recovering the slug echojobs.io's job page expects.
func trimEchoJobsHandle(externalID string) string {
	return strings.TrimPrefix(externalID, ":")
}

// checkEchoJobsLiveAt is checkEchoJobsLive with the job URL template as a parameter (already-stripped slug), so a test can point it at a stub server. 200 means still listed, 404 or 410 means removed (as in liveness.Classify); anything else is Uncertain, the under-closing-biased default.
func checkEchoJobsLiveAt(ctx context.Context, client *http.Client, jobURLTemplate, slug string) (liveness.Verdict, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(jobURLTemplate, slug), nil)
	if err != nil {
		return liveness.Uncertain, ""
	}
	req.Header.Set("User-Agent", liveness.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return liveness.Uncertain, ""
	}
	defer resp.Body.Close()
	// Drain so the transport can reuse the connection across a run that probes many postings.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	switch resp.StatusCode {
	case http.StatusOK:
		return liveness.Live, ""
	case http.StatusNotFound, http.StatusGone:
		return liveness.Expired, "echojobs_job_gone"
	default:
		return liveness.Uncertain, ""
	}
}
