package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// arbeitnowPage renders an arbeitnow page around body: the site chrome, then the content. A
// removed posting keeps the chrome and answers 410, so the body alone cannot tell them apart.
func arbeitnowPage(body string) string {
	return `<!DOCTYPE html><html><head><title>Senior Software Engineer - Berlin | Arbeitnow</title></head><body>` +
		`<nav>Jobs Companies Blog</nav>` + body +
		strings.Repeat(`<p>Arbeitnow lists jobs with visa sponsorship and English-speaking teams in Germany.</p>`, 10) +
		`</body></html>`
}

func TestArbeitnowPassJudgesThePostingPage(t *testing.T) {
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
				_, _ = w.Write([]byte(arbeitnowPage(`<script type="application/ld+json">{"@type":"JobPosting"}</script>` +
					`<a href="/apply">Apply now</a>`)))
			},
			wantVerdict: liveness.Live,
		},
		{
			// A live .ch posting on 2026-10-30, whose own description matched the shared
			// probe's closed-posting phrases. The body is the employer's text, not a banner.
			name: "live posting whose description reads like a closed banner",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(arbeitnowPage(`<script type="application/ld+json">{"@type":"JobPosting"}</script>` +
					`<p>Please note that due to Brexit we are no longer accepting applications from the UK.</p>`)))
			},
			wantVerdict: liveness.Live,
		},
		{
			// What every removed posting sampled on 2026-10-30 answered: 410 with the full chrome.
			name: "removed posting",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusGone)
				_, _ = w.Write([]byte(arbeitnowPage(`<h2>Similar jobs</h2>`)))
			},
			wantVerdict: liveness.Expired,
			wantReason:  "http_gone",
		},
		{
			// The API answers a burst with 429 from the same host the crawl reads it from.
			name:        "rate limited",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
			wantVerdict: liveness.Uncertain,
			wantRefused: true,
		},
		{
			// The status is the whole answer, so a body cut short must not hide a refusal.
			name: "rate limited with a truncated body",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Length", "1000")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte("slow down"))
			},
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
			pass := arbeitnowPass(srv.Client())
			got := pass.check(context.Background(), candidate{url: srv.URL + "/jobs/companies/augustus/senior-software-engineer-berlin-102253"})
			if got.verdict != tc.wantVerdict || got.reason != tc.wantReason || got.refused != tc.wantRefused {
				t.Errorf("got %+v, want verdict=%v reason=%q refused=%v", got, tc.wantVerdict, tc.wantReason, tc.wantRefused)
			}
		})
	}
}
