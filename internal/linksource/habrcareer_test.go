package linksource

import (
	"context"
	"strings"
	"testing"
	"time"
)

// habrVacancyHTML is a career.habr.com vacancy page reduced to the schema.org JobPosting
// ld+json block the adapter reads (address is a bare string, as Habr emits it; the
// description carries an entity and a <script> to prove sanitization). The ld+json
// datePosted is intentionally bogus (a month ahead, like Habr's real output) and the
// trustworthy date lives in the <time class="basic-date" datetime> element — the adapter
// must read the <time>, not datePosted.
const habrVacancyHTML = `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org/","@type":"JobPosting",
 "title":"Junior Product Manager (Дистанционный мониторинг)",
 "datePosted":"2026-07-14",
 "validThrough":"2026-08-13",
 "description":"<p>Обязанности &amp; требования</p><script>alert(1)<\/script><ul><li>REST</li></ul>",
 "identifier":{"@type":"PropertyValue","name":"СберЗдоровье","value":"1000166712"},
 "hiringOrganization":{"@type":"Organization","name":"СберЗдоровье"},
 "jobLocation":[{"@type":"Place","address":"Москва"}],
 "employmentType":"FULL_TIME"}
</script></head><body><h1>Junior Product Manager</h1>
<time class="basic-date" datetime="2026-05-28T12:27:19+03:00">28 мая</time></body></html>`

// habrListingHTML is what u.habr.com/fq3n5 ("Больше вакансий") resolves to: the vacancies
// index, which carries no single JobPosting.
const habrListingHTML = `<html><head><title>Вакансии</title></head><body></body></html>`

func TestHabrCareerResolvesShortLinkToVacancy(t *testing.T) {
	const short = "https://u.habr.com/PnBO7"
	const final = "https://career.habr.com/vacancies/1000166712?utm_source=telegram_career"
	c := (&fakeClient{}).route("u.habr.com/PnBO7", habrVacancyHTML, final)

	job, ok, err := NewHabrCareer(c).Resolve(context.Background(), short)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want the vacancy resolved")
	}
	// Habr is boardless, so the ingest pipeline namespaces its external_id with an empty board
	// (":<id>"); the link-source must produce the same key to dedup against the board crawl.
	if job.ExternalID != ":1000166712" {
		t.Errorf("ExternalID = %q, want :1000166712 (boardless namespace)", job.ExternalID)
	}
	if job.URL != "https://career.habr.com/vacancies/1000166712" {
		t.Errorf("URL = %q, want canonical vacancy URL without utm", job.URL)
	}
	if !strings.Contains(job.Title, "Junior Product Manager") {
		t.Errorf("Title = %q", job.Title)
	}
	if job.Company != "СберЗдоровье" {
		t.Errorf("Company = %q, want СберЗдоровье", job.Company)
	}
	if job.Location != "Москва" {
		t.Errorf("Location = %q, want Москва", job.Location)
	}
	if strings.Contains(job.Description, "<script>") || !strings.Contains(job.Description, "<li>REST</li>") {
		t.Errorf("Description not sanitized/decoded: %q", job.Description)
	}
	// From the <time datetime> (12:27:19+03:00 → 09:27:19 UTC), NOT the bogus ld+json
	// datePosted (2026-07-14).
	if job.PostedAt == nil || !job.PostedAt.Equal(time.Date(2026, 5, 28, 9, 27, 19, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-05-28T09:27:19Z from <time>, not ld+json datePosted", job.PostedAt)
	}
}

func TestHabrCareerResolvesDirectVacancyLink(t *testing.T) {
	const direct = "https://career.habr.com/vacancies/1000166712"
	c := (&fakeClient{}).route("career.habr.com/vacancies/1000166712", habrVacancyHTML, "")

	job, ok, err := NewHabrCareer(c).Resolve(context.Background(), direct)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok || job.ExternalID != ":1000166712" {
		t.Fatalf("direct link not resolved: ok=%v id=%q", ok, job.ExternalID)
	}
}

func TestHabrCareerRejectsRedirectToForeignHost(t *testing.T) {
	// The shortener is untrusted and the client follows redirects to any host. A code that
	// resolves off-Habr (even to a vacancy-shaped path) must not be parsed/ingested.
	const short = "https://u.habr.com/EVIL"
	const final = "https://169.254.169.254/vacancies/123"
	c := (&fakeClient{}).route("u.habr.com/EVIL", habrVacancyHTML, final)

	job, ok, err := NewHabrCareer(c).Resolve(context.Background(), short)
	if err == nil {
		t.Fatalf("want error for a redirect off career.habr.com; got job %+v", job)
	}
	if ok {
		t.Errorf("ok=true, want false for a foreign-host redirect")
	}
}

func TestHabrCareerSkipsNonVacancyLink(t *testing.T) {
	// "Больше вакансий" 301s to the vacancies index — matched host, but not a single
	// vacancy, so it is skipped (ok=false), not an error.
	const short = "https://u.habr.com/fq3n5"
	const final = "https://career.habr.com/vacancies"
	c := (&fakeClient{}).route("u.habr.com/fq3n5", habrListingHTML, final)

	job, ok, err := NewHabrCareer(c).Resolve(context.Background(), short)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ok {
		t.Errorf("ok=true, want skip for a non-vacancy link; got job %+v", job)
	}
}
