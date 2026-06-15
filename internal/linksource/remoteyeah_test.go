package linksource

import (
	"context"
	"strings"
	"testing"
	"time"
)

// remoteYeahJobHTML is a remoteyeah.com/jobs/<slug> page reduced to the JobPosting block
// (alongside the other ld+json blocks the real page carries, to prove the adapter picks
// the JobPosting one). identifier is null — the slug is the id; jobLocationType marks it
// remote; baseSalary is structured.
const remoteYeahJobHTML = `<html><head>
<script type="application/ld+json">{"@context":"https://schema.org/","@type":"WebSite","name":"RemoteYeah"}</script>
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
 "title":"Senior Platform & Security Engineer",
 "datePosted":"2026-06-13T11:31:17+00:00",
 "description":"<p>Build it.</p><script>alert(1)<\/script>",
 "identifier":null,
 "hiringOrganization":{"@type":"Organization","name":"Taekus"},
 "jobLocation":null,
 "jobLocationType":"TELECOMMUTE",
 "baseSalary":{"@type":"MonetaryAmount","currency":"USD","value":{"@type":"QuantitativeValue","minValue":175000,"maxValue":230000}},
 "employmentType":["FULL_TIME"]}
</script></head><body></body></html>`

// remoteYeahXSSHTML carries a malicious currency in the structured salary — the adapter
// folds salary into the {@html}-rendered description, so it must be sanitized.
const remoteYeahXSSHTML = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
 "title":"Engineer","description":"<p>Build it.</p>",
 "hiringOrganization":{"@type":"Organization","name":"Acme"},
 "baseSalary":{"@type":"MonetaryAmount","currency":"USD</p><script>alert(1)<\/script><p>","value":{"@type":"QuantitativeValue","minValue":100000,"maxValue":120000}}}
</script></head><body></body></html>`

func TestRemoteYeahSanitizesFoldedSalary(t *testing.T) {
	c := (&fakeClient{}).route("/jobs/evil", remoteYeahXSSHTML, "")
	job, ok, err := NewRemoteYeah(c).Resolve(context.Background(), "https://remoteyeah.com/jobs/evil")
	if err != nil || !ok {
		t.Fatalf("resolve: ok=%v err=%v", ok, err)
	}
	if strings.Contains(job.Description, "<script>") {
		t.Errorf("description carries unsanitized salary markup: %q", job.Description)
	}
	if !strings.Contains(job.Description, "100000") {
		t.Errorf("salary range dropped: %q", job.Description)
	}
}

func TestRemoteYeahRejectsRedirectToForeignHost(t *testing.T) {
	// The link host matches, but the page resolves (via redirect) to an internal
	// target. The adapter must refuse to ingest a foreign host's HTML.
	c := (&fakeClient{}).route("/jobs/evil", remoteYeahJobHTML, "http://169.254.169.254/latest/meta-data/")
	_, ok, err := NewRemoteYeah(c).Resolve(context.Background(), "https://remoteyeah.com/jobs/evil")
	if err == nil {
		t.Fatalf("expected rejection of redirect to a foreign host, got ok=%v err=nil", ok)
	}
}

func TestRemoteYeahResolvesJobPage(t *testing.T) {
	const link = "https://remoteyeah.com/jobs/remote-senior-platform-security-engineer-taekus?utm_source=telegram"
	c := (&fakeClient{}).route("/jobs/remote-senior-platform-security-engineer-taekus", remoteYeahJobHTML, "")

	job, ok, err := NewRemoteYeah(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want the vacancy resolved")
	}
	if job.ExternalID != "remote-senior-platform-security-engineer-taekus" {
		t.Errorf("ExternalID = %q, want the slug", job.ExternalID)
	}
	if job.URL != "https://remoteyeah.com/jobs/remote-senior-platform-security-engineer-taekus" {
		t.Errorf("URL = %q, want canonical job URL without utm", job.URL)
	}
	if job.Title != "Senior Platform & Security Engineer" {
		t.Errorf("Title = %q", job.Title)
	}
	if job.Company != "Taekus" {
		t.Errorf("Company = %q, want Taekus", job.Company)
	}
	if !job.Remote {
		t.Error("Remote = false, want true (jobLocationType TELECOMMUTE)")
	}
	if strings.Contains(job.Description, "<script>") || !strings.Contains(job.Description, "Build it.") {
		t.Errorf("Description not sanitized: %q", job.Description)
	}
	if !strings.Contains(job.Description, "175000") || !strings.Contains(job.Description, "230000") {
		t.Errorf("Description missing folded salary range: %q", job.Description)
	}
	if job.PostedAt == nil || !job.PostedAt.Equal(time.Date(2026, 6, 13, 11, 31, 17, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-13T11:31:17Z", job.PostedAt)
	}
}
