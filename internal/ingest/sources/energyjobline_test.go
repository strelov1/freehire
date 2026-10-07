package sources

import (
	"context"
	"errors"
	"testing"
)

func energyjoblineSitemapIndexXML(subs ...string) string {
	s := `<?xml version="1.0" encoding="UTF-8"?><sitemapindex>`
	for _, sm := range subs {
		s += `<sitemap><loc>` + sm + `</loc></sitemap>`
	}
	return s + `</sitemapindex>`
}

func energyjoblineURLSetXML(locs ...string) string {
	s := `<?xml version="1.0" encoding="UTF-8"?><urlset>`
	for _, l := range locs {
		s += `<url><loc>` + l + `</loc></url>`
	}
	return s + `</urlset>`
}

func energyjoblineDetailHTML(title, company, description, datePosted, locality, country string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"WebSite","name":"Energy Jobline"}` +
		`</script><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"Organization","name":"Energy Jobline"}` +
		`</script><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"hiringOrganization":{"@type":"Organization","name":"` + company + `"},` +
		`"description":"` + description + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"jobLocation":[{"@type":"Place","address":{"@type":"PostalAddress",` +
		`"addressLocality":"` + locality + `","addressCountry":"` + country + `"}}]}` +
		`</script></head><body></body></html>`
}

func TestEnergyjoblineFetchWalksIndexThenDetailAndMaps(t *testing.T) {
	jobURL := "https://www.energyjobline.com/job/controls-engineer-atlanta-31835232"
	detail := energyjoblineDetailHTML(
		"Controls Engineer in Atlanta", "The AES Corporation",
		"Field service controls engineering.", "2026-10-01", "Atlanta", "US")

	fake := (&routedHTTP{}).
		route("sitemap.xml?page=1", energyjoblineURLSetXML(
			jobURL,
			"https://www.energyjobline.com/",     // site root, not a job
			"https://www.energyjobline.com/jobs", // listing root, not a job
			"https://www.energyjobline.com/news-article/oil-and-gas/some-story", // unrelated
		)).
		route("sitemap.xml?page=2", energyjoblineURLSetXML()).
		// Registered after the page-specific routes: routedHTTP matches by substring in
		// append order, and "sitemap.xml" is itself a substring of "sitemap.xml?page=1" —
		// this generic route must lose that race for the index URL to resolve correctly.
		route("sitemap.xml", energyjoblineSitemapIndexXML(
			"https://www.energyjobline.com/sitemap.xml?page=1",
			"https://www.energyjobline.com/sitemap.xml?page=2",
		)).
		route("/job/controls-engineer-atlanta-31835232", detail)

	jobs, err := NewEnergyJobline(fake).Fetch(context.Background(), CompanyEntry{
		Company: "EnergyJobline", Provider: "energyjobline",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (non-job sitemap entries must be excluded)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "31835232" {
		t.Errorf("ExternalID = %q, want 31835232", j.ExternalID)
	}
	if j.Title != "Controls Engineer in Atlanta" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "The AES Corporation" {
		t.Errorf("Company = %q, want the posting's own hiringOrganization", j.Company)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Location != "Atlanta, US" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-01" {
		t.Errorf("PostedAt = %v, want 2026-10-01", j.PostedAt)
	}
}

func TestEnergyjoblineBrokenSitemapIndexErrorsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("sitemap.xml", errors.New("origin down"))
	_, err := NewEnergyJobline(fake).Fetch(context.Background(), CompanyEntry{
		Company: "EnergyJobline", Provider: "energyjobline",
	})
	if err == nil {
		t.Fatal("Fetch: want error on a broken sitemap index, got nil")
	}
}

func TestEnergyjoblineCompanyComesFromThePosting(t *testing.T) {
	jobURL := "https://www.energyjobline.com/job/welder-kent-27543774"
	detail := energyjoblineDetailHTML("WELDER in Kent", "Energy Jobline ZR",
		"desc", "2026-10-02", "Kent", "GB")
	fake := (&routedHTTP{}).route("/job/welder-kent-27543774", detail)

	j, ok := energyjobline{http: fake}.detail(context.Background(),
		CompanyEntry{Company: "EnergyJobline", Provider: "energyjobline"}, jobURL)
	if !ok {
		t.Fatal("detail returned ok=false")
	}
	// The site attributes agency-submitted postings to the agency's own brand, not a
	// confidential end client — that is the real, honest value of hiringOrganization for
	// this posting (verified against the live site's own human-visible company link, not
	// just its ld+json), so it is stored as-is rather than filtered or guessed around.
	if j.Company != "Energy Jobline ZR" {
		t.Errorf("Company = %q, want the posting's own (agency) hiringOrganization", j.Company)
	}
}

func TestEnergyjoblineUnreadableDetailWhenHiringOrgEmpty(t *testing.T) {
	jobURL := "https://www.energyjobline.com/job/no-employer-99999999"
	detail := energyjoblineDetailHTML("Mystery Role", "", "desc", "2026-10-02", "Kent", "GB")
	fake := (&routedHTTP{}).route("/job/no-employer-99999999", detail)

	j, ok := energyjobline{http: fake}.detail(context.Background(),
		CompanyEntry{Company: "EnergyJobline", Provider: "energyjobline"}, jobURL)
	if !ok {
		t.Fatal("detail returned ok=false, want an unreadableDetail stub")
	}
	if !j.Unreadable {
		t.Error("Unreadable = false, want true (empty hiringOrganization)")
	}
	if j.Company != "EnergyJobline" {
		t.Errorf("Company = %q, want the configured fallback EnergyJobline", j.Company)
	}
}

func TestEnergyjoblineUnreadableDetailWhenNoJobPosting(t *testing.T) {
	jobURL := "https://www.energyjobline.com/job/broken-page-11111111"
	fake := (&routedHTTP{}).route("/job/broken-page-11111111", "<html><body>no ld+json here</body></html>")

	j, ok := energyjobline{http: fake}.detail(context.Background(),
		CompanyEntry{Company: "EnergyJobline", Provider: "energyjobline"}, jobURL)
	if !ok {
		t.Fatal("detail returned ok=false, want an unreadableDetail stub")
	}
	if !j.Unreadable {
		t.Error("Unreadable = false, want true (no JobPosting block)")
	}
}

func TestEnergyjoblineJobID(t *testing.T) {
	cases := map[string]string{
		"https://www.energyjobline.com/job/controls-engineer-atlanta-31835232": "31835232",
		"https://www.energyjobline.com/job/welder-kent-27543774/":              "27543774",
		"https://www.energyjobline.com/":                                       "",
		"https://www.energyjobline.com/jobs":                                   "",
		"https://www.energyjobline.com/news-article/oil-and-gas/story":         "",
		"https://www.energyjobline.com/company/energy-jobline-o-p":             "",
	}
	for u, want := range cases {
		if got := energyjoblineJobID(u); got != want {
			t.Errorf("energyjoblineJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestEnergyjoblineOneBadDetailDoesNotFailTheCrawl(t *testing.T) {
	goodURL := "https://www.energyjobline.com/job/good-job-22222222"
	badURL := "https://www.energyjobline.com/job/bad-job-33333333"
	good := energyjoblineDetailHTML("Good Job", "Real Co", "d", "2026-10-02", "Kent", "GB")

	fake := (&routedHTTP{}).
		route("sitemap.xml?page=1", energyjoblineURLSetXML(goodURL, badURL)).
		route("sitemap.xml", energyjoblineSitemapIndexXML("https://www.energyjobline.com/sitemap.xml?page=1")).
		route("/job/good-job-22222222", good).
		routeErr("/job/bad-job-33333333", errors.New("fetch failed"))

	jobs, err := NewEnergyJobline(fake).Fetch(context.Background(), CompanyEntry{
		Company: "EnergyJobline", Provider: "energyjobline",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the one good posting, bad one dropped silently on fetch error)", len(jobs))
	}
	if jobs[0].Title != "Good Job" {
		t.Errorf("Title = %q, want Good Job", jobs[0].Title)
	}
}

func TestEnergyjoblineProviderAndBoardless(t *testing.T) {
	s := NewEnergyJobline(nil)
	if s.Provider() != "energyjobline" {
		t.Errorf("Provider() = %q, want energyjobline", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("energyjobline must implement boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("energyjobline must implement aggregator")
	}
}
