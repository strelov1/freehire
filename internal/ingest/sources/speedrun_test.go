package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// speedrunCompanyJSON is a company page's payload: the directory record plus the ids of its
// live roles, which the adapter uses to address each detail endpoint.
func speedrunCompanyJSON(ids ...string) string {
	var items []string
	for _, id := range ids {
		items = append(items, `{"id":"`+id+`","title":"listed title"}`)
	}
	return `{"company":{"slug":"brief","name":"Brief","jobs":[` + strings.Join(items, ",") + `]}}`
}

// speedrunDetailJSON is one role's detail payload. description_text is plain text — blank
// lines separate blocks and "- " marks bullets — and apply carries the hosted/federated
// application target.
func speedrunDetailJSON(id, status, applyKind, applyURL string) string {
	return `{"job":{"id":"` + id + `","status":"` + status + `",` +
		`"url":"https://speedrun-talent-network.com/jobs/product-builder-brief-41dac67e",` +
		`"title":"Product Builder","company":"Brief","company_slug":"brief","tier":"speedrun",` +
		`"location":"San Francisco, CA","workplace_type":"OnSite","employment_type":"FullTime",` +
		`"function":"other","seniority":null,"remote":false,` +
		`"comp_summary":"$150K – $200K • 0.5% – 1.5%","comp_min":150000,"comp_max":200000,` +
		`"published_at":"2026-07-15",` +
		`"apply":{"kind":"` + applyKind + `","url":"` + applyURL + `"},` +
		`"description_text":"Brief is the context navigator.\n\nWhat you will do\n- Ship product\n- Talk to users"}}`
}

func TestSpeedrunProvider(t *testing.T) {
	if got := NewSpeedrun(nil).Provider(); got != "speedrun" {
		t.Errorf("Provider() = %q, want %q", got, "speedrun")
	}
}

func TestSpeedrunFetchCompanyThenDetailAndMaps(t *testing.T) {
	const id = "41dac67e-5caa-44c5-9e00-8f64b48d7c0a"
	fake := (&routedHTTP{}).
		route("/api/v1/jobs/"+id, speedrunDetailJSON(id, "open", "onsite",
			"https://speedrun-talent-network.com/jobs/product-builder-brief-41dac67e")).
		route("/api/v1/companies/brief", speedrunCompanyJSON(id))

	jobs, err := NewSpeedrun(fake).Fetch(context.Background(), CompanyEntry{Company: "Brief", Provider: "speedrun", Board: "brief"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != id {
		t.Errorf("ExternalID = %q, want the role UUID %q", j.ExternalID, id)
	}
	if j.URL != "https://speedrun-talent-network.com/jobs/product-builder-brief-41dac67e" {
		t.Errorf("URL = %q, want the application target", j.URL)
	}
	if j.Title != "Product Builder" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Brief" {
		t.Errorf("Company = %q, want the board display name", j.Company)
	}
	if j.Location != "San Francisco, CA" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.WorkMode != "onsite" || j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want onsite and not remote", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want the published_at date", j.PostedAt)
	}
	// The structured band has no Job field of its own, so it leads the description.
	if !strings.Contains(j.Description, "$150K") {
		t.Errorf("Description drops the compensation band: %q", j.Description)
	}
	// Plain text: blank lines are blocks, "- " lines are a list. Rendered raw it would
	// collapse into one wall of text.
	if !strings.Contains(j.Description, "<p>Brief is the context navigator.</p>") {
		t.Errorf("Description lost its paragraph structure: %q", j.Description)
	}
	if !strings.Contains(j.Description, "<li>Ship product</li>") {
		t.Errorf("Description lost its bullets: %q", j.Description)
	}
	// The platform's own function/seniority labels are its normalization, not a first-party
	// field, so the pipeline's title dictionaries stay in charge.
	if j.Category != "" || j.Seniority != "" {
		t.Errorf("Category=%q Seniority=%q, want both left to the dictionaries", j.Category, j.Seniority)
	}
}

func TestSpeedrunPrefersTheFederatedApplyURL(t *testing.T) {
	const id = "6745c6f2-0b9e-4a18-a721-94b1cc1f136c"
	fake := (&routedHTTP{}).
		route("/api/v1/jobs/"+id, speedrunDetailJSON(id, "open", "external",
			"https://jobs.ashbyhq.com/Bastion/6745c6f2/application")).
		route("/api/v1/companies/brief", speedrunCompanyJSON(id))

	jobs, err := NewSpeedrun(fake).Fetch(context.Background(), CompanyEntry{Company: "Brief", Board: "brief"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].URL != "https://jobs.ashbyhq.com/Bastion/6745c6f2/application" {
		t.Fatalf("got %v, want the role to point at where one actually applies", jobs)
	}
}

func TestSpeedrunDropsClosedAndUnreachableRoles(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/api/v1/jobs/live", speedrunDetailJSON("live", "open", "onsite", "https://speedrun-talent-network.com/jobs/live")).
		route("/api/v1/jobs/gone", speedrunDetailJSON("gone", "closed", "onsite", "https://speedrun-talent-network.com/jobs/gone")).
		// "vanished" has no detail route at all → its fetch fails.
		route("/api/v1/companies/brief", speedrunCompanyJSON("live", "gone", "vanished"))

	jobs, err := NewSpeedrun(fake).Fetch(context.Background(), CompanyEntry{Company: "Brief", Board: "brief"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "live" {
		t.Fatalf("got %v, want only the open, reachable role", jobs)
	}
}

func TestSpeedrunUnknownCompanyIsABoardFailure(t *testing.T) {
	fake := &routedHTTP{} // no routes: the company endpoint 404s
	if _, err := NewSpeedrun(fake).Fetch(context.Background(), CompanyEntry{Company: "Brief", Board: "brief"}); err == nil {
		t.Fatal("Fetch: want an error when the company page cannot be read, got nil")
	}
}

func TestSpeedrunEmploymentType(t *testing.T) {
	cases := map[string]string{
		"FullTime": "full_time", "PartTime": "part_time",
		"Contract": "contract", "Temporary": "contract",
		"Intern": "internship", "": "", "Seasonal": "",
	}
	for in, want := range cases {
		if got := speedrunEmploymentType(in); got != want {
			t.Errorf("speedrunEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSpeedrunRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["speedrun"]
	if !ok {
		t.Fatal("All() missing provider speedrun")
	}
	if s.Provider() != "speedrun" {
		t.Errorf("All()[speedrun].Provider() = %q", s.Provider())
	}
	// Board-based (one board per portfolio company): it belongs in the source facet.
	if !slices.Contains(FilterableProviders(), "speedrun") {
		t.Error("FilterableProviders() should include board-based speedrun")
	}
}
