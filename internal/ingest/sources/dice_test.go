package sources

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// diceFixtures holds the trimmed live responses the tests serve.
//
//go:embed testdata/dice
var diceFixtures embed.FS

// The fixtures are trimmed from live dice.com responses fetched on 2026-11-17: five result rows
// of the search page's flight payload verbatim (a salaried on-site posting, an hourly contract,
// a remote one with no location, a "Remote, On-Site" one with a bare figure for salary, and one
// stating "Depends on Experience"; the summary cut), and the ld+json block of a posting page.

func diceFixture(t *testing.T, name string) *html.Node {
	t.Helper()
	b, err := diceFixtures.ReadFile("testdata/dice/" + name)
	if err != nil {
		t.Fatal(err)
	}
	root, err := html.Parse(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// diceFake serves the search by its query and posting pages by URL; an unknown posting is a 404.
type diceFake struct {
	mu sync.Mutex
	// search answers a search request, given its filters and page number.
	search func(filters url.Values, page int) (string, error)
	pages  map[string]string
	errs   map[string]error
	got    []string
}

func (f *diceFake) GetHTML(_ context.Context, raw string) (*html.Node, error) {
	f.mu.Lock()
	f.got = append(f.got, raw)
	f.mu.Unlock()
	if err, ok := f.errs[raw]; ok {
		return nil, err
	}
	if u, err := url.Parse(raw); err == nil && u.Path == "/jobs" {
		q := u.Query()
		page := 1
		if p := q.Get("page"); p != "" {
			fmt.Sscan(p, &page)
		}
		q.Del("page")
		body, err := f.search(q, page)
		if err != nil {
			return nil, err
		}
		return html.Parse(strings.NewReader(body))
	}
	body, ok := f.pages[raw]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: raw}
	}
	return html.Parse(strings.NewReader(body))
}

func (f *diceFake) requested(url string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.got, url)
}

// diceSearchPage renders a search page the way the site does: the result list as one JSON
// string chunk of the flight payload.
func diceSearchPage(guids []string, total, pageCount int) string {
	rows := make([]map[string]any, 0, len(guids))
	for _, g := range guids {
		rows = append(rows, map[string]any{
			"guid": g, "title": "Engineer " + g, "companyName": "Acme " + g,
			"detailsPageUrl": diceBase + "/job-detail/" + g, "postedDate": "2026-11-17T06:15:46Z",
		})
	}
	list, _ := json.Marshal(map[string]any{
		"data": rows,
		"meta": map[string]any{"pageCount": pageCount, "totalResults": total},
	})
	chunk, _ := json.Marshal(`8e:["$","$L8f",null,{"jobList":` + string(list) + "}]\n")
	return `<html><body><script>self.__next_f.push([1,` + string(chunk) + `])</script></body></html>`
}

const (
	diceSAIC = diceBase + "/job-detail/97cb0975-8cf7-4027-b204-9e324431f2fe"
)

func TestDiceResultsReadTheRowsAndTheTotal(t *testing.T) {
	res, ok := parseDiceResults(diceFixture(t, "list.html"))
	if !ok {
		t.Fatal("result list not read")
	}
	if len(res.Data) != 5 || res.Meta.TotalResults != 111962 || res.Meta.PageCount != 3733 {
		t.Fatalf("rows = %d total = %d pages = %d", len(res.Data), res.Meta.TotalResults, res.Meta.PageCount)
	}
	if got := res.Data[0]; got.GUID != "97cb0975-8cf7-4027-b204-9e324431f2fe" || got.CompanyName == "" || got.Title != "Senior Systems Administrator" {
		t.Errorf("first row = %+v", got)
	}

	if res, ok := parseDiceResults(diceFixture(t, "list_empty.html")); !ok || len(res.Data) != 0 || res.Meta.TotalResults != 0 {
		t.Errorf("an empty slice: ok=%v res=%+v", ok, res)
	}
	// A page without the list is a changed markup, not an empty slice.
	if _, ok := parseDiceResults(diceFixture(t, "list_broken.html")); ok {
		t.Error("a page with no jobList read as a result list")
	}
}

func TestDiceRowMapsTheListingsFacts(t *testing.T) {
	res, _ := parseDiceResults(diceFixture(t, "list.html"))
	jobs := make([]Job, len(res.Data))
	for i, r := range res.Data {
		jobs[i] = r.job()
	}

	sa := jobs[0]
	if sa.ExternalID != "97cb0975-8cf7-4027-b204-9e324431f2fe" || sa.URL != diceSAIC {
		t.Errorf("identity = %q %q", sa.ExternalID, sa.URL)
	}
	if sa.Location != "Kirtland Air Force Base, New Mexico, USA" || !slices.Equal(sa.Countries, []string{"us"}) {
		t.Errorf("location = %q countries = %v", sa.Location, sa.Countries)
	}
	if sa.WorkMode != "onsite" || sa.Remote || sa.EmploymentType != "full_time" {
		t.Errorf("work mode = %q remote = %v type = %q", sa.WorkMode, sa.Remote, sa.EmploymentType)
	}
	if sa.SalaryMin == nil || *sa.SalaryMin != 100001 || sa.SalaryMax == nil || *sa.SalaryMax != 140000 ||
		sa.SalaryCurrency != "USD" || sa.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", sa.SalaryMin, sa.SalaryMax, sa.SalaryCurrency, sa.SalaryPeriod)
	}
	if sa.PostedAt == nil || sa.PostedAt.Format("2006-01-02") != "2026-11-17" {
		t.Errorf("posted = %v", sa.PostedAt)
	}

	if h := jobs[1]; h.EmploymentType != "contract" || h.SalaryPeriod != "hour" || *h.SalaryMin != 35 || *h.SalaryMax != 40 {
		t.Errorf("hourly contract = %q %v-%v/%s", h.EmploymentType, h.SalaryMin, h.SalaryMax, h.SalaryPeriod)
	}
	// A remote posting states no location at all.
	if r := jobs[2]; r.WorkMode != "remote" || !r.Remote || r.Location != "" || r.Countries != nil {
		t.Errorf("remote = %q %v location %q countries %v", r.WorkMode, r.Remote, r.Location, r.Countries)
	}
	// Two arrangements state none; "Third Party" is set aside and leaves full time; a bare
	// figure states no period.
	if m := jobs[3]; m.WorkMode != "" || m.Remote || m.EmploymentType != "full_time" || m.SalaryMin != nil {
		t.Errorf("remote or on-site = %q %v %q salary %v", m.WorkMode, m.Remote, m.EmploymentType, m.SalaryMin)
	}
	if d := jobs[4]; d.SalaryMin != nil || d.SalaryCurrency != "" {
		t.Errorf("depends on experience read as a salary: %v", d.SalaryMin)
	}
}

func TestDiceSalaryLabels(t *testing.T) {
	cases := []struct {
		label      string
		lo, hi     int
		cur, per   string
		stateSalar bool
	}{
		{"USD 140,001.00 - 180,000.00 per year", 140001, 180000, "USD", "year", true},
		{"USD 109,464.00 per year", 109464, 109464, "USD", "year", true},
		{"USD 35.00 - 38.42 per hour", 35, 38, "USD", "hour", true},
		{"120000", 0, 0, "", "", false},
		{"$$70 - $89", 0, 0, "", "", false},
		{"USD105000 - USD150000 per annum", 0, 0, "", "", false},
		{"Depends on Experience", 0, 0, "", "", false},
		{"USD 90,000.00 - 60,000.00 per year", 0, 0, "", "", false},
		{"", 0, 0, "", "", false},
	}
	for _, c := range cases {
		var j Job
		diceSalary(&j, c.label)
		if !c.stateSalar {
			if j.SalaryMin != nil || j.SalaryMax != nil || j.SalaryCurrency != "" || j.SalaryPeriod != "" {
				t.Errorf("%q read as a salary: %v-%v", c.label, j.SalaryMin, j.SalaryMax)
			}
			continue
		}
		if j.SalaryMin == nil || j.SalaryMax == nil || *j.SalaryMin != c.lo || *j.SalaryMax != c.hi || j.SalaryCurrency != c.cur || j.SalaryPeriod != c.per {
			t.Errorf("%q = %v-%v %s/%s", c.label, j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
		}
	}
}

func TestDiceEmploymentTypes(t *testing.T) {
	for label, want := range map[string]string{
		"Full-time":                        "full_time",
		"Part-time":                        "part_time",
		"Contract":                         "contract",
		"Contract, Third Party":            "contract",
		"Third Party, Contract":            "contract",
		"Third Party":                      "",
		"Full-time, Part-time, Contract":   "",
		"":                                 "",
		"Full-time, Third Party, Contract": "",
	} {
		if got := diceEmploymentType(label); got != want {
			t.Errorf("%q = %q, want %q", label, got, want)
		}
	}
}

func TestDicePostingCompletesTheBody(t *testing.T) {
	base := diceRow{GUID: "97cb0975-8cf7-4027-b204-9e324431f2fe", URL: diceSAIC, Title: "Senior Systems Administrator", CompanyName: "SAIC"}.job()
	job, ok := parseDicePosting(diceFixture(t, "detail.html"), base)
	if !ok {
		t.Fatal("posting not parsed")
	}
	if !strings.Contains(job.Description, "SAIC is seeking a Space Protection Mission Engineer") {
		t.Errorf("description = %.200q", job.Description)
	}
	if job.Title != base.Title || job.Company != "SAIC" {
		t.Errorf("title/company = %q / %q", job.Title, job.Company)
	}
	// The listing had no date, so the page's states it.
	if job.PostedAt == nil || job.PostedAt.Format("2006-01-02") != "2026-11-15" {
		t.Errorf("posted = %v", job.PostedAt)
	}
	if _, ok := parseDicePosting(diceFixture(t, "detail_no_description.html"), base); ok {
		t.Error("a posting with no body was kept")
	}
}

func TestDiceWindowUnderTheCeilingIsOneSliceWalkedToItsEnd(t *testing.T) {
	f := &diceFake{search: func(filters url.Values, page int) (string, error) {
		if filters.Get("filters.postedDate") != "ONE" || len(filters) != 1 {
			t.Errorf("filters = %v", filters)
		}
		switch page {
		case 1:
			return diceSearchPage([]string{"a", "b"}, 4, 2), nil
		case 2:
			return diceSearchPage([]string{"b", "c"}, 4, 2), nil // b again: a repeat across pages
		}
		t.Errorf("page %d requested past the page count", page)
		return "", nil
	}}
	rows, err := dice{http: f}.list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := diceGUIDs(rows); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("listed %v", got)
	}
}

func diceGUIDs(rows []diceRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.GUID)
	}
	return out
}

func TestDiceOversizedSliceIsCutByTheNextFilterAndRepeatsCollapse(t *testing.T) {
	f := &diceFake{search: func(filters url.Values, page int) (string, error) {
		switch filters.Get("filters.employerType") {
		case "":
			return diceSearchPage([]string{"x"}, diceMaxSlice+1, 26), nil
		case "Direct Hire":
			if filters.Get("filters.postedDate") != "ONE" {
				t.Errorf("the cut lost the window: %v", filters)
			}
			return diceSearchPage([]string{"a", "b"}, 2, 1), nil
		case "Recruiter":
			return diceSearchPage([]string{"b", "c"}, 2, 1), nil
		case "Other":
			return diceSearchPage(nil, 0, 0), nil
		}
		return "", nil
	}}
	rows, err := dice{http: f}.list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The oversized probe itself lists nothing: its postings are read through its children.
	if got := diceGUIDs(rows); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("listed %v", got)
	}
	if !f.requested(diceBase + "/jobs?filters.employerType=Direct%20Hire&filters.postedDate=ONE") {
		t.Errorf("requests = %v", f.got)
	}
}

func TestDiceSliceThatStaysOversizedAfterEveryCutIsWalkedToTheCeiling(t *testing.T) {
	f := &diceFake{search: func(filters url.Values, page int) (string, error) {
		if page > diceMaxPages {
			t.Errorf("page %d is past the search's ceiling", page)
		}
		return diceSearchPage([]string{fmt.Sprintf("g%d-%s", page, filters.Encode())}, 100000, 3333), nil
	}}
	rows, err := dice{http: f}.list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("nothing listed")
	}
	for _, u := range f.got {
		if strings.Contains(u, "page=26") {
			t.Errorf("walked past the ceiling: %s", u)
		}
	}
}

func TestDiceFailures(t *testing.T) {
	t.Run("the first page of the window fails the run", func(t *testing.T) {
		f := &diceFake{search: func(url.Values, int) (string, error) {
			return "", &StatusError{Method: "GET", Code: 503, URL: "x"}
		}}
		if _, err := (dice{http: f}).list(context.Background()); err == nil {
			t.Error("a run that read nothing succeeded")
		}
	})
	t.Run("a window with no posting is a changed page", func(t *testing.T) {
		f := &diceFake{search: func(url.Values, int) (string, error) { return diceSearchPage(nil, 0, 0), nil }}
		if _, err := (dice{http: f}).list(context.Background()); err == nil {
			t.Error("an empty window succeeded")
		}
	})
	t.Run("a later page failing ends its slice with what it gathered", func(t *testing.T) {
		f := &diceFake{search: func(_ url.Values, page int) (string, error) {
			if page == 2 {
				return "", &StatusError{Method: "GET", Code: 503, URL: "x"}
			}
			return diceSearchPage([]string{"a"}, 60, 2), nil
		}}
		rows, err := dice{http: f}.list(context.Background())
		if err != nil || len(rows) != 1 {
			t.Errorf("rows = %v err = %v", diceGUIDs(rows), err)
		}
	})
	t.Run("a skipped slice does not fail the run", func(t *testing.T) {
		f := &diceFake{search: func(filters url.Values, _ int) (string, error) {
			switch filters.Get("filters.employerType") {
			case "":
				return diceSearchPage(nil, diceMaxSlice+1, 26), nil
			case "Recruiter":
				return "", &StatusError{Method: "GET", Code: 503, URL: "x"}
			}
			return diceSearchPage([]string{"a"}, 1, 1), nil
		}}
		rows, err := dice{http: f}.list(context.Background())
		if err != nil || len(rows) != 1 {
			t.Errorf("rows = %v err = %v", diceGUIDs(rows), err)
		}
	})
}

func diceHydrationFake(t *testing.T) *diceFake {
	t.Helper()
	b, err := diceFixtures.ReadFile("testdata/dice/detail.html")
	if err != nil {
		t.Fatal(err)
	}
	return &diceFake{
		search: func(_ url.Values, page int) (string, error) {
			if page > 1 {
				return "", nil
			}
			return diceSearchPage([]string{"new1", "old", "cov", "rej", "new2"}, 5, 1), nil
		},
		pages: map[string]string{
			diceBase + "/job-detail/new1": string(b),
			diceBase + "/job-detail/new2": string(b),
		},
	}
}

func TestDiceHydratesOnlyWhatThePipelineKeeps(t *testing.T) {
	f := diceHydrationFake(t)
	gate := DetailGate{
		Covered: func([]string) map[string]bool { return map[string]bool{"Acme cov": true, "Acme rej": true} },
	}
	jobs, err := dice{http: f, maxNewPerRun: 10}.FetchNewGated(context.Background(), CompanyEntry{},
		func(id string) bool { return id == "old" }, gate)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(byID) != 5 {
		t.Fatalf("yielded %v", byID)
	}
	if !byID["old"].SeenRefresh || byID["old"].Description != "" {
		t.Errorf("a stored posting = %+v", byID["old"])
	}
	for _, id := range []string{"cov", "rej"} {
		if byID[id].Description != "" || byID[id].SeenRefresh || f.requested(diceBase+"/job-detail/"+id) {
			t.Errorf("%s was bought or marked seen: %+v", id, byID[id])
		}
	}
	for _, id := range []string{"new1", "new2"} {
		if !strings.Contains(byID[id].Description, "Space Protection Mission Engineer") {
			t.Errorf("%s has no body", id)
		}
	}
}

func TestDiceHydrationBudgetDefersTheRest(t *testing.T) {
	f := diceHydrationFake(t)
	jobs, err := dice{http: f, maxNewPerRun: 1}.FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id == "old" })
	if err != nil {
		t.Fatal(err)
	}
	// One of the four new postings is read; cov and rej are not told apart without a gate, so
	// they are new too and wait.
	var bodies int
	for _, j := range jobs {
		if j.Description != "" {
			bodies++
		}
	}
	if bodies != 1 {
		t.Errorf("%d bodies read within a budget of 1", bodies)
	}
}

func TestDiceUnreadablePostingsAreDeferredNotStoredBodyless(t *testing.T) {
	f := diceHydrationFake(t)
	f.pages = map[string]string{diceBase + "/job-detail/new1": "<html><body>no posting</body></html>"}
	f.errs = map[string]error{diceBase + "/job-detail/new2": &StatusError{Method: "GET", Code: 429, URL: "x"}}
	_, err := dice{http: f, maxNewPerRun: 10}.FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id == "old" || id == "cov" || id == "rej" })
	if err == nil {
		t.Error("a run that needed bodies and read none succeeded")
	}
}

func TestDiceIsABoardlessAggregatorWithItsOwnSweepGrace(t *testing.T) {
	var s Source = NewDice(&diceFake{})
	if _, ok := s.(interface{ boardless() }); !ok {
		t.Error("not boardless")
	}
	if _, ok := s.(interface{ aggregator() }); !ok {
		t.Error("not an aggregator")
	}
	if _, ok := s.(CoverageGated); !ok {
		t.Error("not coverage gated")
	}
	if g, ok := s.(interface{ sweepGrace() time.Duration }); !ok || g.sweepGrace() != diceSweepGrace {
		t.Error("no sweep grace")
	}
}
