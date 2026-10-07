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

	jobs, err := NewEnergyJobline(fake, fake).Fetch(context.Background(), CompanyEntry{
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
	_, err := NewEnergyJobline(fake, fake).Fetch(context.Background(), CompanyEntry{
		Company: "EnergyJobline", Provider: "energyjobline",
	})
	if err == nil {
		t.Fatal("Fetch: want error on a broken sitemap index, got nil")
	}
}

// The index itself can resolve fine while one of its sub-sitemaps fails; that failure
// propagates as a hard Fetch error too (no fullBoardListing is claimed here, so any shard
// failure means the enumeration is incomplete and the caller should not trust a partial
// result as "today's whole catalogue").
func TestEnergyjoblineSubSitemapFailureFailsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).
		route("sitemap.xml?page=1", energyjoblineURLSetXML("https://www.energyjobline.com/job/ok-1")).
		route("sitemap.xml", energyjoblineSitemapIndexXML(
			"https://www.energyjobline.com/sitemap.xml?page=1",
			"https://www.energyjobline.com/sitemap.xml?page=2",
		)).
		routeErr("sitemap.xml?page=2", errors.New("origin down"))

	_, err := NewEnergyJobline(fake, fake).Fetch(context.Background(), CompanyEntry{
		Company: "EnergyJobline", Provider: "energyjobline",
	})
	if err == nil {
		t.Fatal("Fetch: want error when one sub-sitemap fails, got nil")
	}
}

func TestEnergyjoblineCompanyComesFromThePosting(t *testing.T) {
	jobURL := "https://www.energyjobline.com/job/welder-kent-27543774"
	detail := energyjoblineDetailHTML("WELDER in Kent", "Energy Jobline ZR",
		"desc", "2026-10-02", "Kent", "GB")
	fake := (&routedHTTP{}).route("/job/welder-kent-27543774", detail)

	j, ok := energyjobline{sitemap: fake, pages: fake}.detail(context.Background(),
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

	j, ok := energyjobline{sitemap: fake, pages: fake}.detail(context.Background(),
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

	j, ok := energyjobline{sitemap: fake, pages: fake}.detail(context.Background(),
		CompanyEntry{Company: "EnergyJobline", Provider: "energyjobline"}, jobURL)
	if !ok {
		t.Fatal("detail returned ok=false, want an unreadableDetail stub")
	}
	if !j.Unreadable {
		t.Error("Unreadable = false, want true (no JobPosting block)")
	}
}

// A posting can carry a JobPosting block with no jobLocation entries at all (the field is
// an array in the markup, but nothing guarantees the site always populates it) — this must
// not panic on an empty-slice index and should simply leave Location blank.
func TestEnergyjoblineDetailWithNoJobLocation(t *testing.T) {
	jobURL := "https://www.energyjobline.com/job/no-location-66666666"
	html := `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting",` +
		`"title":"Remote Role","hiringOrganization":{"@type":"Organization","name":"Real Co"},` +
		`"description":"d","datePosted":"2026-10-02","jobLocation":[]}` +
		`</script></head><body></body></html>`
	fake := (&routedHTTP{}).route("/job/no-location-66666666", html)

	j, ok := energyjobline{sitemap: fake, pages: fake}.detail(context.Background(),
		CompanyEntry{Company: "EnergyJobline", Provider: "energyjobline"}, jobURL)
	if !ok {
		t.Fatal("detail returned ok=false")
	}
	if j.Location != "" {
		t.Errorf("Location = %q, want empty (no jobLocation entries)", j.Location)
	}
	if j.Company != "Real Co" {
		t.Errorf("Company = %q, want Real Co", j.Company)
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

	jobs, err := NewEnergyJobline(fake, fake).Fetch(context.Background(), CompanyEntry{
		Company: "EnergyJobline", Provider: "energyjobline",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// A plain transport error states nothing about the posting (it might be a timeout, a
	// transient 5xx — anything short of a confirmed-gone answer), so it must not be
	// mistaken for the posting having been removed: it survives as an Unreadable stub, the
	// same contract fetchDetails documents and bayt.go/gulftalent.go already honour.
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (the good posting plus an Unreadable stub for the one that failed to fetch)", len(jobs))
	}
	markers := unreadableMarkers(jobs)
	if len(markers) != 1 || markers[0].ExternalID != "33333333" {
		t.Fatalf("unreadable markers = %v, want one for the posting whose detail fetch errored", markers)
	}
	if markers[0].Company != "EnergyJobline" {
		t.Errorf("marker Company = %q, want the entry's configured company", markers[0].Company)
	}
	for _, j := range jobs {
		if j.ExternalID == "22222222" && j.Title != "Good Job" {
			t.Errorf("good posting Title = %q, want Good Job", j.Title)
		}
	}
}

// The other half of the distinction (see bayt_test.go's TestBaytGoneDetailDropsThePosting):
// a 404/410 is the platform's own answer that the posting is gone, so the crawl drops it
// rather than marking it unreadable.
func TestEnergyjoblineGoneDetailDropsThePosting(t *testing.T) {
	goodURL := "https://www.energyjobline.com/job/good-job-44444444"
	goneURL := "https://www.energyjobline.com/job/gone-job-55555555"
	good := energyjoblineDetailHTML("Good Job", "Real Co", "d", "2026-10-02", "Kent", "GB")

	fake := (&routedHTTP{}).
		route("sitemap.xml?page=1", energyjoblineURLSetXML(goodURL, goneURL)).
		route("sitemap.xml", energyjoblineSitemapIndexXML("https://www.energyjobline.com/sitemap.xml?page=1")).
		route("/job/good-job-44444444", good).
		routeErr("/job/gone-job-55555555", &StatusError{Method: "GET", Code: 404, URL: goneURL})

	jobs, err := NewEnergyJobline(fake, fake).Fetch(context.Background(), CompanyEntry{
		Company: "EnergyJobline", Provider: "energyjobline",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "44444444" {
		t.Fatalf("got %v, want only the posting whose detail answered — the 404'd one dropped, not marked", jobs)
	}
}

func TestEnergyjoblineProviderAndBoardless(t *testing.T) {
	s := NewEnergyJobline(nil, nil)
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
