package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestCatalogStatusScopedBackstops(t *testing.T) {
	cases := []struct{ source, id, url string }{
		{"arbeitsagentur", "10000-1208350819-S", "https://www.arbeitsagentur.de/jobsuche/jobdetail/10000-1208350819-S"},
		{"xing", "158886509", "https://www.xing.com/jobs/berlin-engineer-158886509"},
		{"gradcracker", "87479", "https://www.gradcracker.com/hub/1/company/graduate-job/87479/software-engineer"},
		{"engage", "352500:17322826", "https://en-gage.net/owltech/work_17322826/"},
		{"betterteam", "surpluscityinc:retail-store-associate-10", "https://surpluscityinc.betterteam.com/retail-store-associate-10"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: catalogStatusTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 410, Body: http.NoBody, Request: r, Header: make(http.Header)}, nil
			})}
			found := false
			for _, pass := range catalogStatusPasses(client) {
				if pass.source != tc.source {
					continue
				}
				found = true
				c := candidate{url: tc.url, externalID: tc.id}
				if got := pass.check(context.Background(), c); got.verdict != liveness.Expired || calls != 1 {
					t.Fatalf("got %+v calls=%d", got, calls)
				}
				for _, bad := range []candidate{
					{url: strings.Replace(tc.url, "://", "://user@", 1), externalID: tc.id}, {url: tc.url, externalID: "wrong:id"}, {url: tc.url + "/unrelated", externalID: tc.id}, {url: "http" + tc.url[5:], externalID: tc.id}} {
					if got := pass.check(context.Background(), bad); got.verdict != liveness.Uncertain || calls != 1 {
						t.Fatalf("accepted %+v: %+v calls=%d", bad, got, calls)
					}
				}
				if tc.source == "betterteam" {
					wrong := candidate{url: tc.url, externalID: "different:retail-store-associate-10"}
					if got := pass.check(context.Background(), wrong); got.verdict != liveness.Uncertain || calls != 1 {
						t.Fatal("accepted different tenant", got)
					}
				}
				if tc.source == "gradcracker" && pass.interval != 10*time.Second {
					t.Fatal("lost measured pacing")
				}
				if tc.source == "engage" && pass.interval != 3500*time.Millisecond {
					t.Fatal("lost measured pacing")
				}
				if pass.maxPerRun != 500 {
					t.Fatalf("unbounded pass: %+v", pass)
				}
			}
			if !found {
				t.Fatal("missing scoped pass")
			}
		})
	}
}

type catalogStatusTransport func(*http.Request) (*http.Response, error)

func (f catalogStatusTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
