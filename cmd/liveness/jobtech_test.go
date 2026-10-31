package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/liveness"
)

func TestJobtechPassJudgesTheAdByJobSearch(t *testing.T) {
	cases := []struct {
		name        string
		handler     http.HandlerFunc
		wantVerdict liveness.Verdict
		wantReason  string
		wantRefused bool
	}{
		{
			// What 147 of 200 stale rows sampled on 2026-10-31 answered.
			name: "live ad",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"id":"31572438","headline":"Lagerarbetare","removed":false,"removed_date":null,"application_deadline":"2026-11-06T23:59:59"}`))
			},
			wantVerdict: liveness.Live,
		},
		{
			// A removal JobSearch still serves, for a while, as the ad with its flag set.
			name: "removed ad",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"id":"31325249","headline":"Snickare","removed":true,"removed_date":"2026-10-30T12:08:17"}`))
			},
			wantVerdict: liveness.Expired,
			wantReason:  "jobtech_ad_removed",
		},
		{
			// What 53 of the 200 answered: an ad removed long enough ago that JobSearch dropped it.
			name: "ad JobSearch no longer knows",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"Ad not found"}`))
			},
			wantVerdict: liveness.Expired,
			wantReason:  "jobtech_ad_gone",
		},
		{
			name:        "rate limited",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
			wantVerdict: liveness.Uncertain,
			wantRefused: true,
		},
		{
			// A 200 that does not say either way is not evidence of anything.
			name: "answer without the removed flag",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"id":"31572438"}`))
			},
			wantVerdict: liveness.Uncertain,
		},
		{
			name: "garbled answer",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`<html>maintenance</html>`))
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
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				tc.handler(w, r)
			}))
			t.Cleanup(srv.Close)
			pass := jobtechPass(srv.Client(), srv.URL+"/ad/%s")
			// The stored external_id carries the boardless namespace's leading colon.
			got := pass.check(context.Background(), candidate{externalID: ":31572438", publicSlug: "lagerarbetare-x"})
			if gotPath != "/ad/31572438" {
				t.Errorf("requested %q, want /ad/31572438", gotPath)
			}
			if got.verdict != tc.wantVerdict || got.reason != tc.wantReason || got.refused != tc.wantRefused {
				t.Errorf("got %+v, want verdict=%v reason=%q refused=%v", got, tc.wantVerdict, tc.wantReason, tc.wantRefused)
			}
		})
	}
}

func TestJobtechPassSkipsACandidateWithNoAdID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("probed a candidate with no ad id")
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	for _, id := range []string{"", ":", ":31572438/../x"} {
		got := jobtechPass(srv.Client(), srv.URL+"/ad/%s").check(context.Background(), candidate{externalID: id})
		if got.verdict != liveness.Uncertain {
			t.Errorf("external_id %q: got %+v, want Uncertain", id, got)
		}
	}
	if !strings.Contains(jobtechAdAPI, "jobsearch.api.jobtechdev.se/ad/") {
		t.Errorf("jobtechAdAPI = %q, want JobSearch's per-ad endpoint", jobtechAdAPI)
	}
}
