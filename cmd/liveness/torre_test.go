package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// torrePage renders the tail of a Torre posting page's embedded state the way the site serves
// it: the members' own statuses first (which must never be read as the posting's), then the
// opportunity's prefilledStatus and status assignments.
func torrePage(prefilled, status string) string {
	return `<html><head><title>Integration Lead at Creative IT | Torre</title></head><body>` +
		strings.Repeat(`<script>/* bundle */</script>`, 20) +
		`<script>(function(a,b,c){var g={};g.members=[{status:"accepted"},{status:"closed"}];` +
		`g.template=c;g.prefilledStatus=` + prefilled + `;g.status="` + status + `";g.locale="en";})(false,true,null)</script>` +
		`</body></html>`
}

func TestCheckTorreLive(t *testing.T) {
	cases := []struct {
		name        string
		handler     http.HandlerFunc
		wantVerdict liveness.Verdict
		wantReason  string
		wantRefused bool
	}{
		{
			name:        "open posting",
			handler:     func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(torrePage("c", "open"))) },
			wantVerdict: liveness.Live,
		},
		{
			name:        "closed posting",
			handler:     func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(torrePage("c", "closed"))) },
			wantVerdict: liveness.Expired,
			wantReason:  "torre_post_closed",
		},
		{
			// A posting created from a template carries prefilledStatus as an object.
			name: "closed posting with a prefilled object",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(torrePage(`{status:"prefilled",opportunityId:"ed89p0br"}`, "closed")))
			},
			wantVerdict: liveness.Expired,
			wantReason:  "torre_post_closed",
		},
		{
			name:        "another status",
			handler:     func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(torrePage("c", "paused"))) },
			wantVerdict: liveness.Uncertain,
		},
		{
			// Only the members' statuses, no assignment of the posting's own: not a verdict.
			name: "page without the posting's status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`<html><script>g.members=[{status:"closed"}];</script></html>`))
			},
			wantVerdict: liveness.Uncertain,
		},
		{
			// An unknown id redirects to /404, which answers 404.
			name: "unknown posting",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/404" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				http.Redirect(w, r, "/404", http.StatusFound)
			},
			wantVerdict: liveness.Expired,
			wantReason:  "torre_post_gone",
		},
		{
			name:        "rate limited",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
			wantVerdict: liveness.Uncertain,
			wantRefused: true,
		},
		{
			name:        "server error",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
			wantVerdict: liveness.Uncertain,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)
			got := checkTorreLiveAt(context.Background(), srv.Client(), srv.URL+"/post/%s", "GdjM761w")
			if got.verdict != tc.wantVerdict || got.reason != tc.wantReason || got.refused != tc.wantRefused {
				t.Errorf("got %+v, want verdict=%v reason=%q refused=%v", got, tc.wantVerdict, tc.wantReason, tc.wantRefused)
			}
		})
	}
}

// The pass asks about the id the external id carries, without UpsertJob's boardless ":".
func TestTorrePassReadsTheExternalID(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(torrePage("c", "open")))
	}))
	t.Cleanup(srv.Close)
	p := torrePass(srv.Client(), srv.URL+"/post/%s")
	if got := p.check(context.Background(), candidate{externalID: ":GdjM761w"}); got.verdict != liveness.Live {
		t.Fatalf("verdict = %v, want Live", got.verdict)
	}
	if path != "/post/GdjM761w" {
		t.Errorf("requested %q, want /post/GdjM761w", path)
	}
}
