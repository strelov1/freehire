package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// djinniPage renders a djinni posting page around body: the site chrome, then the posting.
func djinniPage(body string) string {
	return `<!DOCTYPE html><html><head><title>Senior PHP Developer at Acme – Djinni</title></head><body>` +
		`<nav>Jobs Salaries Companies</nav>` + body +
		strings.Repeat(`<p>We build payment software for European banks with PHP and PostgreSQL.</p>`, 10) +
		`</body></html>`
}

func TestCheckDjinniLive(t *testing.T) {
	cases := []struct {
		name        string
		handler     http.HandlerFunc
		wantVerdict liveness.Verdict
		wantReason  string
		wantRefused bool
	}{
		{
			name: "live posting",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(djinniPage(`<div id="apply_job"><button>Apply for the job</button></div>`)))
			},
			wantVerdict: liveness.Live,
		},
		{
			name: "removed posting",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(djinniPage(`<div class="alert alert-secondary" role="alert">
      <div class="fw-medium">
        The job ad is no longer active
      </div></div>`)))
			},
			wantVerdict: liveness.Expired,
			wantReason:  "expired_body",
		},
		{
			name:        "deleted posting",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			wantVerdict: liveness.Expired,
			wantReason:  "http_gone",
		},
		{
			// djinni's answer to a burst from a datacenter IP.
			name:        "metered",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) },
			wantVerdict: liveness.Uncertain,
			wantRefused: true,
		},
		{
			name:        "rate limited",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
			wantVerdict: liveness.Uncertain,
			wantRefused: true,
		},
		{
			name:        "server error",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) },
			wantVerdict: liveness.Uncertain,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)
			got := checkDjinniLive(context.Background(), srv.Client(), candidate{url: srv.URL + "/jobs/797771-fpga-engineer/"})
			if got.verdict != tc.wantVerdict || got.reason != tc.wantReason || got.refused != tc.wantRefused {
				t.Errorf("got %+v, want verdict=%v reason=%q refused=%v", got, tc.wantVerdict, tc.wantReason, tc.wantRefused)
			}
		})
	}
}
