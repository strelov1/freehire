package sources

import (
	"context"
	"errors"
	"testing"
)

func airswiftSitemapXML(locs ...string) string {
	s := `<?xml version="1.0" encoding="UTF-8"?><urlset>`
	for _, l := range locs {
		s += `<url><loc>` + l + `</loc></url>`
	}
	return s + `</urlset>`
}

func airswiftDetailHTML(title, description, datePosted, locality, country string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + description + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"hiringOrganization":{"@type":"Organization","name":"Airswift"},` +
		`"jobLocation":[{"@type":"Place","address":{"@type":"PostalAddress",` +
		`"addressLocality":"` + locality + `","addressCountry":"` + country + `"}}]}` +
		`</script></head><body></body></html>`
}

func airswiftExpiredHTML() string {
	return `<html><body><div class="c-jobs-article-expired o-content-editor o-text-centre">` +
		`<p>Thank you for your interest in this role, but we are no longer accepting applicants.</p>` +
		`</div></body></html>`
}

func TestAirswiftFetchSitemapThenDetailAndMaps(t *testing.T) {
	jobURL := "https://www.airswift.com/jobs/snr-metering-engineer-1279815"
	detail := airswiftDetailHTML("Snr Metering Engineer", "<p>Lead metering.</p>",
		"2026-10-01", "Doha", "Qatar")

	fake := (&routedHTTP{}).
		route("sitemap.xml", airswiftSitemapXML(
			jobURL,
			"https://www.airswift.com/",
			"https://www.airswift.com/about-us",
		)).
		route("/jobs/snr-metering-engineer-1279815", detail)

	jobs, err := NewAirswift(fake, fake).Fetch(context.Background(), CompanyEntry{
		Company: "Airswift", Provider: "airswift",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (non-job sitemap entries excluded)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "1279815" {
		t.Errorf("ExternalID = %q, want 1279815", j.ExternalID)
	}
	if j.Title != "Snr Metering Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Airswift" {
		t.Errorf("Company = %q, want Airswift", j.Company)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Location != "Doha, Qatar" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-01" {
		t.Errorf("PostedAt = %v, want 2026-10-01", j.PostedAt)
	}
}

func TestAirswiftBrokenSitemapErrorsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("sitemap.xml", errors.New("origin down"))
	_, err := NewAirswift(fake, fake).Fetch(context.Background(), CompanyEntry{
		Company: "Airswift", Provider: "airswift",
	})
	if err == nil {
		t.Fatal("Fetch: want error on a broken sitemap, got nil")
	}
}

func TestAirswiftExpiredPostingIsDroppedNotUnreadable(t *testing.T) {
	jobURL := "https://www.airswift.com/jobs/qc-inspector-paint-coating-1277274"
	fake := (&routedHTTP{}).route("/jobs/qc-inspector-paint-coating-1277274", airswiftExpiredHTML())

	j, ok := airswift{sitemap: fake, pages: fake}.detail(context.Background(),
		CompanyEntry{Company: "Airswift"}, jobURL)
	if ok {
		t.Fatalf("detail returned ok=true for an expired posting, want dropped (ok=false); got %+v", j)
	}
}

func TestAirswiftUnexpectedMissingJobPostingIsUnreadable(t *testing.T) {
	jobURL := "https://www.airswift.com/jobs/mystery-role-1234567"
	fake := (&routedHTTP{}).route("/jobs/mystery-role-1234567", "<html><body>no ld+json here</body></html>")

	j, ok := airswift{sitemap: fake, pages: fake}.detail(context.Background(),
		CompanyEntry{Company: "Airswift"}, jobURL)
	if !ok {
		t.Fatal("detail returned ok=false, want an unreadableDetail stub")
	}
	if !j.Unreadable {
		t.Error("Unreadable = false, want true (no expired marker, no JobPosting)")
	}
	if j.Company != "Airswift" {
		t.Errorf("Company = %q, want the configured fallback Airswift", j.Company)
	}
}

func TestAirswiftGoneDetailDropsThePosting(t *testing.T) {
	goodURL := "https://www.airswift.com/jobs/good-job-1111111"
	goneURL := "https://www.airswift.com/jobs/gone-job-2222222"
	good := airswiftDetailHTML("Good Job", "d", "2026-10-02", "Perth", "Australia")

	fake := (&routedHTTP{}).
		route("sitemap.xml", airswiftSitemapXML(goodURL, goneURL)).
		route("/jobs/good-job-1111111", good).
		routeErr("/jobs/gone-job-2222222", &StatusError{Method: "GET", Code: 404, URL: goneURL})

	jobs, err := NewAirswift(fake, fake).Fetch(context.Background(), CompanyEntry{
		Company: "Airswift", Provider: "airswift",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1111111" {
		t.Fatalf("got %v, want only the good posting — the 404'd one dropped, not marked", jobs)
	}
}

func TestAirswiftOtherFetchErrorIsUnreadable(t *testing.T) {
	goodURL := "https://www.airswift.com/jobs/good-job-3333333"
	badURL := "https://www.airswift.com/jobs/bad-job-4444444"
	good := airswiftDetailHTML("Good Job", "d", "2026-10-02", "Perth", "Australia")

	fake := (&routedHTTP{}).
		route("sitemap.xml", airswiftSitemapXML(goodURL, badURL)).
		route("/jobs/good-job-3333333", good).
		routeErr("/jobs/bad-job-4444444", errors.New("timeout"))

	jobs, err := NewAirswift(fake, fake).Fetch(context.Background(), CompanyEntry{
		Company: "Airswift", Provider: "airswift",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	markers := unreadableMarkers(jobs)
	if len(markers) != 1 || markers[0].ExternalID != "4444444" {
		t.Fatalf("unreadable markers = %v, want one for the posting whose fetch errored", markers)
	}
}

func TestAirswiftJobID(t *testing.T) {
	cases := map[string]string{
		"https://www.airswift.com/jobs/snr-metering-engineer-1279815": "1279815",
		"https://www.airswift.com/jobs/qc-inspector-paint-1277274/":   "1277274",
		"https://www.airswift.com/":                                   "",
		"https://www.airswift.com/about-us":                           "",
		"https://www.airswift.com/jobs/":                              "",
	}
	for u, want := range cases {
		if got := airswiftJobID(u); got != want {
			t.Errorf("airswiftJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestAirswiftProviderAndBoardless(t *testing.T) {
	s := NewAirswift(nil, nil)
	if s.Provider() != "airswift" {
		t.Errorf("Provider() = %q, want airswift", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("airswift must implement boardless")
	}
}
