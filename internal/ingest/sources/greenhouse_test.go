package sources

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestGreenhouseProvider(t *testing.T) {
	if got := NewGreenhouse(nil).Provider(); got != "greenhouse" {
		t.Errorf("Provider() = %q, want %q", got, "greenhouse")
	}
}

// Greenhouse earns fullBoardListing because Fetch is a single unpaginated request that
// returns the board's whole jobs array in one response — there is no loop that could stop
// early, so any listing failure aborts the whole Fetch rather than returning a partial result.
func TestGreenhouseMarkers(t *testing.T) {
	s := NewGreenhouse(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("greenhouse should implement the fullBoardListing marker")
	}
}

func TestGreenhouseRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["greenhouse"] {
		t.Error("FullBoardListingProviders(All(nil)) should include greenhouse")
	}
}

// A listing fetch failure must abort the whole Fetch, never return a partial result as
// success — the property TestGreenhouseMarkers' fullBoardListing claim rests on.
func TestGreenhouseFetchPropagatesAListingError(t *testing.T) {
	fake := &fakeHTTP{err: errors.New("boom")}
	if _, err := NewGreenhouse(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

func TestGreenhouseFetchDecodesAndSanitizesContent(t *testing.T) {
	// Greenhouse delivers `content` as entity-encoded HTML.
	fake := &fakeHTTP{body: `{
		"jobs": [
			{
				"id": 1,
				"title": "Data Engineer",
				"absolute_url": "https://boards.greenhouse.io/dropbox/jobs/1",
				"location": {"name": "Remote"},
				"content": "&lt;h2&gt;Role&lt;/h2&gt;&lt;p&gt;Build pipelines&lt;/p&gt;&lt;script&gt;alert(1)&lt;/script&gt;"
			}
		]
	}`}

	jobs, err := NewGreenhouse(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Dropbox", Provider: "greenhouse", Board: "dropbox",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	got := jobs[0].Description
	if !strings.Contains(got, "<h2>Role</h2>") || !strings.Contains(got, "<p>Build pipelines</p>") {
		t.Errorf("Description should be decoded HTML, got: %s", got)
	}
	if strings.Contains(got, "&lt;") {
		t.Errorf("Description still entity-encoded, got: %s", got)
	}
	if strings.Contains(got, "<script") {
		t.Errorf("Description retained a script tag, got: %s", got)
	}
}

func TestGreenhouseFetch(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [
			{
				"id": 123,
				"title": "Senior Go Developer",
				"absolute_url": "https://boards.greenhouse.io/gitlab/jobs/123",
				"updated_at": "2024-01-15T10:00:00Z",
				"location": {"name": "Remote - US"},
				"content": "<p>Build things</p>"
			}
		]
	}`}

	jobs, err := NewGreenhouse(fake).Fetch(context.Background(), CompanyEntry{
		Company: "GitLab", Provider: "greenhouse", Board: "gitlab",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !strings.Contains(fake.gotURL, "gitlab") || !strings.Contains(fake.gotURL, "content=true") {
		t.Errorf("requested URL %q should target the board with content=true", fake.gotURL)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "123" {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, "123")
	}
	if j.Title != "Senior Go Developer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://boards.greenhouse.io/gitlab/jobs/123" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "GitLab" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "Remote - US" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.Description != "<p>Build things</p>" {
		t.Errorf("Description = %q", j.Description)
	}
	if !j.Remote {
		t.Error("Remote = false, want true for a Remote location")
	}
	if j.PostedAt == nil {
		t.Error("PostedAt = nil, want parsed updated_at")
	}
}

// Live-verified shapes (2026-08-13): 1800contacts/solidpower/carrotfertility all
// carry an "Employment Type" metadata field; coherentsolutions carries one with an
// empty-array value (unset); most boards (e.g. 1910genetics) carry metadata: null.
func TestGreenhouseMetadataEmploymentType(t *testing.T) {
	cases := []struct {
		name     string
		metadata []GreenhouseMetadataField
		want     string
	}{
		{"no metadata field", nil, ""},
		{
			"unrelated field only",
			[]GreenhouseMetadataField{{Name: "Headcount #", Value: json.RawMessage(`null`)}},
			"",
		},
		{
			"Full-time value",
			[]GreenhouseMetadataField{{Name: "Employment Type", Value: json.RawMessage(`"Full-time"`)}},
			"full_time",
		},
		{
			"case/whitespace-insensitive field name",
			[]GreenhouseMetadataField{{Name: " employment type ", Value: json.RawMessage(`"Part-Time"`)}},
			"part_time",
		},
		{
			"empty-array value (unset multi-select) reads as absent",
			[]GreenhouseMetadataField{{Name: "Employment Type", Value: json.RawMessage(`[]`)}},
			"",
		},
		{
			"unrecognized value maps to empty, not a guess",
			[]GreenhouseMetadataField{{Name: "Employment Type", Value: json.RawMessage(`"Variable Hour"`)}},
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := greenhouseMetadataEmploymentType(c.metadata); got != c.want {
				t.Errorf("greenhouseMetadataEmploymentType() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestGreenhouseFetchReadsEmploymentTypeMetadata(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"jobs": [
			{
				"id": 1,
				"title": "Accountant",
				"location": {"name": "Remote"},
				"content": "",
				"metadata": [
					{"id": 1, "name": "Employment Type", "value": "Full-time", "value_type": "single_select"}
				]
			}
		]
	}`}

	jobs, err := NewGreenhouse(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "greenhouse", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := jobs[0].EmploymentType; got != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", got)
	}
}

// Greenhouse implements CompanyDescriber: the board-metadata endpoint (distinct from
// the postings list) carries an employer-authored "about us" field for a meaningful
// share of boards (confirmed live: Coinbase, Figma, Asana).
func TestGreenhouseImplementsCompanyDescriber(t *testing.T) {
	if _, ok := NewGreenhouse(nil).(CompanyDescriber); !ok {
		t.Error("greenhouse should implement CompanyDescriber")
	}
}

func TestGreenhouseCompanyDescriptionSanitizesContent(t *testing.T) {
	fake := &fakeHTTP{body: `{"name":"Coinbase","content":"<p>Ready to be pushed</p><script>alert(1)</script>"}`}

	got, err := NewGreenhouse(fake).(CompanyDescriber).CompanyDescription(context.Background(), CompanyEntry{Board: "coinbase"})
	if err != nil {
		t.Fatalf("CompanyDescription: %v", err)
	}
	if got != "<p>Ready to be pushed</p>" {
		t.Errorf("CompanyDescription = %q, want the sanitized content", got)
	}
	if fake.gotURL != "https://boards-api.greenhouse.io/v1/boards/coinbase" {
		t.Errorf("requested URL = %q, want the board-metadata endpoint (not /jobs)", fake.gotURL)
	}
}

func TestGreenhouseCompanyDescriptionEmptyContentYieldsEmptyString(t *testing.T) {
	fake := &fakeHTTP{body: `{"name":"Greenhouse","content":""}`}

	got, err := NewGreenhouse(fake).(CompanyDescriber).CompanyDescription(context.Background(), CompanyEntry{Board: "greenhouse"})
	if err != nil {
		t.Fatalf("CompanyDescription: %v", err)
	}
	if got != "" {
		t.Errorf("CompanyDescription = %q, want empty for a blank content field", got)
	}
}

func TestGreenhouseCompanyDescriptionPropagatesAFetchError(t *testing.T) {
	fake := &fakeHTTP{err: &StatusError{Method: "GET", Code: 404, URL: "https://boards-api.greenhouse.io/v1/boards/gone"}}

	_, err := NewGreenhouse(fake).(CompanyDescriber).CompanyDescription(context.Background(), CompanyEntry{Board: "gone"})
	if err == nil {
		t.Fatal("expected an error for a 404 board, got nil")
	}
}

// A bare-"Remote" posting states its country only through the offices it is filed under
// (live shapes, 2026-10-23). The office is a FallbackCountries signal, read only when the
// location is a bare remote marker and the offices agree on exactly one country.
func TestGreenhouseOfficeCountry(t *testing.T) {
	cases := []struct {
		name     string
		location string
		offices  string // the posting's raw "offices" JSON
		want     []string
	}{
		// kardfinancialinc/4732546005, freehire job_reports #30.
		{"bare remote, office located in USA", "Remote", `[{"id":1,"name":"Remote","location":"USA"}]`, []string{"us"}},
		{"geocoded office location", "Remote", `[{"name":"Headquarters","location":"Atlanta, Georgia, United States"}]`, []string{"us"}},
		{"label read when the office has no location", "Remote", `[{"name":"Remote US","location":null}]`, []string{"us"}},
		{"a bare Remote office beside a placed one is skipped", "Remote", `[{"name":"Remote","location":null},{"name":"USA","location":null}]`, []string{"us"}},
		{"offices in one country agree", "Remote", `[{"name":"Remote US-AZ","location":"Arizona, United States"},{"name":"Remote US-CO","location":"Colorado, United States"}]`, []string{"us"}},
		{"location marker variants", "(Remote) ", `[{"name":"India","location":"Hyderabad, Telangana, India"}]`, []string{"in"}},
		{"offices in two countries are a footprint", "Remote", `[{"name":"Canada","location":null},{"name":"US","location":null}]`, nil},
		{"an office the dictionary cannot place", "Remote", `[{"name":"Remote - EMEA","location":"Remote - EMEA"},{"name":"London","location":"London, United Kingdom"}]`, nil},
		{"an office named only Remote is no place", "Remote", `[{"name":"Remote","location":"Remote"}]`, nil},
		{"no offices", "Remote", `[]`, nil},
		{"a scoped location is not bare", "Americas Remote", `[{"name":"USA Remote","location":"Oregon, United States"}]`, nil},
		{"an open-anywhere location is not bare", "Remote - Worldwide", `[{"name":"London","location":"London, United Kingdom"}]`, nil},
		{"a non-remote location is not read", "Hybrid", `[{"name":"Dubai","location":"Dubai, United Arab Emirates"}]`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p GreenhousePosting
			raw := `{"id":1,"title":"Engineer","location":{"name":` + strconv.Quote(tc.location) + `},"offices":` + tc.offices + `}`
			if err := json.Unmarshal([]byte(raw), &p); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			got := MapGreenhousePosting(p)
			if !reflect.DeepEqual(got.FallbackCountries, tc.want) {
				t.Errorf("FallbackCountries = %v, want %v", got.FallbackCountries, tc.want)
			}
			if got.Countries != nil {
				t.Errorf("Countries = %v, want nil: an office is never the authoritative signal", got.Countries)
			}
		})
	}
}

// Omitted, the API drops pay_input_ranges entirely rather than returning it empty.
func TestGreenhouseFetchRequestsPayTransparency(t *testing.T) {
	fake := &fakeHTTP{body: `{"jobs": []}`}
	if _, err := NewGreenhouse(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "pay_transparency=true") {
		t.Errorf("requested URL %q should carry pay_transparency=true", fake.gotURL)
	}
}

// The period is read from the range's title, then its blurb; a range that states none,
// or whose blurb calls an hourly-sized amount "annual", serves no salary.
func TestGreenhouseSalaryReadsOnlyAStatedPeriod(t *testing.T) {
	cents := func(v int64) *int64 { return &v }
	cases := []struct {
		name       string
		r          GreenhousePayRange
		wantMin    int
		wantPeriod string
	}{
		{"title hourly", GreenhousePayRange{MinCents: cents(2200), MaxCents: cents(3050), CurrencyType: "USD", Title: "Hourly Pay Range"}, 22, "hour"},
		{"blurb annual", GreenhousePayRange{MinCents: cents(19000000), MaxCents: cents(23000000), CurrencyType: "USD", Title: "Pay Range", Blurb: "<p>The annual compensation range is below.</p>"}, 190000, "year"},
		{"french monthly", GreenhousePayRange{MinCents: cents(220000), CurrencyType: "EUR", Title: "Echelle salariale", Blurb: "Salaire brut mensuel"}, 2200, "month"},
		{"no period stated", GreenhousePayRange{MinCents: cents(19000000), MaxCents: cents(23000000), CurrencyType: "USD", Title: "US Salary Range"}, 0, ""},
		{"hourly amount called annual", GreenhousePayRange{MinCents: cents(1800), MaxCents: cents(2500), CurrencyType: "USD", Title: "Colorado pay range", Blurb: "annual base pay"}, 0, ""},
		{"no currency", GreenhousePayRange{MinCents: cents(2200), Title: "Hourly"}, 0, ""},
		{"zero-decimal currency", GreenhousePayRange{MinCents: cents(20454000), CurrencyType: "JPY", Title: "Annual Salary Range"}, 20454000, "year"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			job := MapGreenhousePosting(GreenhousePosting{PayInputRanges: []GreenhousePayRange{c.r}})
			if c.wantPeriod == "" {
				if job.SalaryMin != nil || job.SalaryMax != nil || job.SalaryPeriod != "" || job.SalaryCurrency != "" {
					t.Errorf("salary = %v-%v %q/%q, want none", job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod)
				}
				return
			}
			if job.SalaryMin == nil || *job.SalaryMin != c.wantMin || job.SalaryPeriod != c.wantPeriod || job.SalaryCurrency != c.r.CurrencyType {
				t.Errorf("salary = %v %q/%q, want %d %s/%s", job.SalaryMin, job.SalaryCurrency, job.SalaryPeriod, c.wantMin, c.r.CurrencyType, c.wantPeriod)
			}
		})
	}
}

// A named bonus listed first is skipped for the base range after it.
func TestGreenhouseSalarySkipsANamedBonusRange(t *testing.T) {
	cents := func(v int64) *int64 { return &v }
	job := MapGreenhousePosting(GreenhousePosting{PayInputRanges: []GreenhousePayRange{
		{MinCents: cents(1000000), CurrencyType: "USD", Title: "Annual Bonus Range"},
		{MinCents: cents(15000000), CurrencyType: "USD", Title: "Annual Base Salary"},
	}})
	if job.SalaryMin == nil || *job.SalaryMin != 150000 {
		t.Errorf("SalaryMin = %v, want the base range's 150000", job.SalaryMin)
	}
}
