package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// hhPage renders the head of an hh.ru vacancy page the way the site serves it: the analytics
// flag in window.globalVars, then the translation table every page carries — which holds the
// archived banner's text even on a LIVE posting.
func hhPage(archived string) string {
	return `<!DOCTYPE html><html><head><script>window.globalVars = {"analyticsParams": {"active": "false", "archived": "` +
		archived + `", "brandingType": null}};</script></head><body>` +
		`<template>{&#34;vacancy.view.archived.title&#34;:&#34;Вакансия в архиве&#34;}</template>` +
		strings.Repeat("<p>описание вакансии</p>", 50) + `</body></html>`
}

func hhServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckHHLive(t *testing.T) {
	cases := []struct {
		name        string
		handler     http.HandlerFunc
		wantVerdict liveness.Verdict
		wantReason  string
		wantRefused bool
	}{
		{
			name:        "live posting",
			handler:     func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(hhPage("false"))) },
			wantVerdict: liveness.Live,
		},
		{
			name:        "archived posting",
			handler:     func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(hhPage("true"))) },
			wantVerdict: liveness.Expired,
			wantReason:  "hh_vacancy_archived",
		},
		{
			name:        "deleted posting",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			wantVerdict: liveness.Expired,
			wantReason:  "hh_vacancy_gone",
		},
		{
			// Measured on ~7% of stale postings, the same ids on every read: not a verdict.
			name:        "forbidden posting",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) },
			wantVerdict: liveness.Uncertain,
		},
		{
			name:        "rate limited",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
			wantVerdict: liveness.Uncertain,
			wantRefused: true,
		},
		{
			// hh.ru's rate limit: a 302 to a captcha page that answers 200. Its body carries
			// the same translation table, so judging it on content would read it as archived.
			name: "captcha redirect",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/account/captcha") {
					_, _ = w.Write([]byte(hhPage("true")))
					return
				}
				http.Redirect(w, r, "/account/captcha?backurl=x", http.StatusFound)
			},
			wantVerdict: liveness.Uncertain,
			wantRefused: true,
		},
		{
			name: "redirect elsewhere",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/" {
					_, _ = w.Write([]byte(hhPage("true")))
					return
				}
				http.Redirect(w, r, "/", http.StatusFound)
			},
			wantVerdict: liveness.Uncertain,
		},
		{
			// The banner text alone — present on every hh page — is never evidence.
			name: "page without the flag",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`<html><body><h2>Вакансия в архиве</h2></body></html>`))
			},
			wantVerdict: liveness.Uncertain,
		},
		{
			name:        "server error",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) },
			wantVerdict: liveness.Uncertain,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := hhServer(t, tc.handler)
			got := checkHHLiveAt(context.Background(), srv.Client(), srv.URL+"/vacancy/%s", "136011204")
			if got.verdict != tc.wantVerdict || got.reason != tc.wantReason || got.refused != tc.wantRefused {
				t.Errorf("got %+v, want verdict=%v reason=%q refused=%v", got, tc.wantVerdict, tc.wantReason, tc.wantRefused)
			}
		})
	}
}

// The reason the shared plain-GET probe is not used for hh: Classify reads the translation
// table's archived phrase on a live page as a death signal. Pinned so that moving hh to
// probeDespiteRegisteredGET fails here first.
func TestSharedClassifierWouldCloseALiveHHPage(t *testing.T) {
	if v, _ := liveness.Classify(200, "https://hh.ru/vacancy/1", hhPage("false")); v != liveness.Expired {
		t.Fatalf("Classify on a live hh page = %v; if it no longer reads Expired, revisit hh.go's rationale", v)
	}
}

func TestHHVacancyID(t *testing.T) {
	cases := map[string]string{"96:136011204": "136011204", "136011204": "136011204", ":136011204": "136011204"}
	for in, want := range cases {
		if got, ok := hhVacancyID(in); !ok || got != want {
			t.Errorf("hhVacancyID(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "96:", "96:abc", "96:1/2"} {
		if got, ok := hhVacancyID(in); ok {
			t.Errorf("hhVacancyID(%q) = %q, ok; want rejected", in, got)
		}
	}
}

// A refusal stops the pass at once: the limit is per IP and shared with the crawl, so a run
// that kept going would hold the crawl host in the captcha too.
func TestApplyHHVerdictsStopsAtFirstRefusal(t *testing.T) {
	var requests atomic.Int64
	srv := hhServer(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	})
	cands := []candidate{
		{id: 1, source: "hh", externalID: "96:1"},
		{id: 2, source: "hh", externalID: "96:2"},
		{id: 3, source: "hh", externalID: "96:3"},
	}
	var st stats
	applyHHVerdicts(context.Background(), srv.Client(), nil, cands, &st, srv.URL+"/vacancy/%s", 1)
	if n := requests.Load(); n != 1 {
		t.Errorf("requests = %d, want 1 (stop at the first refusal)", n)
	}
	if n := st.probed.Load(); n != 0 {
		t.Errorf("probed = %d, want 0 (a refusal is not a verdict)", n)
	}
}

func TestApplyHHVerdictsBoundsOneRun(t *testing.T) {
	var requests atomic.Int64
	srv := hhServer(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusForbidden) // Uncertain: no DB write, so no queries needed
	})
	cands := make([]candidate, hhMaxProbesPerRun+5)
	for i := range cands {
		cands[i] = candidate{id: int64(i), source: "hh", externalID: "96:1"}
	}
	var st stats
	applyHHVerdicts(context.Background(), srv.Client(), nil, cands, &st, srv.URL+"/vacancy/%s", 1)
	if n := requests.Load(); n != hhMaxProbesPerRun {
		t.Errorf("requests = %d, want %d", n, hhMaxProbesPerRun)
	}
}
