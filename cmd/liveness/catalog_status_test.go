package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/strelov1/freehire/internal/job/liveness"
)

func TestCatalogStatusProbeRequiresThePostingURL(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		redirect string
		want     liveness.Verdict
		refused  bool
	}{
		{name: "removed", status: 404, want: liveness.Expired},
		{name: "gone", status: 410, want: liveness.Expired},
		{name: "live", status: 200, want: liveness.Live},
		{name: "server failure", status: 503, want: liveness.Uncertain},
		{name: "blocked", status: 403, want: liveness.Uncertain, refused: true},
		{name: "rate limited", status: 429, want: liveness.Uncertain, refused: true},
		{name: "redirect to missing listing", status: 404, redirect: "/jobs", want: liveness.Uncertain},
		{name: "redirect to live listing", status: 200, redirect: "/jobs", want: liveness.Uncertain},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.redirect != "" && r.URL.Path != "/jobs" {
					http.Redirect(w, r, tt.redirect, http.StatusFound)
					return
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte("This job is no longer accepting applications from the UK."))
			}))
			defer srv.Close()
			got := checkCatalogStatus(context.Background(), srv.Client(), candidate{url: srv.URL + "/jobs/123"})
			if got.verdict != tt.want || got.refused != tt.refused {
				t.Errorf("got %+v, want verdict %v refused %v", got, tt.want, tt.refused)
			}
		})
	}
}

func TestCatalogStatusPassRejectsUnrelatedStoredURLs(t *testing.T) {
	for _, pass := range catalogStatusPasses(nil) {
		for _, u := range []string{"https://employer.example/jobs/123", "https://remote.com/jobs/", "http://jobs.instaffo.com/de/job/123"} {
			got := pass.check(context.Background(), candidate{url: u})
			if got.verdict != liveness.Uncertain {
				t.Errorf("%s: verdict %v for %s", pass.source, got.verdict, u)
			}
		}
	}
}

func TestCatalogStatusProbeRefusesCrossHostRedirect(t *testing.T) {
	destination := httptest.NewServer(http.NotFoundHandler())
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+r.URL.Path, http.StatusFound)
	}))
	defer origin.Close()
	got := checkCatalogStatus(context.Background(), origin.Client(), candidate{url: origin.URL + "/jobs/123"})
	if got.verdict != liveness.Uncertain {
		t.Errorf("got %+v for redirect to another authority", got)
	}
}
