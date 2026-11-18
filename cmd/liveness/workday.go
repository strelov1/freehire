package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// workdayPass reads a *.myworkdayjobs.com posting through Workday's CXS job endpoint, the one the
// workday adapter reads details from. The posting page itself is a JS shell that liveness.Classify
// reads as insufficient_content whether the job is live or not, so it is never fetched. CXS answers
// 200 for a live posting and 403 with errorCode S22 for a removed one; any other 403 or a 429 is
// Workday metering the host and ends the pass.
func workdayPass(client *http.Client) pacedPass {
	return pacedPass{
		source:    "workday",
		interval:  time.Second,
		maxPerRun: 500,
		check: func(ctx context.Context, c candidate) pacedVerdict {
			unknown := pacedVerdict{verdict: liveness.Uncertain}
			api := workdayJobAPI(c.url)
			if api == "" {
				return unknown
			}
			ctx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
			if err != nil {
				return unknown
			}
			req.Header.Set("User-Agent", liveness.UserAgent)
			req.Header.Set("Accept", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				return unknown
			}
			defer resp.Body.Close()
			if resp.Request.URL.String() != req.URL.String() { // a redirect answers for some other resource
				return unknown
			}
			switch resp.StatusCode {
			case http.StatusOK:
				return pacedVerdict{verdict: liveness.Live}
			case http.StatusTooManyRequests:
				unknown.refused = true
			case http.StatusForbidden:
				var body struct {
					ErrorCode string `json:"errorCode"`
				}
				if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body) == nil && body.ErrorCode == "S22" {
					return pacedVerdict{verdict: liveness.Expired, reason: "workday_posting_removed"}
				}
				unknown.refused = true
			}
			return unknown
		},
	}
}

// workdayJobAPI maps a posting URL "https://<tenant>.wdN.myworkdayjobs.com[/<locale>]/<site>/job/..."
// to its CXS endpoint "https://<host>/wday/cxs/<tenant>/<site>/job/...", or "" for any other URL.
func workdayJobAPI(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Port() != "" || u.User != nil || !strings.HasSuffix(u.Host, ".myworkdayjobs.com") {
		return ""
	}
	segs := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	for i := 1; i <= 2 && i+1 < len(segs); i++ {
		if segs[i] == "job" {
			tenant, _, _ := strings.Cut(u.Host, ".")
			return "https://" + u.Host + "/wday/cxs/" + tenant + "/" + strings.Join(segs[i-1:], "/")
		}
	}
	return ""
}
