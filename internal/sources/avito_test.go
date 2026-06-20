package sources

import (
	"context"
	"strings"
	"testing"
)

// avitoSitemapIndex is a sitemap index referencing one vacancy sub-sitemap and one
// non-vacancy sub-sitemap (landing/category pages), mirroring career.avito.com's real
// sitemap.xml shape.
const avitoSitemapIndex = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>https://career.avito.com/sitemap-iblock-2.xml</loc></sitemap>
<sitemap><loc>https://career.avito.com/sitemap-iblock-9.xml</loc></sitemap>
</sitemapindex>`

// avitoVacancySitemap builds a <urlset> sub-sitemap from the given locs.
func avitoVacancySitemap(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

// avitoDetail is a vacancy page: the full <title> (which carries the display city as a
// "в городе <city>" suffix), the JobPosting ld+json job title, and the ld+json
// addressLocality (which Avito always reports as the HQ "Москва", even for remote roles).
// The description embeds a <script> (written <\/script> so the JSON string carries it)
// that sanitizeHTML must strip.
func avitoDetail(fullTitle, jobTitle, ldCity, datePosted string) string {
	return `<html><head><title>` + fullTitle + `</title></head><body>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"` + jobTitle + `",
"description":"<p>Обязанности</p><script>alert(1)<\/script>",
"datePosted":"` + datePosted + `",
"identifier":{"@type":"PropertyValue","name":"Продажи"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"` + ldCity + `","addressCountry":"RU"}}}
</script></body></html>`
}

func TestAvitoProvider(t *testing.T) {
	if got := NewAvito(nil).Provider(); got != "avito" {
		t.Errorf("Provider() = %q, want %q", got, "avito")
	}
}

func TestAvitoIsBoardless(t *testing.T) {
	if _, ok := NewAvito(nil).(boardless); !ok {
		t.Error("avito should implement the boardless marker")
	}
}

func TestAvitoVacancyID(t *testing.T) {
	cases := map[string]string{
		"https://career.avito.com/vacancies/prodazhi/19548/":   "19548",
		"https://career.avito.com/vacancies/razrabotka/17810":  "17810",
		"https://career.avito.com/vacancies/dizayn/":           "", // category, no id
		"https://career.avito.com/vacancies/":                  "",
		"https://career.avito.com/teams/auto/":                 "",
		"https://career.avito.com/vacancies/prodazhi/19548/?x": "19548",
	}
	for loc, want := range cases {
		if got := avitoVacancyID(loc); got != want {
			t.Errorf("avitoVacancyID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestAvitoFetchEnumeratesDedupsAndMaps(t *testing.T) {
	remoteURL := "https://career.avito.com/vacancies/prodazhi/19548/"
	onsiteURL := "https://career.avito.com/vacancies/razrabotka/17810/"
	dupURL := "https://career.avito.com/vacancies/marketing/19548/" // same id as remote → dedup
	catURL := "https://career.avito.com/vacancies/dizayn/"          // no id → skipped

	fake := (&routedHTTP{}).
		route("/sitemap.xml", avitoSitemapIndex).
		route("sitemap-iblock-2.xml", avitoVacancySitemap(remoteURL, onsiteURL, dupURL, catURL)).
		route("sitemap-iblock-9.xml", avitoVacancySitemap("https://career.avito.com/directions/ux/")).
		route("prodazhi/19548", avitoDetail(
			"Вакансия Авито «Менеджер по работе с клиентами» в городе Удалённая работа",
			"Менеджер по работе с клиентами", "Москва", "2026-05-15T16:24:19+03:00")).
		route("razrabotka/17810", avitoDetail(
			"Вакансия Авито «Backend-разработчик» в городе Москва",
			"Backend-разработчик", "Москва", "2026-05-10T09:00:00+03:00"))

	jobs, err := NewAvito(fake).Fetch(context.Background(), CompanyEntry{Company: "Avito", Provider: "avito"})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (dedup by id, category skipped)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	remote, ok := byID["19548"]
	if !ok {
		t.Fatal("vacancy 19548 missing")
	}
	if remote.URL != remoteURL {
		t.Errorf("URL = %q, want %q (first-seen wins over the marketing dup)", remote.URL, remoteURL)
	}
	if remote.Company != "Avito" {
		t.Errorf("Company = %q, want Avito", remote.Company)
	}
	if remote.Title != "Менеджер по работе с клиентами" {
		t.Errorf("Title = %q, want the ld+json job title", remote.Title)
	}
	if remote.Location != "Удалённая работа" {
		t.Errorf("Location = %q, want %q (from <title>, not the ld+json Москва)", remote.Location, "Удалённая работа")
	}
	if !remote.Remote {
		t.Error("remote role should be flagged Remote despite ld+json city Москва")
	}
	if !strings.Contains(remote.Description, "Обязанности") {
		t.Errorf("Description = %q, want it to keep body text", remote.Description)
	}
	if strings.Contains(remote.Description, "alert") {
		t.Errorf("Description = %q, want the <script> stripped", remote.Description)
	}
	if remote.PostedAt == nil || remote.PostedAt.UTC().Format("2006-01-02T15:04:05Z") != "2026-05-15T13:24:19Z" {
		t.Errorf("PostedAt = %v, want 2026-05-15T13:24:19Z", remote.PostedAt)
	}

	onsite, ok := byID["17810"]
	if !ok {
		t.Fatal("vacancy 17810 missing")
	}
	if onsite.Location != "Москва" {
		t.Errorf("Location = %q, want Москва", onsite.Location)
	}
	if onsite.Remote {
		t.Error("on-site Moscow role should not be flagged Remote")
	}
}

func TestAvitoLocationFallsBackToAddressLocality(t *testing.T) {
	loc := "https://career.avito.com/vacancies/prodazhi/30001/"
	fake := (&routedHTTP{}).
		route("/sitemap.xml", avitoSitemapIndex).
		route("sitemap-iblock-2.xml", avitoVacancySitemap(loc)).
		route("sitemap-iblock-9.xml", avitoVacancySitemap()).
		// <title> carries no "в городе" suffix → location comes from ld+json addressLocality.
		route("prodazhi/30001", avitoDetail("Аналитик данных — Авито", "Аналитик данных", "Казань", "2026-04-01T09:00:00+03:00"))

	jobs, err := NewAvito(fake).Fetch(context.Background(), CompanyEntry{Company: "Avito", Provider: "avito"})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if jobs[0].Location != "Казань" {
		t.Errorf("Location = %q, want Казань (addressLocality fallback)", jobs[0].Location)
	}
}

func TestAvitoFetchSitemapIndexErrorIsBoardError(t *testing.T) {
	fake := &routedHTTP{} // no routes → index fetch fails
	if _, err := NewAvito(fake).Fetch(context.Background(), CompanyEntry{Company: "Avito", Provider: "avito"}); err == nil {
		t.Fatal("Fetch() error = nil, want a board error when the sitemap index fails")
	}
}

func TestAvitoFetchSkipsFailedDetailAndMissingJobPosting(t *testing.T) {
	good := "https://career.avito.com/vacancies/prodazhi/40001/"
	noLD := "https://career.avito.com/vacancies/prodazhi/40002/"
	dead := "https://career.avito.com/vacancies/prodazhi/40003/"

	fake := (&routedHTTP{}).
		route("/sitemap.xml", avitoSitemapIndex).
		route("sitemap-iblock-2.xml", avitoVacancySitemap(good, noLD, dead)).
		route("sitemap-iblock-9.xml", avitoVacancySitemap()).
		route("prodazhi/40001", avitoDetail("Вакансия Авито «Kept» в городе Москва", "Kept", "Москва", "2026-04-01T09:00:00+03:00")).
		route("prodazhi/40002", `<html><head><title>No posting</title></head><body>nothing</body></html>`)
	// 40003 has no route → GetHTML fails for it.

	jobs, err := NewAvito(fake).Fetch(context.Background(), CompanyEntry{Company: "Avito", Provider: "avito"})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(jobs) != 1 || jobs[0].Title != "Kept" {
		t.Fatalf("jobs = %+v, want only the one vacancy with a JobPosting", jobs)
	}
}
