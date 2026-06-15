package linksource

import (
	"context"
	"strings"
	"testing"
	"time"
)

// geekjobVacancyHTML mirrors a real geekjob.ru vacancy page's JobPosting ld+json: a hex id
// in the URL (and a separate internal numeric identifier we ignore), a clean company, a
// MonetaryAmount salary with a unitText, and an HTML description.
const geekjobVacancyHTML = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
 "title":"Head of HR в IT Consulting, remote",
 "datePosted":"2026-06-02",
 "description":"<h4>О компании</h4><p>Build &amp; grow.</p><script>evil()<\/script>",
 "identifier":{"@type":"PropertyValue","name":"Geekjob 6a1ebb8520ad023342091661","value":"171018"},
 "hiringOrganization":{"@type":"Organization","name":"Агентство NEWHR"},
 "baseSalary":{"@type":"MonetaryAmount","currency":"USD","value":{"@type":"QuantitativeValue","unitText":"MONTH","minValue":4000,"maxValue":5000}}}
</script></head><body></body></html>`

func TestGeekjobRejectsRedirectToForeignHost(t *testing.T) {
	// The link host matches, but the page resolves to an internal target; the adapter
	// must refuse to ingest a foreign host's HTML.
	c := (&fakeClient{}).route("/vacancy/6a1ebb8520ad023342091661", geekjobVacancyHTML, "http://10.0.0.1/internal")
	_, ok, err := NewGeekjob(c).Resolve(context.Background(), "https://geekjob.ru/vacancy/6a1ebb8520ad023342091661")
	if err == nil {
		t.Fatalf("expected rejection of redirect to a foreign host, got ok=%v err=nil", ok)
	}
}

func TestGeekjobResolvesVacancy(t *testing.T) {
	const link = "https://geekjob.ru/vacancy/6a1ebb8520ad023342091661?utm_source=telegram"
	c := (&fakeClient{}).route("/vacancy/6a1ebb8520ad023342091661", geekjobVacancyHTML, "")

	job, ok, err := NewGeekjob(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want the vacancy resolved")
	}
	// The canonical id is the hex from the URL, not the internal identifier value (171018).
	if job.ExternalID != "6a1ebb8520ad023342091661" {
		t.Errorf("ExternalID = %q, want the URL hex id", job.ExternalID)
	}
	if job.URL != "https://geekjob.ru/vacancy/6a1ebb8520ad023342091661" {
		t.Errorf("URL = %q, want canonical without utm", job.URL)
	}
	if !strings.Contains(job.Title, "Head of HR") {
		t.Errorf("Title = %q", job.Title)
	}
	if job.Company != "Агентство NEWHR" {
		t.Errorf("Company = %q, want Агентство NEWHR", job.Company)
	}
	if strings.Contains(job.Description, "<script>") || !strings.Contains(job.Description, "Build &amp; grow.") {
		t.Errorf("Description not sanitized/decoded: %q", job.Description)
	}
	if !strings.Contains(job.Description, "4000") || !strings.Contains(job.Description, "5000") ||
		!strings.Contains(job.Description, "USD") || !strings.Contains(job.Description, "month") {
		t.Errorf("Description missing folded salary (4000–5000 USD/month): %q", job.Description)
	}
	if job.PostedAt == nil || !job.PostedAt.Equal(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-02", job.PostedAt)
	}
}
