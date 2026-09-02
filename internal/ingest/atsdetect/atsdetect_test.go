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
		// Second tier: any URL FromURL can parse into a board is detected too.
		{
			name:     "workday url in careers page",
			html:     `<a href="https://xavier.wd1.myworkdayjobs.com/en-US/XavierCareers/job/US/role_R123">Apply</a>`,
			provider: "workday", slug: "xavier.wd1.myworkdayjobs.com/XavierCareers", ok: true,
		},
		{
			name:     "oracle url in careers page",
			html:     `<iframe src="https://edzz.fa.em3.oraclecloud.com/hcmUI/CandidateExperience/en/sites/CX_6001/requisitions"></iframe>`,
			provider: "oracle", slug: "edzz.fa.em3.oraclecloud.com/CX_6001", ok: true,
		},
		{
			name:     "taleo url in careers page",
			html:     `window.open('https://valero.taleo.net/careersection/2/jobsearch.ftl')`,
			provider: "taleo", slug: "valero.taleo.net/2", ok: true,
		},
		{
			name:     "cornerstone url in careers page",
			html:     `<link href="https://nintendoeurope.csod.com/ux/ats/careersite/1/home?c=nintendoeurope"/>`,
			provider: "cornerstone", slug: "nintendoeurope", ok: true,
		},
		{
			name:     "first-tier greenhouse still wins over second-tier workday",
			html:     `<a href="https://acme.wd1.myworkdayjobs.com/x/y"></a><a href="https://boards.greenhouse.io/acme"></a>`,
			provider: "greenhouse", slug: "acme", ok: true,
		},
		{
			// "notjobs.lever.co/" contains "jobs.lever.co/" as a plain substring, so an
			// unanchored regex would misdetect this look-alike host as the real Lever
			// domain. The matcher regexes anchor on a left boundary specifically to
			// reject this.
			name: "look-alike lever host is not matched as lever",
			html: `<a href="https://evil-notjobs.lever.co/acme">Careers</a>`,
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

// TestDetectSelfHosted covers the career sites served from the employer's own domain, where the
// board is the host and only the vendor's bundle in the markup says which platform it is. The
// markers are the ones sampled live off the boards already in radancy.yml / phenom.yml /
// jibe.yml / teamtailor.yml.
func TestDetectSelfHosted(t *testing.T) {
	cases := []struct {
		name     string
		html     string
		host     string
		provider string
		board    string
		ok       bool
	}{
		{
			name: "radancy talentbrew bundle", host: "jobs.intuit.com",
			html:     `<script src="https://cdn.example.com/talentbrew/main.js"></script>`,
			provider: "radancy", board: "jobs.intuit.com", ok: true,
		},
		{
			name: "phenom widget config", host: "jobs.kuehne-nagel.com",
			html:     `<script>var phApp={"ddoKey":"refineSearch"};</script><link href="//assets.phenompeople.com/x.css">`,
			provider: "phenom", board: "jobs.kuehne-nagel.com", ok: true,
		},
		{
			name: "jibe apply bundle", host: "careers.amd.com",
			html:     `<div data-jibeapply="1"></div>`,
			provider: "jibe", board: "careers.amd.com", ok: true,
		},
		{
			name: "teamtailor on a custom domain", host: "careers.investengine.com",
			html:     `<meta name="generator" content="Teamtailor">`,
			provider: "teamtailor", board: "careers.investengine.com", ok: true,
		},
		// www is not part of the board: the adapters and board files store the bare careers host.
		{
			name: "www stripped from the board", host: "www.github.careers",
			html:     `<script src="/jibeapply.js"></script>`,
			provider: "jibe", board: "github.careers", ok: true,
		},
		// The vendor's own site carries its own marker; it is never an employer board. Nor is a
		// tenant hosted on the vendor's domain — atsboard resolves that one from the URL alone.
		{name: "vendor marketing site", host: "www.phenom.com", html: `phenompeople`, ok: false},
		{name: "tenant on the vendor domain", host: "bryter.teamtailor.com", html: `Teamtailor`, ok: false},
		// A tenant that moved to Eightfold keeps the previous ATS's tags for a while; the host is
		// the vendor's, so the page is not a self-hosted board of whatever marker it still carries.
		{name: "eightfold tenant still tagged talentbrew", host: "johndeere.eightfold.ai", html: `talentbrew`, ok: false},
		// Another ATS's page must not be claimed.
		{name: "greenhouse embed page", host: "acme.com", html: `<script src="https://boards.greenhouse.io/embed/job_board/js?for=acme"></script>`, ok: false},
		{name: "plain careers page", host: "acme.com", html: `<h1>Join us</h1>`, ok: false},
		{name: "no host", host: "", html: `talentbrew`, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, b, ok := DetectSelfHosted(tc.html, tc.host)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (got provider=%q board=%q)", ok, tc.ok, p, b)
			}
			if ok && (p != tc.provider || b != tc.board) {
				t.Errorf("got (%q, %q), want (%q, %q)", p, b, tc.provider, tc.board)
			}
		})
	}
}
