package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// isolvedSitemapXML lists two /jobs/<id> forms for the same posting (bare + .html, which must
// dedup to one) plus a classification page and the bare /jobs/ listing (neither a posting).
const isolvedSitemapXML = `<?xml version="1.0" encoding="UTF-8"?>
<urlset>
<url><loc>https://acme.isolvedhire.com/jobs/1792515</loc></url>
<url><loc>https://acme.isolvedhire.com/jobs/1792515.html</loc></url>
<url><loc>https://acme.isolvedhire.com/jobsandemployment/classifications/Finance/286423/</loc></url>
<url><loc>https://acme.isolvedhire.com/jobs/</loc></url>
</urlset>`

const isolvedDetailHTML = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting","title":"Post-Harvest Technician",
"description":"<p>Trim plants.</p><script>evil()<\/script>","datePosted":"2026-06-11 00:00:00",
"hiringOrganization":{"@type":"Organization","name":"Crisp Community LLC"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Norwich","addressRegion":"CT","addressCountry":"US"}}}
</script></head><body></body></html>`

func TestIsolvedFamilyProvider(t *testing.T) {
	if got := NewIsolvedHire(nil).Provider(); got != "isolvedhire" {
		t.Errorf("isolvedhire Provider() = %q", got)
	}
	if got := NewApplicantPro(nil).Provider(); got != "applicantpro" {
		t.Errorf("applicantpro Provider() = %q", got)
	}
}

func TestIsolvedFetch(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/sitemap.xml", isolvedSitemapXML).
		route("/jobs/", isolvedDetailHTML)

	jobs, err := NewIsolvedHire(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Crisp Community", Board: "acme"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1 (the /jobs/<id> and .html forms dedup; classification and listing are skipped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "1792515" {
		t.Errorf("external_id = %q", j.ExternalID)
	}
	if j.Title != "Post-Harvest Technician" {
		t.Errorf("title = %q", j.Title)
	}
	if j.Location != "Norwich, CT, US" {
		t.Errorf("location = %q", j.Location)
	}
	if j.URL != "https://acme.isolvedhire.com/jobs/1792515" {
		t.Errorf("url = %q", j.URL)
	}
	if j.PostedAt == nil {
		t.Error("posted_at not parsed from space-separated datePosted")
	}
	if !strings.Contains(j.Description, "Trim plants") || strings.Contains(j.Description, "evil()") {
		t.Errorf("description not sanitized: %q", j.Description)
	}
}

// The applicantpro provider shares the impl but forms its host from applicantpro.com.
func TestApplicantProHost(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/sitemap.xml", strings.ReplaceAll(isolvedSitemapXML, "isolvedhire.com", "applicantpro.com")).
		route("/jobs/", isolvedDetailHTML)
	jobs, err := NewApplicantPro(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].URL != "https://acme.applicantpro.com/jobs/1792515" {
		t.Fatalf("applicantpro url wrong: %+v", jobs)
	}
}

// The two notices below are the platform's verbatim answers for a board it does not serve,
// captured from prod on 2026-09-06. Both arrive as HTTP 200 at the sitemap URL, which is why
// the adapter has to read the body to tell them from a sitemap.
const (
	isolvedDisabledNotice = `This career site has been disabled. Contact the Sales Representative in charge of ` +
		`this account to find out how to enable this career site.
<!-- GA4 - Google tag (gtag.js) -->
<script async src="https://www.googletagmanager.com/gtag/js?id=G-1QL0HHW9LT"></script>`
	isolvedNoTenantNotice = `You may have typed the url for this website incorrectly. Please double check what ` +
		`you typed and try again.
<!-- GA4 - Google tag (gtag.js) -->
<script async src="https://www.googletagmanager.com/gtag/js?id=G-1QL0HHW9LT"></script>`
)

// A board the platform no longer serves must come back as ErrBoardGone and not as whatever
// the XML decoder made of the notice. Before this check the disabled page surfaced in
// board_health as "XML syntax error on line 3", under which 41 gone boards sat unnoticed
// from July to September 2026.
func TestIsolvedFetchReportsBoardGone(t *testing.T) {
	for name, notice := range map[string]string{
		"disabled by the vendor": isolvedDisabledNotice,
		"no such tenant":         isolvedNoTenantNotice,
	} {
		t.Run(name, func(t *testing.T) {
			fake := (&routedHTTP{}).route("/sitemap.xml", notice)
			_, err := NewIsolvedHire(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
			if !errors.Is(err, ErrBoardGone) {
				t.Fatalf("err = %v, want it to wrap ErrBoardGone", err)
			}
			if !strings.Contains(err.Error(), "acme") {
				t.Errorf("err = %q, want the board named in it", err)
			}
		})
	}
}

// The notice check reads only the head of the stream and must leave every byte for the
// decoder — a sitemap whose first bytes are inspected still has to parse in full.
func TestIsolvedFetchStillReadsASitemapAfterThePeek(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/sitemap.xml", isolvedSitemapXML).
		route("/jobs/", isolvedDetailHTML)
	jobs, err := NewIsolvedHire(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1 — the peek consumed part of the stream", len(jobs))
	}
}

// A real sitemap that happens to carry a posting worded like the notice is still a sitemap.
// The check looks at the head only, so a match deeper in the document must not fire.
func TestIsolvedBoardGoneIgnoresNoticeWordingInsideASitemap(t *testing.T) {
	if isolvedBoardGone(isolvedSitemapXML) {
		t.Error("a plain sitemap was read as a gone board")
	}
	if !isolvedBoardGone(strings.ToUpper(isolvedDisabledNotice)) {
		t.Error("the notice must match regardless of case")
	}
}
