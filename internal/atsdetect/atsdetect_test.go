package atsdetect

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		name     string
		html     string
		provider string
		slug     string
		ok       bool
	}{
		{
			name:     "greenhouse direct board link",
			html:     `<a href="https://boards.greenhouse.io/acme">Careers</a>`,
			provider: "greenhouse", slug: "acme", ok: true,
		},
		{
			name:     "greenhouse job-boards host",
			html:     `fetch("https://job-boards.greenhouse.io/acme-corp/jobs")`,
			provider: "greenhouse", slug: "acme-corp", ok: true,
		},
		{
			name:     "greenhouse embed captures for= not embed",
			html:     `<script src="https://boards.greenhouse.io/embed/job_board/js?for=acme"></script>`,
			provider: "greenhouse", slug: "acme", ok: true,
		},
		{
			name:     "lever",
			html:     `<a href="https://jobs.lever.co/scopear/">Jobs</a>`,
			provider: "lever", slug: "scopear", ok: true,
		},
		{
			name:     "ashby",
			html:     `window.location='https://jobs.ashbyhq.com/verge-genomics'`,
			provider: "ashby", slug: "verge-genomics", ok: true,
		},
		{
			name: "no ats link",
			html: `<html><body>We are hiring! Email us.</body></html>`,
			ok:   false,
		},
		{
			name:     "greenhouse precedence when multiple present",
			html:     `<a href="https://jobs.lever.co/acme"></a><a href="https://boards.greenhouse.io/acme"></a>`,
			provider: "greenhouse", slug: "acme", ok: true,
		},
		{
			name: "bare embed without for= is not a board",
			html: `<script src="https://boards.greenhouse.io/embed/job_board/js"></script>`,
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, s, ok := Detect(tc.html)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (got provider=%q slug=%q)", ok, tc.ok, p, s)
			}
			if ok && (p != tc.provider || s != tc.slug) {
				t.Errorf("got (%q, %q), want (%q, %q)", p, s, tc.provider, tc.slug)
			}
		})
	}
}
