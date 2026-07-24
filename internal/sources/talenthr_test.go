package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// talenthrListingJSON is a tenant listing page's __NEXT_DATA__: positions carries each open
// posting's id and slug, which the adapter uses to address the detail page.
func talenthrListingJSON(items ...talenthrListItem) string {
	var parts []string
	for _, it := range items {
		parts = append(parts, `{"id":`+it.ID.String()+`,"slug":"`+it.Slug+`"}`)
	}
	return `<script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"positions":[` + strings.Join(parts, ",") + `]}}}` +
		`</script>`
}

// talenthrDetailJSON is a detail page's __NEXT_DATA__ data record. The description embeds a
// <script> that sanitizeHTML must strip; employment_type is the work-mode enum and
// employment_status_name the full-time/part-time enum.
func talenthrDetailJSON(title, employmentType, status string) string {
	return `<script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"data":{"job_position_title":"` + title + `",` +
		`"job_description":"<p>Own the backend.</p><script>alert(1)</script>",` +
		`"is_remote":false,"employment_type":"` + employmentType + `",` +
		`"employment_status_name":"` + status + `",` +
		`"city":"","region":"","country":"CH","location_name":"CH",` +
		`"publish_date":"2026-07-17 10:33:19","created_at":"2026-07-17 10:33:18"}}}}` +
		`</script>`
}

func TestTalentHRProvider(t *testing.T) {
	if got := NewTalentHR(nil).Provider(); got != "talenthr" {
		t.Errorf("Provider() = %q, want %q", got, "talenthr")
	}
}

func TestTalentHRFetchListingThenDetailAndMaps(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/dnext/senior-backend-developer-2/22", talenthrDetailJSON("Senior Backend Developer", "Remote", "Full-Time")).
		route("jobs.talenthr.io/dnext", talenthrListingJSON(talenthrListItem{ID: "22", Slug: "senior-backend-developer-2"}))

	jobs, err := NewTalentHR(fake).Fetch(context.Background(), CompanyEntry{Company: "Dnext Intelligence", Provider: "talenthr", Board: "dnext"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "22" {
		t.Errorf("ExternalID = %q, want 22", j.ExternalID)
	}
	if j.URL != "https://jobs.talenthr.io/dnext/senior-backend-developer-2/22" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Senior Backend Developer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Dnext Intelligence" {
		t.Errorf("Company = %q, want the board display name", j.Company)
	}
	if j.Location != "CH" {
		t.Errorf("Location = %q, want CH", j.Location)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want remote", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Own the backend") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 17, 10, 33, 19, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want publish_date 2026-07-17 10:33:19", j.PostedAt)
	}
}

func TestTalentHRDropsMissingSlugAndMissingDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/dnext/live-role/10", talenthrDetailJSON("Live Role", "Onsite", "Full-Time")).
		// "gone" has a slug but no detail route → its detail fetch fails
		route("jobs.talenthr.io/dnext", talenthrListingJSON(
			talenthrListItem{ID: "10", Slug: "live-role"},
			talenthrListItem{ID: "11", Slug: ""}, // no slug → cannot address the posting
			talenthrListItem{ID: "12", Slug: "gone"},
		))

	jobs, err := NewTalentHR(fake).Fetch(context.Background(), CompanyEntry{Company: "Dnext Intelligence", Board: "dnext"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "10" {
		t.Fatalf("got %v, want only the posting with a slug and a reachable detail", jobs)
	}
}

func TestTalentHRWorkMode(t *testing.T) {
	cases := []struct {
		employmentType string
		isRemote       bool
		want           string
	}{
		{"Remote", false, "remote"},
		{"Hybrid", false, "hybrid"},
		{"Onsite", false, "onsite"},
		{"On-site", false, "onsite"},
		{"", true, "remote"},
		{"", false, ""},
		{"Weird", false, ""},
	}
	for _, c := range cases {
		if got := talenthrWorkMode(c.employmentType, c.isRemote); got != c.want {
			t.Errorf("talenthrWorkMode(%q, %v) = %q, want %q", c.employmentType, c.isRemote, got, c.want)
		}
	}
}

func TestTalentHREmploymentType(t *testing.T) {
	cases := map[string]string{
		"Full-Time": "full_time", "Part-Time": "part_time",
		"Contract": "contract", "Internship": "internship", "": "", "Seasonal": "",
	}
	for in, want := range cases {
		if got := talenthrEmploymentType(in); got != want {
			t.Errorf("talenthrEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTalentHRRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["talenthr"]
	if !ok {
		t.Fatal("All() missing provider talenthr")
	}
	if s.Provider() != "talenthr" {
		t.Errorf("All()[talenthr].Provider() = %q", s.Provider())
	}
	// Multi-tenant, board-based: it belongs in the source facet.
	if !slices.Contains(FilterableProviders(), "talenthr") {
		t.Error("FilterableProviders() should include board-based talenthr")
	}
}
