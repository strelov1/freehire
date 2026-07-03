package sources

import (
	"context"
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
