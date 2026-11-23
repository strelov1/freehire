package main

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
)

func TestDiceLivenessRequiresPostingEvidence(t *testing.T) {
	if !slices.Contains(probeDespiteRegistered, "dice") {
		t.Fatal("stale Dice postings are excluded from liveness")
	}

	const id = "99ccfb87-b404-47de-90e6-bae6eebc958b"
	const postingURL = "https://www.dice.com/job-detail/" + id
	const posting = `<script type="application/ld+json">{"@type":"JobPosting","description":"This job is no longer available in Canada."}</script>`
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		redirect string
		want     liveness.Verdict
		refused  bool
	}{
		{name: "gone", status: 410, body: posting, want: liveness.Expired},
		{name: "posting", status: 200, body: posting, want: liveness.Live},
		{name: "not found page", status: 200, body: "<title>Job Not Found | Dice.com</title>", want: liveness.Uncertain},
		{name: "not found status", status: 404, want: liveness.Uncertain},
		{name: "forbidden", status: 403, want: liveness.Uncertain, refused: true},
		{name: "rate limited", status: 429, want: liveness.Uncertain, refused: true},
		{name: "server error", status: 503, want: liveness.Uncertain},
		{name: "timeout", want: liveness.Uncertain},
		{name: "gone after redirect", status: 410, redirect: "/jobs", want: liveness.Uncertain},
		{name: "posting after redirect", status: 200, body: posting, redirect: "/job-detail/another", want: liveness.Uncertain},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: catalogStatusTransport(func(r *http.Request) (*http.Response, error) {
				if tc.status == 0 {
					return nil, context.DeadlineExceeded
				}
				resp := &http.Response{
					StatusCode: tc.status, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(tc.body)), Request: r,
				}
				if tc.redirect != "" && r.URL.String() == postingURL {
					resp.StatusCode = http.StatusFound
					resp.Header.Set("Location", tc.redirect)
				} else if r.URL.String() != postingURL && r.URL.Path != tc.redirect {
					t.Errorf("unexpected request URL %s", r.URL)
				}
				return resp, nil
			})}
			passes := pacedPasses(client)
			i := slices.IndexFunc(passes, func(p pacedPass) bool { return p.source == "dice" })
			if i < 0 {
				t.Fatal("Dice has no paced liveness pass")
			}
			pass := passes[i]
			if pass.interval != time.Second || pass.maxPerRun <= 0 {
				t.Fatalf("Dice pace or budget is missing: %+v", pass)
			}
			got := pass.check(t.Context(), candidate{externalID: ":" + id})
			if got.verdict != tc.want || got.refused != tc.refused {
				t.Fatalf("got %+v, want %v, refused %v", got, tc.want, tc.refused)
			}
			if tc.want == liveness.Expired && got.reason != "http_gone" {
				t.Fatalf("got close reason %q, want http_gone", got.reason)
			}
		})
	}
}
