package sources

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
)

// firefishFixture reads the RSS document captured verbatim from
// careers.arnoldclark.com/rss/adverts/latest.aspx (2026-11-07, trimmed to four items covering
// every mapping branch: salary with bounds and rate, contract day rate, pay withheld, country
// withheld); list_empty.xml is a well-formed channel with no items.
func firefishFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/firefish/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

const firefishArnoldClark = "careers.arnoldclark.com/rss/adverts/latest.aspx"

func firefishArnoldClarkHTTP(t *testing.T) *routedHTTP {
	return (&routedHTTP{}).route(firefishArnoldClark, firefishFixture(t, "list_arnoldclark.xml"))
}

var firefishArnoldClarkEntry = CompanyEntry{Company: "Arnold Clark", Provider: "firefish", Board: "careers.arnoldclark.com"}

func TestFirefishProvider(t *testing.T) {
	s, ok := All(nil)["firefish"]
	if !ok || s.Provider() != "firefish" {
		t.Fatal("All() must register firefish")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("firefish must be a HydratingSource: the ingest sweep drives FetchNew")
	}
	if !FullBoardListingProviders(All(nil))["firefish"] {
		t.Error("firefish's feed is one unpaginated document: it must be fullBoardListing")
	}
}

func TestFirefishMapsTheFeed(t *testing.T) {
	jobs, err := NewFirefish(firefishArnoldClarkHTTP(t)).Fetch(context.Background(), firefishArnoldClarkEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("jobs = %d, want the fixture's 4", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	m := byID["92656"]
	if m.URL != "https://careers.arnoldclark.com/job/mechanic-92656.aspx" || m.Title != "Mechanic" || m.Company != "Arnold Clark" {
		t.Errorf("URL/Title/Company = %q/%q/%q", m.URL, m.Title, m.Company)
	}
	if m.Location != "Shrewsbury" || !slices.Equal(m.Countries, []string{"gb"}) {
		t.Errorf("Location/Countries = %q/%v", m.Location, m.Countries)
	}
	if m.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", m.EmploymentType)
	}
	if m.PostedAt == nil || m.PostedAt.UTC().Format("2006-01-02") != "2026-11-06" {
		t.Errorf("PostedAt = %v", m.PostedAt)
	}
	if d := m.ApplicationDeadline; d == nil || d.UTC().Format("2006-01-02") != "2026-12-04" {
		t.Errorf("ApplicationDeadline = %v", d)
	}
	if m.SalaryMin == nil || *m.SalaryMin != 40000 || m.SalaryMax == nil || *m.SalaryMax != 50000 {
		t.Errorf("salary = %v-%v, want 40000-50000", m.SalaryMin, m.SalaryMax)
	}
	if m.SalaryCurrency != "GBP" || m.SalaryPeriod != "year" {
		t.Errorf("currency/period = %q/%q", m.SalaryCurrency, m.SalaryPeriod)
	}
	if !strings.Contains(m.Description, "Light Vehicle Maintenance Level 3") || strings.Contains(m.Description, "CDATA") {
		t.Errorf("Description = %.120q, want the decoded CDATA body", m.Description)
	}

	// A contract day rate maps as a contract with a per-day salary.
	d := byID["92601"]
	if d.EmploymentType != "contract" || d.SalaryPeriod != "day" || *d.SalaryMin != 180 || *d.SalaryMax != 220 {
		t.Errorf("contract facets = %q/%q/%v-%v", d.EmploymentType, d.SalaryPeriod, d.SalaryMin, d.SalaryMax)
	}
	if !slices.Equal(d.Countries, []string{"gb"}) {
		t.Errorf("contract countries = %v, want [gb] (England)", d.Countries)
	}

	// Pay withheld (0.00) and country withheld leave the facets unset.
	s := byID["92590"]
	if s.SalaryMin != nil || s.SalaryMax != nil || s.SalaryCurrency != "" {
		t.Errorf("withheld salary = %v-%v/%q, want unset", s.SalaryMin, s.SalaryMax, s.SalaryCurrency)
	}
	if s.Countries != nil {
		t.Errorf("withheld country = %v, want unset", s.Countries)
	}
}

// The salary maps only when the platform states both a period and a positive bound; a rate
// nobody recognises is dropped whole rather than emitted bare.
func TestFirefishSalaryNeedsRateAndBound(t *testing.T) {
	tests := []struct {
		name       string
		min, max   string
		rate, want string
	}{
		{"per year with bounds", "40000.00", "50000.00", "Per Year", "year"},
		{"per hour", "12.50", "15.00", "Per Hour", "hour"},
		{"zero minimum means not stated", "0.00", "50000.00", "Per Year", ""},
		{"unknown rate", "40000.00", "50000.00", "Negotiable", ""},
		{"empty rate", "40000.00", "50000.00", "", ""},
	}
	for _, tt := range tests {
		it := firefishItem{
			Title: "x", Reference: "1", Link: "https://b.example.com/job/1.aspx",
			MinPayment: tt.min, MaxPayment: tt.max, PaymentRate: tt.rate, Currency: "GBP",
		}
		j, ok := it.toJob("B", "b.example.com")
		if !ok {
			t.Fatalf("%s: toJob declined", tt.name)
		}
		if got := j.SalaryPeriod; got != tt.want {
			t.Errorf("%s: SalaryPeriod = %q, want %q", tt.name, got, tt.want)
		}
		if tt.want == "" && (j.SalaryMin != nil || j.SalaryCurrency != "") {
			t.Errorf("%s: salary carried without a period: %v/%q", tt.name, j.SalaryMin, j.SalaryCurrency)
		}
	}
}

func TestFirefishEmploymentType(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Permanent", "full_time"},
		{"Contract", "contract"},
		{"Part Time", "part_time"},
		{"Apprenticeship", "internship"},
		{"Temporary", "contract"},
		{"Volunteer", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := firefishEmploymentType(tt.in); got != tt.want {
			t.Errorf("firefishEmploymentType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// A site with no openings answers a well-formed channel with no items: a successful empty read.
func TestFirefishEmptySite(t *testing.T) {
	fake := (&routedHTTP{}).route("quiet.example.com/rss/adverts/latest.aspx", firefishFixture(t, "list_empty.xml"))
	jobs, err := NewFirefish(fake).Fetch(context.Background(), CompanyEntry{Provider: "firefish", Board: "quiet.example.com"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs/err = %d/%v, want an empty board", len(jobs), err)
	}
}

// A tenant the platform does not serve answers 404; the adapter reads it as a gone board.
func TestFirefishGoneBoard(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("gone.example.com", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound})
	_, err := NewFirefish(fake).Fetch(context.Background(), CompanyEntry{Provider: "firefish", Board: "gone.example.com"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

// An answer that is not the feed must fail, not read as an empty site.
func TestFirefishReshapedFeedFails(t *testing.T) {
	fake := (&routedHTTP{}).route("x.example.com", `<html><body>maintenance</body></html>`)
	if _, err := NewFirefish(fake).Fetch(context.Background(), CompanyEntry{Provider: "firefish", Board: "x.example.com"}); err == nil {
		t.Fatal("a non-feed answer must fail, not read as an empty site")
	}
}

// Items without a reference number or a link are dropped: neither a close nor a visitor can
// reach them.
func TestFirefishDropsItemsWithoutIdentity(t *testing.T) {
	body := `<?xml version="1.0"?><rss version="2.0" xmlns:ffAdvert="http://www.firefishsoftware.com/"><channel><title>t</title>` +
		`<item><title>no reference</title><link>https://b.example.com/job/1.aspx</link></item>` +
		`<item><title>no link</title><ffAdvert:ReferenceNumber>2</ffAdvert:ReferenceNumber></item>` +
		`<item><title>dupe</title><link>https://b.example.com/job/3.aspx</link><ffAdvert:ReferenceNumber>3</ffAdvert:ReferenceNumber></item>` +
		`<item><title>dupe again</title><link>https://b.example.com/job/3b.aspx</link><ffAdvert:ReferenceNumber>3</ffAdvert:ReferenceNumber></item>` +
		`</channel></rss>`
	fake := (&routedHTTP{}).route("b.example.com/rss/adverts/latest.aspx", body)
	jobs, err := NewFirefish(fake).Fetch(context.Background(), CompanyEntry{Provider: "firefish", Board: "b.example.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "3" || jobs[0].URL != "https://b.example.com/job/3.aspx" {
		t.Fatalf("jobs = %+v, want the one well-formed item", jobs)
	}
}

// FetchNew marks vacancies seen — the feed carries the whole posting, so a re-upsert costs
// nothing and no detail round-trip exists to skip.
func TestFirefishFetchNewRefreshes(t *testing.T) {
	jobs, err := NewFirefish(firefishArnoldClarkHTTP(t)).(HydratingSource).FetchNew(context.Background(), firefishArnoldClarkEntry,
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("jobs = %d, want the fixture's 4", len(jobs))
	}
	for _, j := range jobs {
		if j.SeenRefresh {
			t.Errorf("job %s must not be a content-less SeenRefresh: the feed already carries its body", j.ExternalID)
		}
	}
}
