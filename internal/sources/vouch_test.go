package sources

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

// vouchPage wraps a flight body (the concatenated RSC stream content) into the
// self.__next_f.push([1,"…"]) <script> shape a vouch company page server-renders. The body
// is JSON-string-escaped, exactly as Next.js emits it, so the adapter's flight decoder
// round-trips it back.
func vouchPage(flightBody string) string {
	esc, _ := json.Marshal(flightBody)
	return `<html><body><script>self.__next_f.push([1,` + string(esc) + `])</script></body></html>`
}

// vouchFlight embeds a listings JSON array inside a plausible flight body (surrounded by
// other RSC content, so bracketSlice must isolate the array).
func vouchFlight(listingsJSON string) string {
	return `["$","div",null,{"id":"jobs","children":["$","$L37",null,{"listings":` + listingsJSON + `}]}],"footer":1`
}

func TestVouchProvider(t *testing.T) {
	if got := NewVouch(nil).Provider(); got != "vouch" {
		t.Errorf("Provider() = %q, want %q", got, "vouch")
	}
}

func TestVouchRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["vouch"]
	if !ok {
		t.Fatal("All() missing provider vouch")
	}
	if s.Provider() != "vouch" {
		t.Errorf("All()[vouch].Provider() = %q", s.Provider())
	}
	if !slices.Contains(FilterableProviders(), "vouch") {
		t.Error("FilterableProviders() should include vouch")
	}
}

func TestVouchNoFlightIsError(t *testing.T) {
	fake := &fakeHTTP{body: `<html><body>no flight here</body></html>`}
	if _, err := NewVouch(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("want error when the page carries no flight payload")
	}
}

func TestVouchNoListingsIsError(t *testing.T) {
	fake := &fakeHTTP{body: vouchPage(`["$","div",null,{"children":"nothing"}]`)}
	if _, err := NewVouch(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("want error when the flight has no listings array")
	}
}

func TestVouchMapsLiveListing(t *testing.T) {
	listing := `[{
		"id":"cmjob1","url":"/companies/acme/cmjob1","title":"Senior Software Engineer",
		"pitch":"Shape an AI platform.","must":"<p>Ship production systems.</p>","nice":"<p>NestJS.</p>",
		"description":"","employmentType":["Full-time","Remote"],
		"publishedAt":"2025-10-21T07:15:49.762Z","activated":true,"draft":false,"unlisted":false,
		"company":{"name":"Laine"},"locations":[]
	}]`
	fake := &fakeHTTP{body: vouchPage(vouchFlight(listing))}

	jobs, err := NewVouch(fake).Fetch(context.Background(), CompanyEntry{Company: "Fallback", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "cmjob1" {
		t.Errorf("ExternalID = %q, want cmjob1", j.ExternalID)
	}
	if j.URL != "https://vouch.careers/companies/acme/cmjob1" {
		t.Errorf("URL = %q, want resolved absolute", j.URL)
	}
	if j.Title != "Senior Software Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Laine" {
		t.Errorf("Company = %q, want company.name", j.Company)
	}
	if j.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (employmentType has Remote)", j.WorkMode)
	}
	if !j.Remote {
		t.Error("Remote should be true for a Remote employmentType")
	}
	if !strings.Contains(j.Description, "Ship production systems") || !strings.Contains(j.Description, "NestJS") {
		t.Errorf("Description lost body content: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Shape an AI platform") {
		t.Errorf("Description lost pitch: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2025, 10, 21, 7, 15, 49, 762000000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2025-10-21T07:15:49Z", j.PostedAt)
	}
}

func TestVouchExcludesNonLiveAndEmptyID(t *testing.T) {
	listings := `[
		{"id":"live","url":"/companies/acme/live","title":"Live","employmentType":["Full-time"],"publishedAt":"2025-10-21T07:15:49.762Z","activated":true,"draft":false,"unlisted":false,"company":{"name":"Co"},"locations":[]},
		{"id":"draft","url":"/companies/acme/draft","title":"Draft","activated":true,"draft":true,"unlisted":false,"company":{"name":"Co"}},
		{"id":"unlisted","url":"/companies/acme/unlisted","title":"Unlisted","activated":true,"draft":false,"unlisted":true,"company":{"name":"Co"}},
		{"id":"deactivated","url":"/companies/acme/deact","title":"Deactivated","activated":false,"draft":false,"unlisted":false,"company":{"name":"Co"}},
		{"id":"","url":"/companies/acme/noid","title":"No ID","activated":true,"draft":false,"unlisted":false,"company":{"name":"Co"}}
	]`
	fake := &fakeHTTP{body: vouchPage(vouchFlight(listings))}

	jobs, err := NewVouch(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "live" {
		t.Fatalf("got %v, want only the live listing (draft/unlisted/deactivated/empty-id excluded)", jobs)
	}
}

func TestVouchEscapesPlainTextPitch(t *testing.T) {
	// A pitch is plain prose; a bare "<" must be escaped, else the sanitizer reads the tail
	// as a bogus tag and drops it.
	listing := `[{
		"id":"cmjob1","url":"/companies/acme/cmjob1","title":"Role",
		"pitch":"Join a <10 person team building the future.","must":"<p>Ship.</p>",
		"employmentType":["Full-time","Remote"],"publishedAt":"2025-10-21T07:15:49.762Z",
		"activated":true,"draft":false,"unlisted":false,"company":{"name":"Co"},"locations":[]
	}]`
	fake := &fakeHTTP{body: vouchPage(vouchFlight(listing))}

	jobs, err := NewVouch(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if !strings.Contains(jobs[0].Description, "10 person team building the future") {
		t.Errorf("pitch tail dropped (unescaped '<'): %q", jobs[0].Description)
	}
	if !strings.Contains(jobs[0].Description, "Ship") {
		t.Errorf("must section lost: %q", jobs[0].Description)
	}
}

func TestVouchWorkModeAndLocation(t *testing.T) {
	listings := `[
		{"id":"h","url":"/companies/acme/h","title":"Hybrid Role","employmentType":["Full-time","Hybrid"],"activated":true,"draft":false,"unlisted":false,"company":{"name":"Co"},
		 "locations":[{"address":"Geneva, Switzerland","city":"Geneva","administrative_area":"Geneva","country":"CH"}]},
		{"id":"o","url":"/companies/acme/o","title":"Onsite Role","employmentType":["Full-time","On-site"],"activated":true,"draft":false,"unlisted":false,"company":{"name":"Co"},"locations":[]}
	]`
	fake := &fakeHTTP{body: vouchPage(vouchFlight(listings))}

	jobs, err := NewVouch(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if byID["h"].WorkMode != "hybrid" {
		t.Errorf("hybrid job WorkMode = %q, want hybrid", byID["h"].WorkMode)
	}
	if byID["h"].Location != "Geneva, Switzerland" {
		t.Errorf("hybrid job Location = %q, want %q", byID["h"].Location, "Geneva, Switzerland")
	}
	if byID["o"].WorkMode != "onsite" {
		t.Errorf("onsite job WorkMode = %q, want onsite", byID["o"].WorkMode)
	}
}
