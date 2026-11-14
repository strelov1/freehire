package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/liveness"
)

type linkedInTransport func(*http.Request) (*http.Response, error)

func (f linkedInTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLinkedInProbeRequiresExplicitPostingEvidence(t *testing.T) {
	const page = `<meta property="og:url" content="https://www.linkedin.com/jobs/view/123/">
<section class="top-card-layout"><h1 class="top-card-layout__title">Engineer</h1>
<a class="topcard__org-name-link">Acme</a>STATUS</section>
<div class="show-more-less-html__markup"><p>Build APIs.</p></div>`
	for _, tc := range []struct {
		name, body string
		status     int
		want       liveness.Verdict
		refused    bool
	}{
		{"open", strings.Replace(page, "STATUS", `<button data-modal="job-details-topcard-apply-modal">Apply</button>`, 1), 200, liveness.Live, false},
		{"closed", strings.Replace(page, "STATUS", `<figcaption class="closed-job__flavor--closed">No longer accepting applications</figcaption>`, 1), 200, liveness.Expired, false},
		{"login", `<h1>Sign in</h1>`, 200, liveness.Uncertain, false},
		{"missing", "", 404, liveness.Uncertain, false},
		{"gone", "", 410, liveness.Uncertain, false},
		{"rate limit", "", 429, liveness.Uncertain, true},
		{"refused", "", 403, liveness.Uncertain, true},
		{"linkedin refusal", "", 999, liveness.Uncertain, true},
		{"unavailable", "", 503, liveness.Uncertain, false},
		{"wrong id", strings.ReplaceAll(page, "view/123/", "view/456/"), 200, liveness.Uncertain, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: linkedInTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://www.linkedin.com/jobs/view/123/" {
					t.Errorf("unexpected request: %s", r.URL)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header), Request: r}, nil
			})}
			got := linkedInPass(client).check(context.Background(), candidate{url: "https://www.linkedin.com/jobs/view/123/"})
			if got.verdict != tc.want || got.refused != tc.refused {
				t.Fatalf("got %+v, want %v, refused %v", got, tc.want, tc.refused)
			}
			if tc.want == liveness.Expired && got.reason != "linkedin_post_closed" {
				t.Fatalf("wrong reason: %s", got.reason)
			}
		})
	}
}

func TestLinkedInProbeCannotCloseAfterRedirectToAnotherJob(t *testing.T) {
	client := &http.Client{Transport: linkedInTransport(func(r *http.Request) (*http.Response, error) {
		resp := &http.Response{StatusCode: 200, Header: make(http.Header), Request: r}
		if r.URL.Path == "/jobs/view/123/" {
			resp.StatusCode = 302
			resp.Header.Set("Location", "https://www.linkedin.com/jobs/view/456/")
			resp.Body = io.NopCloser(strings.NewReader(""))
		} else {
			resp.Body = io.NopCloser(strings.NewReader(`<section class="top-card-layout"><h1 class="top-card-layout__title">Engineer</h1><a class="topcard__org-name-link">Acme</a><figcaption class="closed-job__flavor--closed">No longer accepting applications</figcaption></section><div class="show-more-less-html__markup">Build APIs.</div>`))
		}
		return resp, nil
	})}
	got := linkedInPass(client).check(context.Background(), candidate{url: "https://www.linkedin.com/jobs/view/123/"})
	if got.verdict != liveness.Uncertain {
		t.Fatalf("redirect closed the wrong job: %+v", got)
	}
}
