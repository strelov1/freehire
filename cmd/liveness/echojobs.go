package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/strelov1/freehire/internal/liveness"
)

// echojobsJobURL is echojobs.io's own per-posting detail page — already used live for
// description backfill (see internal/sources/echojobs.go). Unlike jobs.url for this source (the
// employer's own ATS link — Workday, Greenhouse, and others, with wildly variable per-company
// reliability across a ~330k-posting catalogue), this is the single site echojobs.io itself, and
// a removed posting answers with a plain 404 rather than whatever the employer's own ATS
// happens to do.
//
// echojobs.io retired its public JSON detail API (/api/jobs/%s, which used to answer a removed
// posting with a distinguishable {"error":"job fetch failed"} body) around 2026-08-13; this now
// hits the site's server-rendered job page instead, whose plain HTTP status is the liveness
// signal.
const echojobsJobURL = "https://echojobs.io/job/%s"

// checkEchoJobsLive queries echojobs.io's own job page for one posting's slug (its ExternalID,
// minus the boardless ":" prefix UpsertJob stores boardless external ids with — see
// internal/sources/source.go) and reports whether it is still listed.
func checkEchoJobsLive(ctx context.Context, client *http.Client, externalID string) (liveness.Verdict, string) {
	return checkEchoJobsLiveAt(ctx, client, echojobsJobURL, trimEchoJobsHandle(externalID))
}

// trimEchoJobsHandle strips the boardless ":" prefix UpsertJob stores boardless
// external ids with, recovering the slug echojobs.io's job page expects.
func trimEchoJobsHandle(externalID string) string {
	return strings.TrimPrefix(externalID, ":")
}

// checkEchoJobsLiveAt is checkEchoJobsLive with the job URL template as a parameter (already-
// stripped slug), so a test can point it at a stub server. The page's plain HTTP status is the
// liveness signal: 200 means still listed, 404 or 410 means removed (the same pair the shared
// liveness.Classify treats as "gone" — see internal/liveness/liveness.go); anything else is
// Uncertain, the same under-closing-biased default every other adapter's liveness check falls
// back to.
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
