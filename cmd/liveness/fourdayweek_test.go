package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/liveness"
)

func TestCheckFourDayWeekLive(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		wantVerdict liveness.Verdict
		wantReason  string
		wantRefused bool
	}{
		{name: "listed posting", status: http.StatusOK, wantVerdict: liveness.Live},
		{name: "removed posting", status: http.StatusNotFound, wantVerdict: liveness.Expired, wantReason: "4dayweek_job_gone"},
		{name: "gone posting", status: http.StatusGone, wantVerdict: liveness.Expired, wantReason: "4dayweek_job_gone"},
		// Cloudflare's challenge and its rate limit: the edge declining, not a verdict.
		{name: "challenged", status: http.StatusForbidden, wantVerdict: liveness.Uncertain, wantRefused: true},
		{name: "rate limited", status: http.StatusTooManyRequests, wantVerdict: liveness.Uncertain, wantRefused: true},
		{name: "server error", status: http.StatusBadGateway, wantVerdict: liveness.Uncertain},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var path, ua string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path, ua = r.URL.Path, r.UserAgent()
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(srv.Close)
			got := checkFourDayWeekLiveAt(context.Background(), srv.Client(), srv.URL+"/api/v2/jobs/%s", "data-scientist-at-amgen-e48390c3")
			if got.verdict != tc.wantVerdict || got.reason != tc.wantReason || got.refused != tc.wantRefused {
				t.Errorf("got %+v, want verdict=%v reason=%q refused=%v", got, tc.wantVerdict, tc.wantReason, tc.wantRefused)
			}
			if path != "/api/v2/jobs/data-scientist-at-amgen-e48390c3" {
				t.Errorf("requested %q, want the slug's API path", path)
			}
			// Cloudflare answers every self-identifying client 403, which would end the pass on
			// its first request.
			if ua != sources.BrowserUserAgent {
				t.Errorf("User-Agent = %q, want the browser string", ua)
			}
		})
	}
}

func TestFourDayWeekSlug(t *testing.T) {
	ok := map[string]string{
		"https://4dayweek.io/job/data-scientist-at-amgen-e48390c3": "data-scientist-at-amgen-e48390c3",
	}
	for in, want := range ok {
		if got, found := fourDayWeekSlug(in); !found || got != want {
			t.Errorf("fourDayWeekSlug(%q) = %q, %v; want %q", in, got, found, want)
		}
	}
	for _, in := range []string{
		"",
		"https://4dayweek.io/job/",
		"https://4dayweek.io/jobs/data-scientist",
		"https://4dayweek.io/job/a/b",
		"https://example.com/job/data-scientist",
	} {
		if got, found := fourDayWeekSlug(in); found {
			t.Errorf("fourDayWeekSlug(%q) = %q; want rejected", in, got)
		}
	}
}
