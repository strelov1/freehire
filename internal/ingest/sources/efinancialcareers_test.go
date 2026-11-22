package sources

import (
	"context"
	"embed"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

//go:embed testdata/efinancialcareers
var efcFixtures embed.FS

func efcFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := efcFixtures.ReadFile("testdata/efinancialcareers/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// efcHTTP answers the search by its query: a facet probe gets the fixture named for the facet
// (or a bare total when total is set), a row page the page fixture (a short page, so the walk
// ends). Every query is recorded.
type efcHTTP struct {
	t     *testing.T
	total int
	seen  []url.Values
}

func (f *efcHTTP) GetJSON(_ context.Context, raw string, v any) error {
	u, err := url.Parse(raw)
	if err != nil {
		f.t.Fatal(err)
	}
	q := u.Query()
	f.seen = append(f.seen, q)
	if facet := q.Get("facets"); facet != "" {
		if f.total > 0 {
			return json.Unmarshal([]byte(`{"data":[],"meta":{"totalResults":`+strconv.Itoa(f.total)+`}}`), v)
		}
		return json.Unmarshal(efcFixture(f.t, "probe_"+facet+".json"), v)
	}
	return json.Unmarshal(efcFixture(f.t, "page.json"), v)
}

func TestEFinancialCareersWholeCatalogueUnderTheWindowWalkedAsOne(t *testing.T) {
	f := &efcHTTP{t: t, total: 2}
	var jobs []Job
	s := efinancialcareers{http: f, limiter: noWait{}}
	if err := s.FetchStream(context.Background(), CompanyEntry{}, func(j Job) { jobs = append(jobs, j) }); err != nil {
		t.Fatal(err)
	}
	// The third row has no body, but its listing still establishes its identity.
	if len(jobs) != 3 {
		t.Fatalf("got %d results, want two postings and one unreadable marker", len(jobs))
	}
	if got := jobs[2]; got.ExternalID != "25003998" || !got.Unreadable {
		t.Fatalf("expected the blank-body listing's unreadable identity, got %+v", got)
	}
	if q := f.seen[1]; q.Get("sortBy") != "POSTED_DATE" || q.Get("filters.countryId") != "" {
		t.Errorf("walk query = %v", q)
	}
	a := jobs[0]
	if a.ExternalID != "25004048" || a.Title != "Principal DevOps/Platform Engineer" ||
		a.URL != "https://www.efinancialcareers.com/jobs-United_States-CT-Stamford-Principal_DevOpsPlatform_Engineer.id25004048" ||
		a.Company != "Fnality International" || a.Location != "Stamford, United States" || a.WorkMode != "hybrid" ||
		a.EmploymentType != "full_time" ||
		a.IsTechHint {
		t.Errorf("job = %+v", a)
	}
	// The body is plain text: markup in it is literal prose, escaped, never live.
	if strings.Contains(a.Description, "<script") || !strings.Contains(a.Description, "&lt;script&gt;") ||
		!strings.Contains(a.Description, "&amp; grow") {
		t.Errorf("description not sanitized: %q", a.Description)
	}
	if a.SalaryMin == nil || *a.SalaryMin != 90000 || a.SalaryPeriod != "year" || a.SalaryCurrency != "USD" {
		t.Errorf("salary = %v %v %q", a.SalaryMin, a.SalaryMax, a.SalaryPeriod)
	}
	if a.PostedAt == nil {
		t.Error("posted_at unset")
	}
	b := jobs[1]
	if b.Company != "Brand Ltd" || b.EmploymentType != "contract" || b.WorkMode != "remote" || !b.Remote || b.SalaryMin != nil {
		t.Errorf("contract job = %+v", b)
	}
}

// The plan: country cuts the catalogue, an oversized country is cut by sector, a sector with no
// sub-sector falls through to seniority, and a slice that fits is walked as it stands. The fixture
// pages repeat the same two postings, so the run also ends short of the stated total and fails.
func TestEFinancialCareersOversizedCatalogueIsCutAndShortfallFails(t *testing.T) {
	f := &efcHTTP{t: t}
	s := efinancialcareers{http: f, limiter: noWait{}}
	err := s.FetchStream(context.Background(), CompanyEntry{}, func(Job) {})
	if err == nil || !strings.Contains(err.Error(), "2 distinct postings of the 21000") {
		t.Fatalf("err = %v, want a shortfall", err)
	}
	var walked []string
	for _, q := range f.seen {
		if q.Get("facets") == "" {
			walked = append(walked, q.Get("filters.countryId")+"/"+q.Get("filters.sectors")+"/"+q.Get("filters.seniority"))
		}
	}
	want := []string{
		"6252001/INFORMATION_TECHNOLOGY/JUNIOR",
		"6252001/INFORMATION_TECHNOLOGY/VP_PRINCIPAL",
		"6252001/INSURANCE/",
		"2921044//",
	}
	if strings.Join(walked, " ") != strings.Join(want, " ") {
		t.Errorf("walked %v, want %v", walked, want)
	}
}

func TestEFinancialCareersEmptyListingFails(t *testing.T) {
	s := efinancialcareers{http: &efcHTTP{t: t, total: 0}, limiter: noWait{}}
	if err := s.FetchStream(context.Background(), CompanyEntry{}, func(Job) {}); err == nil {
		t.Fatal("a run that listed nothing must fail")
	}
}
