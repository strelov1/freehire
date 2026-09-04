package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// edjoinTestBoard is the job type sources/edjoin.yml crawls: EDJOIN's own technology facet.
const edjoinTestBoard = "25"

// edjoinTestRow is one posting as the fixtures describe it. A zero field means the district
// left that box empty, which is what most of them do.
type edjoinTestRow struct {
	id                     int
	title, district, city  string
	salaryMode, from, to   string
	period, schedule, date string
}

// edjoinRowJSON renders one /Home/LoadJobs row the way the endpoint does, mixed casing and
// .NET date wrappers included. The keys are written out rather than marshalled from
// edjoinPosting, so a wrong struct tag fails a test instead of round-tripping through it. Only
// the fields the adapter reads are rendered; the live payload carries some forty more, all of
// them null or zero on every posting sampled.
func edjoinRowJSON(r edjoinTestRow) string {
	row := map[string]any{
		"postingID":          r.id,
		"positionTitle":      r.title,
		"districtName":       r.district,
		"city":               r.city,
		"countyName":         "Santa Clara",
		"stateName":          "California",
		"postingDate":        r.date,
		"FullTimePartTime":   r.schedule,
		"SalaryInfoSelect":   r.salaryMode,
		"PayRangeFrom":       "",
		"PayRangeTo":         "",
		"PayRangeDropdown":   "",
		"SingleRate":         "",
		"SingleRateDropdown": "",
		// The listing states no body at all; both fields are null on every live posting.
		"postingInformation": nil,
		"JobSummary":         nil,
	}
	switch r.salaryMode {
	case "Pay Range":
		row["PayRangeFrom"], row["PayRangeTo"], row["PayRangeDropdown"] = r.from, r.to, r.period
	case "Single Rate":
		row["SingleRate"], row["SingleRateDropdown"] = r.from, r.period
	}
	b, err := json.Marshal(row)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// edjoinListingJSON renders a listing page around the given rows. totalRecords is the whole
// slice's exact count, which the endpoint repeats on every page.
func edjoinListingJSON(total int, rows ...string) string {
	return fmt.Sprintf(`{"search":{"jobTypes":%q},"totalPages":1,"totalRecords":%d,`+
		`"totalOpenings":0,"displayRecords":%d,"data":[%s]}`,
		edjoinTestBoard, total, len(rows), strings.Join(rows, ","))
}

// edjoinPostingHTML renders a posting page the way edjoin.org does: a schema.org JobPosting in
// an ld+json block, alongside the page's own markup. The description arrives as escaped HTML
// inside the JSON string, which is how the platform emits it.
func edjoinPostingHTML(title, district, description string) string {
	ld, err := json.Marshal(map[string]any{
		"@context":    "https://schema.org/",
		"@type":       "JobPosting",
		"title":       title,
		"description": description,
		"identifier":  map[string]any{"@type": "PropertyValue", "name": district, "value": 2243913},
		"hiringOrganization": map[string]any{
			"@type": "Organization", "name": district,
			"sameAs": "https://www.edjoin.org/milpitasunified",
		},
		"datePosted":   "2026-07-01T07:00:00Z",
		"validThrough": "2027-07-01T07:00:00Z",
		// Both are verbatim excerpts of description on every live posting sampled, so the
		// adapter must not append them a second time.
		"experienceRequirements": "A Bachelor's Degree.",
		"employerOverview":       "The district serves 10,000 students.",
	})
	if err != nil {
		panic(err)
	}
	return `<html><head><title>` + title + ` at ` + district + ` | EDJOIN</title>
<script type="application/ld+json">` + string(ld) + `</script>
</head><body><h1>` + title + `</h1></body></html>`
}

// edjoinFixture wires a one-page slice holding one posting, plus that posting's page.
func edjoinFixture() *routedHTTP {
	return (&routedHTTP{}).
		route("page=1", edjoinListingJSON(1, edjoinRowJSON(edjoinTestRow{
			id: 2243913, title: " Network Technician ", city: "Alhambra",
			district:   "Alhambra Unified School District",
			salaryMode: "Pay Range", from: "$24.51", to: "$32.13", period: "Per Hour",
			schedule: "Full Time", date: "/Date(1782864000000)/",
		}))).
		route("/Home/JobPosting/2243913", edjoinPostingHTML(
			"Network Technician", "Alhambra Unified School District",
			"<p><strong>Job Summary</strong></p><p>Maintain the district network.</p>"))
}

func edjoinEntry() CompanyEntry {
	return CompanyEntry{
		Company:  "EDJOIN — Information technology",
		Provider: "edjoin",
		Board:    edjoinTestBoard,
	}
}

func TestEdjoinProvider(t *testing.T) {
	if got := NewEdjoin(nil).Provider(); got != "edjoin" {
		t.Errorf("Provider() = %q, want %q", got, "edjoin")
	}
}

func TestEdjoinRegisteredAndFacet(t *testing.T) {
	if _, ok := All(nil)["edjoin"]; !ok {
		t.Fatal("edjoin not registered in sources.All")
	}
	// The board selects a slice of one central index, not a tenant, so edjoin is board-keyed
	// and — because every district on it is its own employer — an aggregator.
	if !slices.Contains(BoardKeyedProviders(Taxonomy()), "edjoin") {
		t.Error("edjoin should be board-keyed")
	}
	if !slices.Contains(AggregatorProviders(Taxonomy()), "edjoin") {
		t.Error("edjoin should be an aggregator")
	}
	if !slices.Contains(FilterableProviders(), "edjoin") {
		t.Error("edjoin should appear in the source facet")
	}
	// The listing carries no body, so the adapter must hydrate only what the catalogue lacks.
	if _, ok := All(nil)["edjoin"].(HydratingSource); !ok {
		t.Error("edjoin should be a HydratingSource")
	}
}

// The endpoint dereferences catID, districtID and recruitmentCenterID without a null check and
// answers a 500 error page when any is missing, so the URL builder must always send all three.
func TestEdjoinListingURLCarriesMandatoryFilters(t *testing.T) {
	got := edjoinListingURL(edjoinTestBoard, 3)
	for _, want := range []string{
		"catID=0", "districtID=0", "recruitmentCenterID=0",
		"jobTypes=25", "page=3", "rows=500",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("listing URL %q is missing %q", got, want)
		}
	}
}

func TestEdjoinFetchMapsListingAndDetail(t *testing.T) {
	jobs, err := NewEdjoin(edjoinFixture()).Fetch(context.Background(), edjoinEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "2243913" {
		t.Errorf("ExternalID = %q, want 2243913", j.ExternalID)
	}
	if j.URL != "https://www.edjoin.org/Home/JobPosting/2243913" {
		t.Errorf("URL = %q", j.URL)
	}
	// The listing pads titles with spaces on roughly one posting in seven.
	if j.Title != "Network Technician" {
		t.Errorf("Title = %q, want the trimmed title", j.Title)
	}
	// The employer is the district on the posting, never the board file's slice label.
	if j.Company != "Alhambra Unified School District" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Alhambra, California" {
		t.Errorf("Location = %q, want %q", j.Location, "Alhambra, California")
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.Remote || j.WorkMode != "" {
		t.Errorf("Remote/WorkMode = %v/%q, want false and no claim", j.Remote, j.WorkMode)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 25 || j.SalaryMax == nil || *j.SalaryMax != 32 {
		t.Errorf("salary bounds = %v/%v, want 25/32", j.SalaryMin, j.SalaryMax)
	}
	if j.SalaryCurrency != "USD" || j.SalaryPeriod != "hour" {
		t.Errorf("salary = %s/%s, want USD/hour", j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-07-01" {
		t.Errorf("PostedAt = %v, want 2026-07-01", j.PostedAt)
	}
	if !strings.Contains(j.Description, "Maintain the district network.") {
		t.Errorf("Description = %q", j.Description)
	}
	// experienceRequirements and employerOverview are excerpts of the description or district
	// boilerplate; reading description alone is what keeps them out.
	if strings.Contains(j.Description, "10,000 students") {
		t.Error("description must not carry the district's employerOverview boilerplate")
	}
}

func TestEdjoinFetchNewSkipsSeenPostings(t *testing.T) {
	fixture := edjoinFixture()
	jobs, err := NewEdjoin(fixture).(HydratingSource).
		FetchNew(context.Background(), edjoinEntry(), func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if !jobs[0].SeenRefresh {
		t.Error("a seen posting should come back flagged SeenRefresh")
	}
	if jobs[0].Description != "" {
		t.Error("a seen posting must not be re-hydrated")
	}
	// The title travels with the refresh so the pipeline can re-apply the catalogue filter.
	if jobs[0].Title != "Network Technician" {
		t.Errorf("Title = %q", jobs[0].Title)
	}
	// One listing page — the slice's exact total ends the walk — and no detail request.
	if fixture.calls != 1 {
		t.Errorf("made %d requests, want the listing alone and no detail", fixture.calls)
	}
}

func TestEdjoinFetchDropsPostingWhoseDetailFails(t *testing.T) {
	// No route for the posting page: the listing carries no body to fall back on, and a
	// stored body-less row would never be hydrated again.
	fixture := (&routedHTTP{}).
		route("page=1", edjoinListingJSON(1, edjoinRowJSON(edjoinTestRow{
			id: 2243913, title: "Network Technician", city: "Alhambra",
			district: "Alhambra Unified School District", salaryMode: "Dependent",
		})))
	jobs, err := NewEdjoin(fixture).Fetch(context.Background(), edjoinEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("got %d jobs, want the posting deferred to the next crawl", len(jobs))
	}
}

func TestEdjoinListWalksPagesAndDedups(t *testing.T) {
	row := func(id int) string {
		return edjoinRowJSON(edjoinTestRow{
			id: id, title: "IT Specialist", district: "Madera Unified School District",
			city: "Madera",
		})
	}
	// The same posting can land on two adjacent pages, so the walk dedups on postingID and
	// stops once it holds the slice's exact total.
	fixture := (&routedHTTP{}).
		route("page=1", edjoinListingJSON(3, row(1), row(2))).
		route("page=2", edjoinListingJSON(3, row(2), row(3))).
		route("page=3", edjoinListingJSON(3, row(4)))
	postings, err := NewEdjoin(fixture).(edjoin).list(context.Background(), edjoinEntry())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(postings) != 3 {
		t.Fatalf("got %d postings, want 3 deduped", len(postings))
	}
	if fixture.calls != 2 {
		t.Errorf("made %d requests, want the walk to stop on totalRecords", fixture.calls)
	}
}

func TestEdjoinListSkipsUnusableRows(t *testing.T) {
	fixture := (&routedHTTP{}).
		route("page=1", edjoinListingJSON(0,
			edjoinRowJSON(edjoinTestRow{id: 0, title: "No id", district: "Madera Unified"}),
			edjoinRowJSON(edjoinTestRow{id: 7, title: "No district", district: "  "}),
			edjoinRowJSON(edjoinTestRow{id: 8, title: "Fine", district: "Madera Unified"}))).
		route("page=2", edjoinListingJSON(0))
	postings, err := NewEdjoin(fixture).(edjoin).list(context.Background(), edjoinEntry())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(postings) != 1 || postings[0].PostingID != 8 {
		t.Errorf("got %+v, want only the row carrying both an id and a district", postings)
	}
}

func TestEdjoinListPageFailures(t *testing.T) {
	// The first page failing is a board-level error.
	if _, err := NewEdjoin(&routedHTTP{}).(edjoin).list(context.Background(), edjoinEntry()); err == nil {
		t.Error("a failing first page should fail the board")
	}
	// A later page failing ends the walk with what was already gathered.
	row := edjoinRowJSON(edjoinTestRow{id: 1, title: "IT Specialist", district: "Madera Unified"})
	fixture := (&routedHTTP{}).route("page=1", edjoinListingJSON(99, row))
	postings, err := NewEdjoin(fixture).(edjoin).list(context.Background(), edjoinEntry())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(postings) != 1 {
		t.Errorf("got %d postings, want page 1's to survive page 2 failing", len(postings))
	}
}

// The pay fields are free-text boxes a district types into, so the pattern is the safeguard.
func TestEdjoinAmount(t *testing.T) {
	amount := map[string]int{
		"$25.04":      25,
		"$116,336.37": 116336,
		"6,097.87":    6098,
		"60000":       60000,
		"$32":         32,
		" $24.40 ":    24,
	}
	for in, want := range amount {
		got := edjoinAmount(in)
		if got == nil || *got != want {
			t.Errorf("edjoinAmount(%q) = %v, want %d", in, got, want)
		}
	}
	for _, in := range []string{
		"", "TBD", "Stipend", "Seasonal", "UNPAID", "District Salary Schedule",
		"Category I: $3,402", "Range 1: $18.30", "$22 - $26/hr", "$26.61/hour",
		"$33.11 [STEP 1]", "$ 21.88", "$24.300", "$0", "Range 15",
	} {
		if got := edjoinAmount(in); got != nil {
			t.Errorf("edjoinAmount(%q) = %d, want nil", in, *got)
		}
	}
}

func TestEdjoinApplySalary(t *testing.T) {
	cases := []struct {
		name             string
		p                edjoinPosting
		wantMin, wantMax int
		wantPeriod       string
	}{
		{
			name: "pay range",
			p: edjoinPosting{SalaryInfoSelect: "Pay Range", PayRangeFrom: "$6,033.00",
				PayRangeTo: "$7,963.00", PayRangeDropdown: "Monthly"},
			wantMin: 6033, wantMax: 7963, wantPeriod: "month",
		},
		{
			// A single rate is one amount, so it lands on both bounds.
			name: "single rate",
			p: edjoinPosting{SalaryInfoSelect: "Single Rate", SingleRate: "$24.40",
				SingleRateDropdown: "Per Hour"},
			wantMin: 24, wantMax: 24, wantPeriod: "hour",
		},
		{
			name: "annual range",
			p: edjoinPosting{SalaryInfoSelect: "Pay Range", PayRangeFrom: "93,766.40",
				PayRangeTo: "122,345.60", PayRangeDropdown: "Annually"},
			wantMin: 93766, wantMax: 122346, wantPeriod: "year",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var j Job
			tc.p.applySalary(&j)
			if j.SalaryMin == nil || *j.SalaryMin != tc.wantMin {
				t.Errorf("SalaryMin = %v, want %d", j.SalaryMin, tc.wantMin)
			}
			if j.SalaryMax == nil || *j.SalaryMax != tc.wantMax {
				t.Errorf("SalaryMax = %v, want %d", j.SalaryMax, tc.wantMax)
			}
			if j.SalaryCurrency != "USD" || j.SalaryPeriod != tc.wantPeriod {
				t.Errorf("salary = %s/%s, want USD/%s", j.SalaryCurrency, j.SalaryPeriod, tc.wantPeriod)
			}
		})
	}

	// A posting the district published no figure for, and one whose period freehire has no
	// value for, both yield nothing rather than a half-qualified amount.
	for _, p := range []edjoinPosting{
		{SalaryInfoSelect: "Dependent", PayRangeFrom: "$25.04", PayRangeDropdown: "Per Hour"},
		{SalaryInfoSelect: "Pay Range", PayRangeFrom: "$1,500", PayRangeTo: "$1,500",
			PayRangeDropdown: "Bi-weekly"},
		{SalaryInfoSelect: "Single Rate", SingleRate: "Stipend", SingleRateDropdown: "Stipend"},
	} {
		var j Job
		p.applySalary(&j)
		if j.SalaryMin != nil || j.SalaryMax != nil || j.SalaryPeriod != "" {
			t.Errorf("applySalary(%+v) wrote %v/%v/%q, want nothing",
				p, j.SalaryMin, j.SalaryMax, j.SalaryPeriod)
		}
	}
}

// The FullTimePartTime multi-select states the schedule AND the remote flag, so both readings
// go through one parse — otherwise a value naming both ("Part Time, Remote") is read as remote
// by one and as no schedule at all by the other. Every value below is a live spelling.
func TestEdjoinScheduleFields(t *testing.T) {
	cases := map[string]struct {
		employment string
		remote     bool
	}{
		"Full Time": {employment: "full_time"},
		"Part Time": {employment: "part_time"},
		// A schedule ticked beside a duration, a pay class or the remote flag still names
		// that schedule — those options are orthogonal to it, not alternatives to it.
		"Part Time, Temporary":  {employment: "part_time"},
		"Full Time, Temporary":  {employment: "full_time"},
		"Full Time, Management": {employment: "full_time"},
		"Part Time, Remote":     {employment: "part_time", remote: true},
		"Temporary, Remote":     {remote: true},
		// Both schedules ticked — including the district's one-word spelling of that — means
		// the position may be either, so it names no single type.
		"Full Time, Part Time":            {},
		"Full Time, Part Time, Temporary": {},
		"Full and Part Time":              {},
		// A duration or a pay class alone names no schedule.
		"Temporary":  {},
		"Management": {},
		"":           {},
	}
	for in, want := range cases {
		schedule := edjoinSchedule(in)
		if got := edjoinEmploymentType(schedule); got != want.employment {
			t.Errorf("employment type of %q = %q, want %q", in, got, want.employment)
		}
		if got := edjoinRemote(schedule); got != want.remote {
			t.Errorf("remote of %q = %v, want %v", in, got, want.remote)
		}
	}
}

func TestEdjoinSalaryPeriod(t *testing.T) {
	period := map[string]string{
		"Per Hour": "hour", "Daily": "day", "Monthly": "month", "Annually": "year",
		// EDJOIN's remaining periods have no freehire value.
		"Stipend": "", "Bi-weekly": "", "Semi-Monthly": "", "": "",
	}
	for in, want := range period {
		if got := edjoinSalaryPeriod(in); got != want {
			t.Errorf("edjoinSalaryPeriod(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEdjoinDate(t *testing.T) {
	got := edjoinDate("/Date(1782864000000)/")
	if got == nil || got.UTC().Format("2006-01-02") != "2026-07-01" {
		t.Errorf("edjoinDate = %v, want 2026-07-01", got)
	}
	for _, in := range []string{
		"",
		"2026-07-01",
		// .NET's DateTime.MinValue: the field was never set, not a date in year 1.
		"/Date(-62135568000000)/",
		"/Date(0)/",
		"/Date(nope)/",
	} {
		if got := edjoinDate(in); got != nil {
			t.Errorf("edjoinDate(%q) = %v, want nil", in, got)
		}
	}
}

func TestEdjoinLocation(t *testing.T) {
	cases := map[edjoinPosting]string{
		{City: "Alhambra", CountyName: "Los Angeles", StateName: "California"}: "Alhambra, California",
		// Roughly one posting in sixty states no city; the county is then the narrowest place.
		{City: "  ", CountyName: "Los Angeles", StateName: "California"}: "Los Angeles, California",
		{City: "", CountyName: "", StateName: "California"}:              "California",
	}
	for p, want := range cases {
		if got := p.location(); got != want {
			t.Errorf("location(%+v) = %q, want %q", p, got, want)
		}
	}
}
