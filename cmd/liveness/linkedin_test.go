package main

import (
	"context"
	"io"
	"net/http"
	"os"
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
		{"missing", "", 404, liveness.Expired, false},
		{"gone", "", 410, liveness.Expired, false},
		{"rate limit", "", 429, liveness.Uncertain, true},
		{"refused", "", 403, liveness.Uncertain, true},
		{"linkedin refusal", "", 999, liveness.Uncertain, true},
		{"unavailable", "", 503, liveness.Uncertain, false},
		{"wrong id", strings.ReplaceAll(strings.Replace(page, "STATUS", `<figcaption class="closed-job__flavor--closed">No longer accepting applications</figcaption>`, 1), "view/123/", "view/456/"), 200, liveness.Uncertain, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: linkedInTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/123" {
					t.Errorf("unexpected request: %s", r.URL)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header), Request: r}, nil
			})}
			got := linkedInPass(client, false).check(context.Background(), candidate{url: "https://www.linkedin.com/jobs/view/123/"})
			if got.verdict != tc.want || got.refused != tc.refused {
				t.Fatalf("got %+v, want %v, refused %v", got, tc.want, tc.refused)
			}
			if tc.want == liveness.Expired && got.reason != "linkedin_post_closed" && got.reason != "linkedin_post_gone" {
				t.Fatalf("wrong reason: %s", got.reason)
			}
		})
	}
}

func TestLinkedInProbeCannotCloseAfterRedirectToAnotherJob(t *testing.T) {
	client := &http.Client{Transport: linkedInTransport(func(r *http.Request) (*http.Response, error) {
		resp := &http.Response{StatusCode: 200, Header: make(http.Header), Request: r}
		if r.URL.Path == "/jobs-guest/jobs/api/jobPosting/123" {
			resp.StatusCode = 302
			resp.Header.Set("Location", "https://www.linkedin.com/jobs/view/456/")
			resp.Body = io.NopCloser(strings.NewReader(""))
		} else {
			resp.Body = io.NopCloser(strings.NewReader(`<section class="top-card-layout"><h1 class="top-card-layout__title">Engineer</h1><a class="topcard__org-name-link">Acme</a><figcaption class="closed-job__flavor--closed">No longer accepting applications</figcaption></section><div class="show-more-less-html__markup">Build APIs.</div>`))
		}
		return resp, nil
	})}
	got := linkedInPass(client, false).check(context.Background(), candidate{url: "https://www.linkedin.com/jobs/view/123/"})
	if got.verdict != liveness.Uncertain {
		t.Fatalf("redirect closed the wrong job: %+v", got)
	}
}

// Real guest pages saved 2026-11-22 (scripts and styles stripped): a posting LinkedIn marks
// "No longer accepting applications" and one still taking applications.
func TestLinkedInProbeReadsSavedGuestPages(t *testing.T) {
	for _, tc := range []struct {
		file, id string
		want     liveness.Verdict
	}{
		{"testdata/linkedin_closed_4488268754.html", "4488268754", liveness.Expired},
		{"testdata/linkedin_open_4491614305.html", "4491614305", liveness.Live},
	} {
		page, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		client := &http.Client{Transport: linkedInTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(page))), Header: make(http.Header), Request: r}, nil
		})}
		got := linkedInPass(client, false).check(context.Background(), candidate{url: "https://www.linkedin.com/jobs/view/" + tc.id + "/"})
		if got.verdict != tc.want {
			t.Errorf("%s: got %+v, want %v", tc.file, got, tc.want)
		}
	}
}

// A 404 only counts for the posting's own guest URL, never after a redirect elsewhere.
func TestLinkedInProbeIgnoresRedirectedNotFound(t *testing.T) {
	client := &http.Client{Transport: linkedInTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/jobs-guest/jobs/api/jobPosting/123" {
			h := make(http.Header)
			h.Set("Location", "https://www.linkedin.com/jobs/view/456/")
			return &http.Response{StatusCode: http.StatusFound, Header: h, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	if got := linkedInPass(client, false).check(context.Background(), candidate{url: "https://www.linkedin.com/jobs/view/123/"}); got.verdict != liveness.Uncertain {
		t.Fatalf("redirected 404 closed the job: %+v", got)
	}
}

// Through the rotating proxy a refusal names one exit: it is retried, and only a refusal on
// every attempt stops the pass. A direct client is not retried.
func TestLinkedInProbeRetriesRefusalOnlyWhenProxied(t *testing.T) {
	for _, tc := range []struct {
		proxied   bool
		refusals  int
		wantCalls int
		refused   bool
	}{
		{false, 1, 1, true},
		{true, 2, 3, false},
		{true, 5, 3, true},
	} {
		calls := 0
		client := &http.Client{Transport: linkedInTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls <= tc.refusals {
				return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			}
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		})}
		got := linkedInPass(client, tc.proxied).check(context.Background(), candidate{url: "https://www.linkedin.com/jobs/view/123/"})
		if calls != tc.wantCalls || got.refused != tc.refused {
			t.Errorf("proxied=%v refusals=%d: calls=%d verdict=%+v", tc.proxied, tc.refusals, calls, got)
		}
	}
}
