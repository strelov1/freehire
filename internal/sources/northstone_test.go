package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// northstoneListingHTML is a Northstone brand listing page: server-rendered anchors to
// /<section>/<slug> detail pages, plus noise anchors (the bare listing and another section)
// that the job-link predicate must reject.
func northstoneListingHTML(section string, slugs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><ul>`)
	for _, s := range slugs {
		b.WriteString(`<li><a href="/` + section + `/` + s + `">A Role</a></li>`)
	}
	b.WriteString(`<a href="/` + section + `/">All roles</a>`) // bare listing — not a posting
	b.WriteString(`<a href="/about">About</a>`)                // other section — not a posting
	b.WriteString(`</ul></body></html>`)
	return b.String()
}

// northstoneRemoteDetailHTML is a fully-remote detail page (EnzRossi-shaped): jobLocation is
// null and the location comes from applicantLocationRequirements; jobLocationType TELECOMMUTE
// makes it remote. The description embeds a <script> (escaped as <\/script>) sanitizeHTML strips.
func northstoneRemoteDetailHTML(title, id string) string {
	return `<html><head></head><body>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"` + title + `",
"description":"<h2>Role</h2><p>Ship features.</p><script>alert(1)<\/script>",
"datePosted":"2026-07-14T13:34:41.575+00:00",
"employmentType":"FULL_TIME",
"jobLocationType":"TELECOMMUTE",
"jobLocation":null,
"applicantLocationRequirements":[{"@type":"Country","name":"Brazil"},{"@type":"Country","name":"Argentina"}],
"hiringOrganization":{"@type":"Organization","name":"EnzRossi","sameAs":"https://vetted.enzrossi.com"},
"identifier":{"@type":"PropertyValue","name":"EnzRossi","value":"` + id + `"}}
</script></body></html>`
}

// northstoneOnsiteDetailHTML is an on-site detail page (Langford-shaped): the JobPosting is
// wrapped in a single-element ARRAY (as Langford emits), the location comes from the jobLocation
// address, there is no jobLocationType so WorkMode stays empty, employmentType "OTHER" maps to
// no structured type, and datePosted is a bare date the RFC3339 parser must fall back on.
func northstoneOnsiteDetailHTML(title, id string) string {
	return `<html><body>
<script type="application/ld+json">
[{"@type":"JobPosting","title":"` + title + `",
"description":"<p>On location.</p>",
"datePosted":"2026-07-01",
"employmentType":"OTHER",
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress",
"addressLocality":"Calgary","addressRegion":"Alberta","addressCountry":"CA"}},
"identifier":{"@type":"PropertyValue","name":"Langford Staffing","value":"` + id + `"}}]
</script></body></html>`
}

func TestNorthstoneProvider(t *testing.T) {
	if got := NewNorthstone(nil).Provider(); got != "northstone" {
		t.Errorf("Provider() = %q, want %q", got, "northstone")
	}
}

func TestNorthstoneBoard(t *testing.T) {
	okCases := map[string]struct{ host, section string }{
		"vetted.enzrossi.com/positions":      {"vetted.enzrossi.com", "positions"},
		"www.revun.com/careers":              {"www.revun.com", "careers"},
		"/www.langfordstaffing.com/careers/": {"www.langfordstaffing.com", "careers"},
	}
	for board, want := range okCases {
		host, section, err := northstoneBoard(board)
		if err != nil {
			t.Errorf("northstoneBoard(%q) error: %v", board, err)
			continue
		}
		if host != want.host || section != want.section {
			t.Errorf("northstoneBoard(%q) = (%q,%q), want (%q,%q)", board, host, section, want.host, want.section)
		}
	}
	for _, bad := range []string{"", "hostonly", "host/", "/careers", "host/a/b"} {
		if _, _, err := northstoneBoard(bad); err == nil {
			t.Errorf("northstoneBoard(%q) = nil error, want error", bad)
		}
	}
}

func TestNorthstoneIsJob(t *testing.T) {
	isJob := northstoneIsJob("careers")
	accept := []string{"/careers/cto-head-of-engineering-calgary", "https://www.revun.com/careers/some-role/"}
	reject := []string{"/careers/", "/careers", "/about", "/careers/role/apply", "/positions/role"}
	for _, href := range accept {
		if !isJob(href) {
			t.Errorf("isJob(%q) = false, want true", href)
		}
	}
	for _, href := range reject {
		if isJob(href) {
			t.Errorf("isJob(%q) = true, want false", href)
		}
	}
}

func TestNorthstoneFetchMapsRemotePosting(t *testing.T) {
	id := "5ec4e6b2-364d-4e4b-be3d-ede0c77da0de"
	// Detail routes precede the listing route: the listing URL matches only "/positions/",
	// while a detail URL matches its more specific slug substring first.
	fake := (&routedHTTP{}).
		route("positions/senior-go-engineer", northstoneRemoteDetailHTML("Senior Go Engineer", id)).
		route("/positions/", northstoneListingHTML("positions", "senior-go-engineer"))

	jobs, err := NewNorthstone(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Fallback Co", Provider: "northstone", Board: "vetted.enzrossi.com/positions",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != id {
		t.Errorf("ExternalID = %q, want identifier.value %q", j.ExternalID, id)
	}
	if j.URL != "https://vetted.enzrossi.com/positions/senior-go-engineer/" {
		t.Errorf("URL = %q, want canonical trailing-slash detail URL", j.URL)
	}
	if j.Title != "Senior Go Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "EnzRossi" {
		t.Errorf("Company = %q, want hiringOrganization name", j.Company)
	}
	if j.Location != "Brazil, Argentina" {
		t.Errorf("Location = %q, want applicant-location countries", j.Location)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want remote from TELECOMMUTE", j.Remote, j.WorkMode)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Ship features") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 14, 13, 34, 41, 575000000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-07-14T13:34:41.575Z", j.PostedAt)
	}
}

func TestNorthstoneFetchMapsOnsitePostingFromJobLocation(t *testing.T) {
	id := "2842000000195206"
	fake := (&routedHTTP{}).
		route("careers/account-manager-calgary", northstoneOnsiteDetailHTML("Account Manager", id)).
		route("/careers/", northstoneListingHTML("careers", "account-manager-calgary"))

	jobs, err := NewNorthstone(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Langford Staffing", Provider: "northstone", Board: "www.langfordstaffing.com/careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != id {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, id)
	}
	if j.Location != "Calgary, Alberta, CA" {
		t.Errorf("Location = %q, want jobLocation address", j.Location)
	}
	if j.Remote || j.WorkMode != "" {
		t.Errorf("Remote=%v WorkMode=%q, want on-site (no jobLocationType)", j.Remote, j.WorkMode)
	}
	if j.EmploymentType != "" {
		t.Errorf("EmploymentType = %q, want empty for OTHER", j.EmploymentType)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want bare-date 2026-07-01", j.PostedAt)
	}
}

func TestNorthstoneDropsDetailWithoutJobPosting(t *testing.T) {
	fake := (&routedHTTP{}).
		route("positions/good-role", northstoneRemoteDetailHTML("Good Role", "abc")).
		route("positions/no-ld", `<html><body>no ld+json here</body></html>`).
		route("/positions/", northstoneListingHTML("positions", "good-role", "no-ld"))

	jobs, err := NewNorthstone(fake).Fetch(context.Background(), CompanyEntry{Board: "vetted.enzrossi.com/positions"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "abc" {
		t.Fatalf("got %v, want only the posting with a JobPosting block", jobs)
	}
}

func TestNorthstoneRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["northstone"]
	if !ok {
		t.Fatal("All() missing provider northstone")
	}
	if s.Provider() != "northstone" {
		t.Errorf("All()[northstone].Provider() = %q", s.Provider())
	}
	if !slices.Contains(FilterableProviders(), "northstone") {
		t.Error("FilterableProviders() should include northstone (board-based)")
	}
}
