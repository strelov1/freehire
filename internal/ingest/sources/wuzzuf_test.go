package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// wuzzufFakeHTTP serves the trimmed real data (2026-10-18) the way wuzzuf.net's search page
// carries it: each page embeds the store state its front end boots from, holding that page's
// slice of the IT/Software Development search (by the start parameter, pageSize rows a page)
// and the job documents of exactly those rows.
type wuzzufFakeHTTP struct {
	t *testing.T
	// pageSize is the site's page size, wuzzufPageSize outside tests.
	pageSize int
	// mutateSearch and mutateJobs edit the decoded fixture before it is served.
	mutateSearch func(body map[string]any)
	mutateJobs   func(docs []any) []any
	// failSearchPage makes that page (0-based) error.
	failSearchPage int
	pages          []url.Values
}

func newWuzzufFake(t *testing.T) *wuzzufFakeHTTP {
	return &wuzzufFakeHTTP{t: t, pageSize: 3, failSearchPage: -1}
}

func (f *wuzzufFakeHTTP) fixture(name string) map[string]any {
	raw, err := os.ReadFile("testdata/wuzzuf/" + name)
	if err != nil {
		f.t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		f.t.Fatal(err)
	}
	return body
}

func (f *wuzzufFakeHTTP) GetText(_ context.Context, u string) (string, error) {
	parsed, err := url.Parse(u)
	if err != nil || !strings.HasPrefix(u, wuzzufSearchPageURL+"?") {
		return "", fmt.Errorf("unexpected GetText %s", u)
	}
	q := parsed.Query()
	page := len(f.pages)
	f.pages = append(f.pages, q)
	if page == f.failSearchPage {
		return "", errors.New("search page failed")
	}
	start, err := strconv.Atoi(q.Get("start"))
	if err != nil {
		return "", fmt.Errorf("start %q: %w", q.Get("start"), err)
	}
	search := f.fixture("search_it.json")
	if f.mutateSearch != nil {
		f.mutateSearch(search)
	}
	docs := f.fixture("jobs_it.json")["data"].([]any)
	if f.mutateJobs != nil {
		docs = f.mutateJobs(docs)
	}
	byID := map[string]any{}
	for _, d := range docs {
		byID[d.(map[string]any)["id"].(string)] = d
	}
	rows := search["data"].([]any)
	lo := min(start*f.pageSize, len(rows))
	hi := min(lo+f.pageSize, len(rows))
	order, set, collection := []any{}, map[string]any{}, map[string]any{}
	for _, r := range rows[lo:hi] {
		id := r.(map[string]any)["id"].(string)
		order = append(order, id)
		set[id] = r
		if d, ok := byID[id]; ok {
			collection[id] = d
		}
	}
	state := map[string]any{
		"jobSearch": map[string]any{"sets": map[string]any{parsed.RequestURI(): map[string]any{
			"start":             0,
			"totalResultsCount": search["meta"].(map[string]any)["totalResultsCount"],
			"resultsOrder":      order,
			"resultsSet":        set,
		}}},
		"entities": map[string]any{"job": map[string]any{"collection": collection}},
	}
	raw, _ := json.Marshal(state)
	return "<html><script>\n\t\t\tWuzzuf.initialStoreState = " + string(raw) +
		";\n\t\t\tWuzzuf.lang = \"en\";\n</script></html>", nil
}

const wuzzufITBoard = "IT/Software Development"

func wuzzufFetch(t *testing.T, f *wuzzufFakeHTTP) ([]Job, error) {
	t.Helper()
	s := wuzzuf{http: f, pageSize: f.pageSize}
	return s.Fetch(context.Background(), CompanyEntry{Board: wuzzufITBoard})
}

func wuzzufByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestWuzzufFetchMapsTheBoard(t *testing.T) {
	f := newWuzzufFake(t)
	jobs, err := wuzzufFetch(t, f)
	if err != nil {
		t.Fatal(err)
	}
	// Seven listed; the one whose employer is hidden names no company and is not yielded.
	if len(jobs) != 6 {
		t.Fatalf("got %d jobs, want 6", len(jobs))
	}
	// Three search pages of three (the last one short), every one filtered by the board's role.
	if len(f.pages) != 3 {
		t.Fatalf("search pages = %d, want 3", len(f.pages))
	}
	for i, q := range f.pages {
		if q.Get("start") != strconv.Itoa(i) || q.Get("filters[roles][0]") != wuzzufITBoard || len(q) != 2 {
			t.Errorf("page %d: %v", i, q)
		}
	}
	byID := wuzzufByID(jobs)
	if _, ok := byID["79aa9412-8b27-4254-ad52-d7bbdae30b97"]; ok {
		t.Error("the hidden-employer posting was yielded")
	}

	erp := byID["db64539c-1e1d-4c1d-9a37-60d455bbd6e5"]
	if erp.Title != "ERPNext Developer" || erp.Company != "Our Education" || !erp.IsTechHint {
		t.Errorf("identity: %q / %q tech=%v", erp.Title, erp.Company, erp.IsTechHint)
	}
	if erp.URL != "https://wuzzuf.net/jobs/p/oxiq6qk0egqn-erpnext-developer-our-education-cairo-egypt" {
		t.Errorf("url: %s", erp.URL)
	}
	if erp.Location != "Maadi, Cairo, Egypt" || len(erp.Countries) != 1 || erp.Countries[0] != "eg" {
		t.Errorf("geography: %q %v", erp.Location, erp.Countries)
	}
	if erp.WorkMode != "onsite" || erp.Remote || erp.EmploymentType != "full_time" {
		t.Errorf("arrangement: %q remote=%v %q", erp.WorkMode, erp.Remote, erp.EmploymentType)
	}
	// "10/18/2026 13:26:53" is Cairo wall time, three hours ahead of UTC in October.
	if want := time.Date(2026, 10, 18, 10, 26, 53, 0, time.UTC); erp.PostedAt == nil || !erp.PostedAt.Equal(want) {
		t.Errorf("posted: %v, want %v", erp.PostedAt, want)
	}
	if erp.ExperienceYearsMin == nil || *erp.ExperienceYearsMin != 2 || erp.Seniority != "" {
		t.Errorf("experience: %v seniority %q", erp.ExperienceYearsMin, erp.Seniority)
	}
	if !strings.Contains(erp.Description, "ERPNext") || !strings.Contains(erp.Description, "Requirements") {
		t.Errorf("description lacks the body or the requirements: %.200s", erp.Description)
	}
	if erp.SalaryCurrency != "" || erp.SalaryMin != nil {
		t.Errorf("hidden salary published: %v %q", erp.SalaryMin, erp.SalaryCurrency)
	}

	tam := byID["32742d1d-5290-4f46-95ef-81ad9764f97f"]
	if tam.WorkMode != "remote" || !tam.Remote {
		t.Errorf("remote: %q %v", tam.WorkMode, tam.Remote)
	}
	if tam.SalaryMin == nil || *tam.SalaryMin != 70000 || tam.SalaryMax == nil || *tam.SalaryMax != 80000 ||
		tam.SalaryCurrency != "EGP" || tam.SalaryPeriod != "month" {
		t.Errorf("salary: %v-%v %q %q", tam.SalaryMin, tam.SalaryMax, tam.SalaryCurrency, tam.SalaryPeriod)
	}

	// A posting filed under several work types states no single employment type.
	devops := byID["b103bf23-e666-423c-aec6-bca42dda6be4"]
	if devops.EmploymentType != "" || len(devops.Countries) != 1 || devops.Countries[0] != "cy" ||
		devops.Location != "Limassol, Cyprus" {
		t.Errorf("multi-type / Cyprus: %q %v %q", devops.EmploymentType, devops.Countries, devops.Location)
	}
	if !containsString(devops.Skills, "docker") || !containsString(devops.Skills, "linux") {
		t.Errorf("skills: %v", devops.Skills)
	}

	gip := byID["9ce6dcd4-b6d0-45ab-93ce-0a6fdd69bcb4"]
	if gip.Seniority != "intern" || gip.EmploymentType != "internship" ||
		len(gip.Countries) != 1 || gip.Countries[0] != "jp" {
		t.Errorf("internship: %q %q %v", gip.Seniority, gip.EmploymentType, gip.Countries)
	}
	if gip.SalaryCurrency != "JPY" || gip.SalaryMin == nil || *gip.SalaryMin != 150000 {
		t.Errorf("internship salary: %v %q", gip.SalaryMin, gip.SalaryCurrency)
	}
	// An internship lives under its own path, not /jobs/p/.
	if gip.URL != "https://wuzzuf.net/internship/tv75uk9ifozp-global-internship-program-gip-full-stack-pathway-hennge-tokyo-japan" {
		t.Errorf("internship url: %s", gip.URL)
	}

	// The search pads some employer names with spaces.
	if jr := byID["6fb744bf-8150-4211-b6b0-47df693bc4f3"]; jr.Seniority != "junior" || jr.Company != "Bowline Egypt" {
		t.Errorf("entry level: %q / %q", jr.Seniority, jr.Company)
	}
	if dxb := byID["c4c57a94-bb76-4f20-9e43-b2d28a7c4cad"]; len(dxb.Countries) != 1 || dxb.Countries[0] != "ae" {
		t.Errorf("Dubai: %v %q", dxb.Countries, dxb.Location)
	}
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestWuzzufRejectsABoardThatIsNotARoleName(t *testing.T) {
	f := newWuzzufFake(t)
	_, err := wuzzuf{http: f, pageSize: f.pageSize}.Fetch(context.Background(), CompanyEntry{Board: `IT"}`})
	if err == nil || len(f.pages) != 0 {
		t.Fatalf("err=%v pages=%d", err, len(f.pages))
	}
}

func TestWuzzufFailsWhenTheListingDisagreesWithItsCount(t *testing.T) {
	f := newWuzzufFake(t)
	f.mutateSearch = func(b map[string]any) { b["meta"].(map[string]any)["totalResultsCount"] = 8 }
	if _, err := wuzzufFetch(t, f); err == nil {
		t.Fatal("a listing one short of its count was accepted")
	}
}

func TestWuzzufFailsOnAnEmptyRole(t *testing.T) {
	f := newWuzzufFake(t)
	f.mutateSearch = func(b map[string]any) {
		b["meta"].(map[string]any)["totalResultsCount"] = 0
		b["data"] = []any{}
	}
	if _, err := wuzzufFetch(t, f); err == nil {
		t.Fatal("a role matching nothing read as an empty board")
	}
}

func TestWuzzufFailsOnARoleOverTheCeiling(t *testing.T) {
	f := newWuzzufFake(t)
	f.mutateSearch = func(b map[string]any) {
		b["meta"].(map[string]any)["totalResultsCount"] = wuzzufMaxPages*3 + 1
	}
	if _, err := wuzzufFetch(t, f); err == nil || len(f.pages) != 1 {
		t.Fatalf("err=%v pages=%d", err, len(f.pages))
	}
}

func TestWuzzufFailsOnALaterSearchPage(t *testing.T) {
	f := newWuzzufFake(t)
	f.failSearchPage = 1
	if _, err := wuzzufFetch(t, f); err == nil {
		t.Fatal("a failed later page ended the walk as a success")
	}
}

func TestWuzzufFailsOnAPostingListedTwice(t *testing.T) {
	f := newWuzzufFake(t)
	f.mutateSearch = func(b map[string]any) {
		rows := b["data"].([]any)
		rows[4] = rows[0]
	}
	if _, err := wuzzufFetch(t, f); err == nil {
		t.Fatal("a repeated posting was accepted")
	}
}

// The search ignores a filter key it does not know and answers the whole catalogue, so a
// posting outside the board's role means the filter was not applied.
func TestWuzzufFailsOnAPostingOutsideTheRole(t *testing.T) {
	f := newWuzzufFake(t)
	f.mutateJobs = func(docs []any) []any {
		attrs := docs[1].(map[string]any)["attributes"].(map[string]any)
		attrs["workRoles"] = []any{map[string]any{"id": 20, "name": "Manufacturing/Production"}}
		return docs
	}
	if _, err := wuzzufFetch(t, f); err == nil {
		t.Fatal("a posting outside the role was accepted")
	}
}

// A page that lists a posting without carrying its document has not answered about it, which
// is not the platform saying it is gone.
func TestWuzzufMarksAListedPostingWithoutItsDocumentUnreadable(t *testing.T) {
	f := newWuzzufFake(t)
	f.mutateJobs = func(docs []any) []any { return docs[1:] }
	jobs, err := wuzzufFetch(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 6 {
		t.Fatalf("got %d jobs, want 6", len(jobs))
	}
	j := wuzzufByID(jobs)["db64539c-1e1d-4c1d-9a37-60d455bbd6e5"]
	if !j.Unreadable || j.Company != "Our Education" {
		t.Fatalf("the posting without a document: %+v", j)
	}
}

// The page's state is the whole answer; a page without it is not an empty page.
func TestWuzzufFailsOnAPageWithoutItsState(t *testing.T) {
	_, err := wuzzuf{http: wuzzufShell{}, pageSize: 3}.Fetch(context.Background(), CompanyEntry{Board: wuzzufITBoard})
	if err == nil {
		t.Fatal("a page carrying no store state read as a success")
	}
}

type wuzzufShell struct{}

func (wuzzufShell) GetText(context.Context, string) (string, error) {
	return "<html><title>Just a moment...</title></html>", nil
}

func TestWuzzufDropsAPostingNoLongerActive(t *testing.T) {
	f := newWuzzufFake(t)
	f.mutateJobs = func(docs []any) []any {
		docs[0].(map[string]any)["attributes"].(map[string]any)["status"] = "expired"
		return docs
	}
	jobs, err := wuzzufFetch(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 5 {
		t.Fatalf("got %d jobs, want 5", len(jobs))
	}
	if _, ok := wuzzufByID(jobs)["db64539c-1e1d-4c1d-9a37-60d455bbd6e5"]; ok {
		t.Error("an expired posting was yielded")
	}
}

func TestWuzzufIsAFullBoardListingAggregator(t *testing.T) {
	s := NewWuzzuf(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("wuzzuf must carry fullBoardListing")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("wuzzuf must be an aggregator")
	}
	if _, ok := s.(boardless); ok {
		t.Error("wuzzuf is board-based: the board selects the role")
	}
}
