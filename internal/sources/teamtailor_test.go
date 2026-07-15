package sources

import (
	"context"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse url %q: %v", s, err)
	}
	return u
}

// ttListingHTML builds a Teamtailor listing page linking each given job URL.
func ttListingHTML(jobURLs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><ul class="jobs">`)
	for _, u := range jobURLs {
		b.WriteString(`<li><a href="` + u + `">A job</a></li>`)
	}
	b.WriteString(`</ul></body></html>`)
	return b.String()
}

// ttDetailHTML builds a job page carrying a JobPosting ld+json block.
func ttDetailHTML(title, encodedDescription, datePosted, locality, country, locationType string) string {
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"http://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + encodedDescription + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"jobLocationType":"` + locationType + `",` +
		`"jobLocation":[{"@type":"Place","address":{"addressLocality":"` + locality + `","addressCountry":"` + country + `"}}]}` +
		`</script></head><body></body></html>`
}

func TestTTJobLinks(t *testing.T) {
	h := `<html><body>
		<a href="https://jobs.acme.com/jobs/12345-cloud-engineer">Cloud Engineer</a>
		<a href="https://jobs.acme.com/jobs/12345-cloud-engineer">Apply now</a>
		<a href="https://jobs.acme.com/jobs/678-designer">Designer</a>
		<a href="https://jobs.acme.com/departments">Departments</a>
		<a href="https://jobs.acme.com/jobs/connect">Connect</a>
	</body></html>`
	base := mustURL(t, "https://jobs.acme.com/jobs?page=1")
	got := ttJobLinks(base, parseHTML(t, h))
	want := []string{
		"https://jobs.acme.com/jobs/12345-cloud-engineer",
		"https://jobs.acme.com/jobs/678-designer",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ttJobLinks() = %v, want %v", got, want)
	}
}

func TestTTJobLinksResolvesRelativeHrefs(t *testing.T) {
	// A board that emits relative hrefs must still yield absolute, fetchable URLs —
	// otherwise the detail GET fails on a bare path and the posting silently drops.
	h := `<html><body>
		<a href="/jobs/12345-engineer">Engineer</a>
		<a href="https://jobs.acme.com/jobs/678-designer">Designer</a>
	</body></html>`
	base := mustURL(t, "https://jobs.acme.com/jobs?page=1")
	got := ttJobLinks(base, parseHTML(t, h))
	want := []string{
		"https://jobs.acme.com/jobs/12345-engineer",
		"https://jobs.acme.com/jobs/678-designer",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ttJobLinks() = %v, want %v", got, want)
	}
}

func TestTTJobLinksEmpty(t *testing.T) {
	h := `<html><body><a href="https://jobs.acme.com/departments">x</a></body></html>`
	base := mustURL(t, "https://jobs.acme.com/jobs?page=1")
	if got := ttJobLinks(base, parseHTML(t, h)); len(got) != 0 {
		t.Errorf("ttJobLinks() = %v, want empty", got)
	}
}

func TestTTJobPosting(t *testing.T) {
	h := `<html><head>
		<script type="application/ld+json">{"@type":"WebSite","name":"Careers"}</script>
		<script type="application/ld+json">{"@context":"http://schema.org/","@type":"JobPosting","title":"Cloud Engineer","description":"&lt;p&gt;Build it.&lt;/p&gt;","datePosted":"2026-06-08T00:00:00+02:00","employmentType":"FULL_TIME","jobLocationType":"TELECOMMUTE","jobLocation":[{"@type":"Place","address":{"addressLocality":"Stockholm","addressCountry":"SE"}}]}</script>
	</head><body></body></html>`
	p, ok := ttJobPosting(parseHTML(t, h))
	if !ok {
		t.Fatal("ttJobPosting() ok = false, want true")
	}
	if p.Title != "Cloud Engineer" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.DatePosted != "2026-06-08T00:00:00+02:00" {
		t.Errorf("DatePosted = %q", p.DatePosted)
	}
	if p.Description != "&lt;p&gt;Build it.&lt;/p&gt;" {
		t.Errorf("Description = %q (want still HTML-encoded)", p.Description)
	}
	if p.JobLocationType != "TELECOMMUTE" {
		t.Errorf("JobLocationType = %q", p.JobLocationType)
	}
	if len(p.JobLocation) != 1 || p.JobLocation[0].Address.AddressLocality != "Stockholm" ||
		p.JobLocation[0].Address.AddressCountry != "SE" {
		t.Errorf("JobLocation = %+v", p.JobLocation)
	}
}

func TestTTJobPostingAbsent(t *testing.T) {
	h := `<html><head><script type="application/ld+json">{"@type":"WebSite"}</script></head></html>`
	if _, ok := ttJobPosting(parseHTML(t, h)); ok {
		t.Error("ttJobPosting() ok = true, want false when no JobPosting present")
	}
}

func TestTeamtailorProvider(t *testing.T) {
	if got := NewTeamtailor(nil).Provider(); got != "teamtailor" {
		t.Errorf("Provider() = %q, want %q", got, "teamtailor")
	}
}

func TestTTJobID(t *testing.T) {
	cases := map[string]string{
		"https://jobs.tibber.com/jobs/7860742-cloud-platform-engineer": "7860742",
		"https://careers.normative.io/jobs/678-designer":               "678",
		"https://jobs.tibber.com/jobs/12345":                           "12345",
		"https://jobs.tibber.com/jobs/connect":                         "",
	}
	for u, want := range cases {
		if got := ttJobID(u); got != want {
			t.Errorf("ttJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestTeamtailorFetchListingThenDetailAndMaps(t *testing.T) {
	jobURL := "https://jobs.tibber.com/jobs/7860742-cloud-platform-engineer"
	detail := ttDetailHTML(
		"Cloud Platform Engineer",
		"&lt;p&gt;Build &lt;b&gt;it&lt;/b&gt;.&lt;/p&gt;&lt;script&gt;alert(1)&lt;/script&gt;",
		"2026-06-08T00:00:00+02:00", "Stockholm", "SE", "")
	fake := (&routedHTTP{}).
		route("page=1", ttListingHTML(jobURL)).
		route("page=2", ttListingHTML()).
		route("/jobs/7860742", detail)

	jobs, err := NewTeamtailor(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Tibber", Provider: "teamtailor", Board: "jobs.tibber.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "7860742" {
		t.Errorf("ExternalID = %q, want 7860742", j.ExternalID)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Title != "Cloud Platform Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Tibber" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Stockholm, SE" {
		t.Errorf("Location = %q, want %q", j.Location, "Stockholm, SE")
	}
	if strings.Contains(j.Description, "<script>") ||
		!strings.Contains(j.Description, "<p>") || !strings.Contains(j.Description, "<b>it</b>") {
		t.Errorf("Description not unescaped/sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 7, 22, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-08T00:00:00+02:00", j.PostedAt)
	}
}

func TestTeamtailorPaginatesUntilEmpty(t *testing.T) {
	d := ttDetailHTML("Role", "&lt;p&gt;x&lt;/p&gt;", "2026-06-08T00:00:00+02:00", "Berlin", "DE", "")
	fake := (&routedHTTP{}).
		route("page=1", ttListingHTML("https://b/jobs/1-a", "https://b/jobs/2-b")).
		route("page=2", ttListingHTML("https://b/jobs/3-c")).
		route("page=3", ttListingHTML()).
		route("/jobs/1", d).route("/jobs/2", d).route("/jobs/3", d)

	jobs, err := NewTeamtailor(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (two pages enumerated)", len(jobs))
	}
}

func TestTeamtailorStopsWhenPageYieldsNoNewLinks(t *testing.T) {
	// A board that returns the same links for any ?page=N must not loop: a page with no
	// *new* links terminates enumeration just like an empty page.
	d := ttDetailHTML("Role", "&lt;p&gt;x&lt;/p&gt;", "2026-06-08T00:00:00+02:00", "Oslo", "NO", "")
	fake := (&routedHTTP{}).
		route("/jobs?page", ttListingHTML("https://b/jobs/1-a")). // every page returns the same link
		route("/jobs/1", d)

	jobs, err := NewTeamtailor(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (de-duplicated, no runaway loop)", len(jobs))
	}
}

func TestTeamtailorRemoteFromLocationType(t *testing.T) {
	jobURL := "https://b/jobs/9-remote-role"
	d := ttDetailHTML("Backend Engineer", "&lt;p&gt;x&lt;/p&gt;", "2026-06-08T00:00:00+02:00", "", "", "TELECOMMUTE")
	fake := (&routedHTTP{}).
		route("page=1", ttListingHTML(jobURL)).route("page=2", ttListingHTML()).
		route("/jobs/9", d)
	jobs, err := NewTeamtailor(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].Remote {
		t.Fatalf("want one remote job, got %+v", jobs)
	}
}

func TestTeamtailorRemoteIgnoresTitle(t *testing.T) {
	jobURL := "https://b/jobs/9-remote-sensing-engineer"
	// jobLocationType empty + "Remote" only in the title must NOT flag remote: jobLocationType
	// is the authoritative signal, isRemote(location) the fallback — never the title.
	d := ttDetailHTML("Remote Sensing Engineer", "&lt;p&gt;x&lt;/p&gt;", "2026-06-08T00:00:00+02:00", "Berlin", "DE", "")
	fake := (&routedHTTP{}).
		route("page=1", ttListingHTML(jobURL)).route("page=2", ttListingHTML()).
		route("/jobs/9", d)
	jobs, err := NewTeamtailor(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Remote {
		t.Fatalf("title-only 'Remote' must not flag remote, got %+v", jobs)
	}
}

func TestTeamtailorFailedDetailDropsOnlyThatPosting(t *testing.T) {
	d := ttDetailHTML("Kept", "&lt;p&gt;x&lt;/p&gt;", "2026-06-08T00:00:00+02:00", "Paris", "FR", "")
	// No route for /jobs/222 → GetHTML errors → that posting drops.
	fake := (&routedHTTP{}).
		route("page=1", ttListingHTML("https://b/jobs/111-kept", "https://b/jobs/222-dropped")).
		route("page=2", ttListingHTML()).
		route("/jobs/111", d)

	jobs, err := NewTeamtailor(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "111" {
		t.Fatalf("got %v, want only the kept posting", jobs)
	}
}

func TestTeamtailorRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["teamtailor"]
	if !ok {
		t.Fatal("All() missing provider teamtailor")
	}
	if s.Provider() != "teamtailor" {
		t.Errorf("All()[teamtailor].Provider() = %q", s.Provider())
	}
}

// ttRootListingHTTP models a Teamtailor site that disables /jobs (404) and renders its
// listing on the root instead: /jobs?page=N → 404, /?page=1 → one job, /?page=2 → empty.
type ttRootListingHTTP struct {
	jobURL, detail string
}

func (h ttRootListingHTTP) GetHTML(_ context.Context, u string) (*html.Node, error) {
	switch {
	case strings.Contains(u, "/jobs?page="):
		return nil, &StatusError{Code: 404, URL: u}
	case strings.Contains(u, "/jobs/"): // detail page
		return html.Parse(strings.NewReader(h.detail))
	case strings.Contains(u, "/?page=1"):
		return html.Parse(strings.NewReader(ttListingHTML(h.jobURL)))
	default: // /?page=2 and beyond → empty listing ends enumeration
		return html.Parse(strings.NewReader(ttListingHTML()))
	}
}

func TestTeamtailorFallsBackToRootListingWhenJobsPath404s(t *testing.T) {
	jobURL := "https://jobs.proxify.io/jobs/8024669-enterprise-account-executive"
	d := ttDetailHTML("Enterprise Account Executive", "&lt;p&gt;x&lt;/p&gt;",
		"2026-07-06T00:00:00+02:00", "Stockholm", "SE", "")
	fake := ttRootListingHTTP{jobURL: jobURL, detail: d}

	jobs, err := NewTeamtailor(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Proxify", Board: "jobs.proxify.io",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "8024669" {
		t.Fatalf("got %v, want the one root-listed posting", jobs)
	}
}

func TestTeamtailorEmptyListingYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("page=1", ttListingHTML())
	jobs, err := NewTeamtailor(fake).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
