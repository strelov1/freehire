package boardresolve

import (
	"context"
	"testing"
)

func TestResolveCustomDomain(t *testing.T) {
	const oracle = `<base href="/en/sites/ciklum-career" data-sitenumber="CX_1001" data-apibaseurl="https://ialmme.fa.ocs.oraclecloud.com:443">`
	const odooScript = `<script id="web.layout.odooscript"></script>`
	const oracleBoard = "ialmme.fa.ocs.oraclecloud.com/CX_1001"
	const oracleURL = "https://ialmme.fa.ocs.oraclecloud.com/hcmUI/CandidateExperience/en/sites/CX_1001"
	const odooURL = "https://www.voltho.com/jobs"

	for _, tc := range []struct {
		name, rawURL, body, source, board, canonical string
	}{
		{"Oracle vanity posting", "https://explore-jobs.ciklum.com/en/sites/ciklum-career/job/4517", oracle, "oracle", oracleBoard, oracleURL},
		{"Odoo vanity posting", odooURL + "/engineer-5", odooScript + `<div class="wrap js_hr_recruitment">`, "odoo", "www.voltho.com", odooURL},
		{"Odoo listing", odooURL, odooScript + `<div class="o_website_hr_recruitment_jobs_list">`, "odoo", "www.voltho.com", odooURL},
		{"Odoo store page", "https://www.voltho.com/shop", odooScript, "", "", ""},
		{"Oracle wrong API host", "https://example.com/jobs", `<base data-sitenumber="CX_1001" data-apibaseurl="https://ialmme.fa.ocs.oraclecloud.com.example.com">`, "", "", ""},
		{"Oracle attributes on different elements", "https://example.com/jobs", `<base data-apibaseurl="https://ialmme.fa.ocs.oraclecloud.com"><div data-sitenumber="CX_1001">`, "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, board, canonical, ok := resolver(tc.body, nil).Resolve(context.Background(), tc.rawURL)
			if source != tc.source || board != tc.board || canonical != tc.canonical || ok != (tc.source != "") {
				t.Fatalf("resolved (%q, %q, %q, %v), want (%q, %q, %q, %v)", source, board, canonical, ok, tc.source, tc.board, tc.canonical, tc.source != "")
			}
		})
	}
}

func TestResolveRecognizesRedirectedATS(t *testing.T) {
	r := &Resolver{http: redirectedCompanyPages{}}
	source, board, canonical, ok := r.Resolve(context.Background(), "https://old.example/ats")
	if !ok || source != "lever" || board != "acme" || canonical != "https://jobs.eu.lever.co/acme" {
		t.Fatalf("redirected ATS = %s/%s/%s/%v", source, board, canonical, ok)
	}
}

func TestResolveMultiEmployerIndicesDoNotPickFeaturedEmployer(t *testing.T) {
	for _, tc := range []struct {
		index, featured string
	}{
		{"https://web3.career/?ref=x", "https://tether.recruitee.com/o/engineer"},
		{"https://employmenthero.com/jobs/", "https://employmenthero.com/jobs/organisations/acme/"},
		{"https://jobboard.catch-talents.de/", "https://jobboard.catch-talents.de/companies/259"},
	} {
		r := resolver(`<a href="`+tc.featured+`">Featured job</a>`, nil)
		if source, board, _, ok := r.Resolve(context.Background(), tc.index); ok {
			t.Fatalf("%s resolved to %s/%s", tc.index, source, board)
		}
	}
}

func TestResolveEmploymentHeroPositionThroughEmployerURL(t *testing.T) {
	r := resolver(`<script type="application/ld+json">{"@type":"JobPosting","hiringOrganization":{"sameAs":"https://employmenthero.com/jobs/organisations/employmenthero/"}}</script>`, nil)
	source, board, _, ok := r.Resolve(context.Background(), "https://employmenthero.com/jobs/position/employment-hero-senior-backend-engineer-remote-vietnam-0jarr/")
	if !ok || source != "employmenthero" || board != "employmenthero" {
		t.Fatalf("employment hero posting resolved to %s/%s/%v", source, board, ok)
	}
}

type redirectedIndexFixture struct{ fakeFetcher }

func (redirectedIndexFixture) GetTextResolved(context.Context, string) (string, string, error) {
	return `<a href="https://jobboard.catch-talents.de/companies/259">Featured job</a>`,
		"https://jobboard.catch-talents.de/", nil
}

func TestResolveRedirectedMultiEmployerIndex(t *testing.T) {
	r := &Resolver{http: redirectedIndexFixture{}}
	if source, board, _, ok := r.Resolve(context.Background(), "https://example.com/jobs"); ok {
		t.Fatalf("redirected index resolved to %s/%s", source, board)
	}
}
