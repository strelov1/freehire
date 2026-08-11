package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/strelov1/freehire/internal/liveness"
)

// echojobsDetailURL is echojobs.io's own per-posting detail API — already used live for
// description backfill (see internal/sources/echojobs.go's detail helper). Unlike jobs.url
// for this source (the employer's own ATS link — Workday, Greenhouse, and others, with
// wildly variable per-company reliability across a ~330k-posting catalogue), this is the
// single site echojobs.io itself, and a removed posting answers with a distinguishable
// error rather than whatever the employer's own ATS happens to do.
const echojobsDetailURL = "https://echojobs.io/api/jobs/%s"

// echojobsErrorBody is the body a removed posting's detail request returns — verified
// live: {"error":"job fetch failed"}, HTTP 500. A live posting returns 200 with the full
// detail payload (id/title/description/…) instead, which this type does not need to read.
type echojobsErrorBody struct {
	Error string `json:"error"`
}

// echojobsGoneError is the exact error text a removed posting's detail request returns.
// Matched exactly (not "any non-empty error field") so an unrelated API hiccup or rate
// limit — a different message, or the same 500 without this body — reads as Uncertain
// rather than a false death signal; under-closing is the safe failure mode here, same as
// everywhere else in this worker.
const echojobsGoneError = "job fetch failed"

// checkEchoJobsLive queries echojobs.io's detail API for one posting's job_handle (its
// ExternalID, minus the boardless ":" prefix UpsertJob stores boardless external ids
// with — see internal/sources/source.go) and reports whether it is still listed.
func checkEchoJobsLive(ctx context.Context, client *http.Client, externalID string) (liveness.Verdict, string) {
	return checkEchoJobsLiveAt(ctx, client, echojobsDetailURL, trimEchoJobsHandle(externalID))
}

// trimEchoJobsHandle strips the boardless ":" prefix UpsertJob stores boardless
// external ids with, recovering the job_handle echojobs' own detail API expects.
func trimEchoJobsHandle(externalID string) string {
	return strings.TrimPrefix(externalID, ":")
}

// checkEchoJobsLiveAt is checkEchoJobsLive with the detail URL template as a parameter
// (already-stripped handle), so a test can point it at a stub server.
func checkEchoJobsLiveAt(ctx context.Context, client *http.Client, detailURLTemplate, handle string) (liveness.Verdict, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(detailURLTemplate, handle), nil)
	if err != nil {
		return liveness.Uncertain, ""
	}
	req.Header.Set("User-Agent", liveness.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return liveness.Uncertain, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return liveness.Live, ""
	}
	var body echojobsErrorBody
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body)
	if body.Error == echojobsGoneError {
		return liveness.Expired, "echojobs_detail_gone"
	}
	return liveness.Uncertain, ""
}
