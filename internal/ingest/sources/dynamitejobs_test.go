package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const dynamiteBase = "https://dynamitejobs.com"

func dynamiteIndexXML() string {
	return `<?xml version="1.0"?><sitemapindex>` +
		`<sitemap><loc>` + dynamiteBase + `/sitemap/site.xml</loc></sitemap>` +
		`<sitemap><loc>` + dynamiteBase + `/sitemap/jobs-of-company.xml</loc></sitemap>` +
		`<sitemap><loc>` + dynamiteBase + `/sitemap/jobs-of-category.xml</loc></sitemap>` +
		`</sitemapindex>`
}

func dynamiteURLSet(locs ...string) string {
	s := `<?xml version="1.0"?><urlset>`
	for _, l := range locs {
		s += `<url><loc>` + l + `</loc></url>`
	}
	return s + `</urlset>`
}

func dynamiteDetailHTML(title, company, countriesJSON, employmentType string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[]}` +
		`</script><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting",` +
		`"applicantLocationRequirements":` + countriesJSON + `,` +
		`"datePosted":"2026-11-16T00:28:21.240Z",` +
		`"description":"<h3>About</h3><p>Sell &amp; grow.</p>",` +
		`"employmentType":"` + employmentType + `",` +
		`"hiringOrganization":{"@type":"Organization","name":"` + company + `"},` +
		`"jobLocationType":"TELECOMMUTE","title":"` + title + `"}` +
		`</script></head><body></body></html>`
}

const (
	dynamiteJobA = dynamiteBase + "/company/acme/remote-job/account-executive"
	dynamiteJobB = dynamiteBase + "/company/globex/remote-job/support-lead"
)

func dynamiteFake() *routedHTTP {
	return (&routedHTTP{}).
		route("jobs-of-company.xml", dynamiteURLSet(
			dynamiteJobA,
			dynamiteBase+"/skill/remote-b2b-sales-jobs",      // not a posting
			dynamiteBase+"/company/acme",                     // company page
			dynamiteBase+"/remote-jobs/business-development", // category page
		)).
		route("jobs-of-category.xml", dynamiteURLSet(dynamiteJobA, dynamiteJobB)).
		route("site.xml", dynamiteURLSet(dynamiteBase+"/enter/login")).
		route("/sitemap/index.xml", dynamiteIndexXML()).
		route("/company/acme/remote-job/account-executive", dynamiteDetailHTML(
			"Account Executive (SaaS / B2B), Remote Job", "Acme Inc",
			`[{"@type":"Country","name":"United States"}]`, "FULL_TIME")).
		route("/company/globex/remote-job/support-lead", dynamiteDetailHTML(
			"Support Lead, Remote Job", "Globex",
			`[{"@type":"Country","name":"Poland"},{"@type":"Country","name":"Portugal"},`+
				`{"@type":"Country","name":"Spain"},{"@type":"Country","name":"France"},`+
				`{"@type":"Country","name":"Germany"},{"@type":"Country","name":"Italy"}]`, "PART_TIME"))
}

func TestDynamiteJobsFetchMapsPostings(t *testing.T) {
	fake := dynamiteFake()
	jobs, err := NewDynamiteJobs(fake, fake).Fetch(context.Background(), CompanyEntry{Company: "Dynamite Jobs"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (non-posting pages excluded, repeats deduplicated)", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	a, ok := byID["acme/account-executive"]
	if !ok {
		t.Fatalf("missing acme/account-executive in %v", byID)
	}
	if a.Title != "Account Executive (SaaS / B2B)" {
		t.Errorf("Title = %q, want the ', Remote Job' suffix dropped", a.Title)
	}
	if a.Company != "Acme Inc" || a.URL != dynamiteJobA {
		t.Errorf("Company/URL = %q / %q", a.Company, a.URL)
	}
	if !a.Remote || a.WorkMode != "remote" {
		t.Errorf("Remote/WorkMode = %v / %q", a.Remote, a.WorkMode)
	}
	if a.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", a.EmploymentType)
	}
	if a.Location != "Remote - United States" {
		t.Errorf("Location = %q", a.Location)
	}
	if len(a.Countries) != 1 || a.Countries[0] != "us" {
		t.Errorf("Countries = %v, want [us]", a.Countries)
	}
	if a.PostedAt == nil || a.PostedAt.Format("2006-01-02") != "2026-11-16" {
		t.Errorf("PostedAt = %v", a.PostedAt)
	}
	if !strings.Contains(a.Description, "Sell &amp; grow") {
		t.Errorf("Description = %q", a.Description)
	}
	if a.IsTechHint {
		t.Error("IsTechHint set: the board is not technical-only")
	}

	b := byID["globex/support-lead"]
	if b.Location != "Remote" {
		t.Errorf("Location = %q, want bare Remote for a region-preset sized list", b.Location)
	}
	if len(b.Countries) != 6 {
		t.Errorf("Countries = %v, want all six", b.Countries)
	}
	if b.EmploymentType != "part_time" {
		t.Errorf("EmploymentType = %q", b.EmploymentType)
	}
}

func TestDynamiteJobsFetchNewSkipsSeenPostings(t *testing.T) {
	fake := dynamiteFake()
	seen := func(id string) bool { return id == "acme/account-executive" }
	jobs, err := NewDynamiteJobs(fake, fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Company: "Dynamite Jobs"}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var refreshed, hydrated int
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
		} else {
			hydrated++
		}
	}
	if refreshed != 1 || hydrated != 1 {
		t.Errorf("refreshed=%d hydrated=%d, want 1 and 1", refreshed, hydrated)
	}
}

func TestDynamiteJobsBudgetDefersTheRest(t *testing.T) {
	fake := dynamiteFake()
	s := dynamitejobs{sitemap: fake, pages: fake, maxNewPerRun: 1}
	jobs, err := s.Fetch(context.Background(), CompanyEntry{Company: "Dynamite Jobs"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("got %d jobs, want 1 under a budget of 1", len(jobs))
	}
}

func TestDynamiteJobsSitemapFailureFailsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("sitemap/index.xml", errors.New("origin down"))
	if _, err := NewDynamiteJobs(fake, fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error on a broken sitemap index")
	}
}

func TestDynamiteJobsReadingNoneOfWhatItListedFails(t *testing.T) {
	fake := (&routedHTTP{}).
		route("jobs-of-company.xml", dynamiteURLSet(dynamiteJobA)).
		route("/sitemap/index.xml", `<sitemapindex><sitemap><loc>`+dynamiteBase+`/sitemap/jobs-of-company.xml</loc></sitemap></sitemapindex>`).
		routeErr("/company/acme/", &StatusError{Code: 404})
	// A 404 is the site's own "gone": nothing to read, nothing wrong.
	if _, err := NewDynamiteJobs(fake, fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error when every listed posting dropped")
	}
}

func TestDynamiteJobsUnreadablePageIsAMarkerNotADrop(t *testing.T) {
	fake := (&routedHTTP{}).
		route("jobs-of-company.xml", dynamiteURLSet(dynamiteJobA)).
		route("/sitemap/index.xml", `<sitemapindex><sitemap><loc>`+dynamiteBase+`/sitemap/jobs-of-company.xml</loc></sitemap></sitemapindex>`).
		routeErr("/company/acme/", errors.New("timeout"))
	jobs, err := NewDynamiteJobs(fake, fake).Fetch(context.Background(), CompanyEntry{Company: "Dynamite Jobs"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].Unreadable {
		t.Fatalf("got %+v, want one Unreadable marker", jobs)
	}
}

func TestDynamiteJobsSkipsTheSitesOwnProbePosting(t *testing.T) {
	probe := dynamiteBase + "/company/djauditprobe/remote-job/pentest-probe-job"
	fake := (&routedHTTP{}).
		route("jobs-of-company.xml", dynamiteURLSet(dynamiteJobA, probe)).
		route("/sitemap/index.xml", `<sitemapindex><sitemap><loc>`+dynamiteBase+`/sitemap/jobs-of-company.xml</loc></sitemap></sitemapindex>`).
		route("/company/djauditprobe/", dynamiteDetailHTML("Pentest Probe Job, Remote Job", "DJ Audit Probe", `[]`, "")).
		route("/company/acme/", dynamiteDetailHTML("Account Executive, Remote Job", "Acme Inc", `[{"@type":"Country","name":"United States"}]`, "FULL_TIME"))
	jobs, err := NewDynamiteJobs(fake, fake).Fetch(context.Background(), CompanyEntry{Company: "Dynamite Jobs"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Company != "Acme Inc" {
		t.Fatalf("got %+v, want only the Acme posting", jobs)
	}
}

func TestDynamiteJobsJobID(t *testing.T) {
	for in, want := range map[string]string{
		dynamiteJobA:                                 "acme/account-executive",
		dynamiteJobA + "/":                           "acme/account-executive",
		dynamiteBase + "/company/acme":               "",
		dynamiteBase + "/skill/remote-b2b-jobs":      "",
		"https://example.com/company/a/remote-job/b": "",
	} {
		if got := dynamitejobsJobID(in); got != want {
			t.Errorf("dynamitejobsJobID(%q) = %q, want %q", in, got, want)
		}
	}
}
