package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

const brassringHome = `<html><body>
<input id="CookieValue" type="hidden" value="SESSION-1" />
<input name="__RequestVerificationToken" type="hidden" value="TOKEN-1" />
</body></html>`

// fakeBrassring serves a Talent Gateway site: the home page, the paged listing and the details.
type fakeBrassring struct {
	mu        sync.Mutex
	home      string
	total     int                 // the JobsCount the listing states
	pages     map[int][]string    // page number -> reqids on it ("" = a row with no reqid)
	details   map[string]string   // reqid -> JobDetails response body
	detailErr map[string]error    // reqid -> transport error
	titles    map[string]string   // reqid -> listed jobtitle
	headers   []map[string]string // headers of every POST
	detailReq []string
}

func (f *fakeBrassring) GetHTML(_ context.Context, url string) (*html.Node, error) {
	if !strings.Contains(url, "/TGnewUI/Search/Home/Home?partnerid=") {
		return nil, fmt.Errorf("unexpected GET %s", url)
	}
	return html.Parse(strings.NewReader(f.home))
}

func (f *fakeBrassring) PostJSONWithHeaders(_ context.Context, url string, headers map[string]string, body, v any) error {
	raw, _ := json.Marshal(body)
	var req map[string]any
	_ = json.Unmarshal(raw, &req)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.headers = append(f.headers, headers)
	if req["encryptedSessionValue"] != "SESSION-1" {
		return &StatusError{Method: "POST", Code: http.StatusInternalServerError, URL: url}
	}
	switch {
	case strings.HasSuffix(url, "/ProcessSortAndShowMoreJobs"):
		page := int(req["pageNumber"].(float64))
		resp := map[string]any{"JobsCount": f.total, "Jobs": nil}
		if ids := f.pages[page]; len(ids) > 0 {
			var jobs []any
			for _, id := range ids {
				qs := []map[string]string{{"QuestionName": "jobtitle", "Value": f.titles[id]}}
				if id != "" {
					qs = append(qs, map[string]string{"QuestionName": "reqid", "Value": id})
				}
				jobs = append(jobs, map[string]any{"Questions": qs})
			}
			resp["Jobs"] = map[string]any{"Job": jobs}
		}
		out, _ := json.Marshal(resp)
		return json.Unmarshal(out, v)
	case strings.HasSuffix(url, "/JobDetails"):
		id := req["jobid"].(string)
		f.detailReq = append(f.detailReq, id)
		if err := f.detailErr[id]; err != nil {
			return err
		}
		return json.Unmarshal([]byte(f.details[id]), v)
	}
	return fmt.Errorf("unexpected POST %s", url)
}

func brassringDetailBody(title string, qs ...[3]string) string {
	var parts []string
	for _, q := range qs {
		b, _ := json.Marshal(map[string]string{"QuestionName": q[0], "AnswerValue": q[1], "QuestionType": q[2]})
		parts = append(parts, string(b))
	}
	return fmt.Sprintf(`{"ServiceResponse":{"Jobdetails":{"Title":%q,"JobDetailQuestions":[%s]}}}`, title, strings.Join(parts, ","))
}

func newFakeBrassring() *fakeBrassring {
	return &fakeBrassring{
		home:      brassringHome,
		pages:     map[int][]string{},
		details:   map[string]string{},
		detailErr: map[string]error{},
		titles:    map[string]string{},
	}
}

func TestBrassringProviderAndRegistry(t *testing.T) {
	if got := NewBrassring(nil).Provider(); got != "brassring" {
		t.Errorf("Provider() = %q", got)
	}
	if s, ok := All(nil)["brassring"]; !ok || s.Provider() != "brassring" {
		t.Fatal("All() missing provider brassring")
	}
	if !FullBoardListingProviders(All(nil))["brassring"] {
		t.Error("brassring should be a fullBoardListing provider")
	}
}

func TestParseBrassringBoard(t *testing.T) {
	ok := map[string]string{
		"sjobs.brassring.com?partnerid=25416&siteid=6007": "sjobs.brassring.com?partnerid=25416&siteid=6007",
		"XJOBS.brassring.com?SiteId=5496&PartnerId=26144": "xjobs.brassring.com?partnerid=26144&siteid=5496",
		"krb-sjobs.brassring.com?partnerid=1&siteid=2":    "krb-sjobs.brassring.com?partnerid=1&siteid=2",
	}
	for in, want := range ok {
		b, err := parseBrassringBoard(in)
		if err != nil || b.String() != want {
			t.Errorf("parse %q = %q, %v; want %q", in, b, err, want)
		}
		if got := brassringBoardIdentity(in); got != want {
			t.Errorf("identity %q = %q; want %q", in, got, want)
		}
	}
	for _, in := range []string{
		"", "sjobs.brassring.com", "sjobs.brassring.com?partnerid=1", "sjobs.brassring.com?siteid=1",
		"sjobs.brassring.com?partnerid=x&siteid=1", "sjobs.brassring.com?partnerid=1&siteid=",
		"sjobs.brassring.com?partnerid=1&siteid=2&x=3", "sjobs.brassring.com?partnerid=1&partnerid=2&siteid=3",
		"evil.example.com?partnerid=1&siteid=2", "brassring.com.evil.io?partnerid=1&siteid=2",
		"sjobs.brassring.com/x?partnerid=1&siteid=2", "user@sjobs.brassring.com?partnerid=1&siteid=2",
	} {
		if _, err := parseBrassringBoard(in); err == nil {
			t.Errorf("board %q: want error", in)
		}
		if _, err := NewBrassring(newFakeBrassring()).Fetch(context.Background(), CompanyEntry{Board: in}); err == nil {
			t.Errorf("Fetch with board %q: want error", in)
		}
	}
	if got := brassringBoardIdentity("garbage"); got != "garbage" {
		t.Errorf("identity of an unparseable board = %q; want it unchanged", got)
	}
}

const brassringBoardID = "xjobs.brassring.com?partnerid=26144&siteid=5496"

func TestBrassringFetch(t *testing.T) {
	f := newFakeBrassring()
	f.total = 4
	// Page 1 holds a posting with no reqid and a duplicate; page 2 completes the total.
	f.pages[1] = []string{"101", "", "101"}
	f.pages[2] = []string{"102"}
	f.titles["101"] = "Dir, Process Eng. &amp; Maint."
	f.titles["102"] = ""
	f.details["101"] = brassringDetailBody("ignored",
		[3]string{"", "Dir, Process Eng.", "text"},
		[3]string{"Job Purpose", "Lead <b>engineering</b>.<br>Second line.", "textarea"},
		[3]string{"<!--Pay Range-->", "Great pay", "textarea"},
		[3]string{"Empty", "  ", "textarea"},
		[3]string{"City", "Albertville", "text"},
		[3]string{"State:", "Alabama", "single-select"},
		[3]string{"Pay", "100", "text"},
	)
	f.details["102"] = brassringDetailBody("Remote Analyst",
		[3]string{"Work Location Options", "Remote - US", "text"},
		[3]string{"", "unlabelled text", "textarea"},
	)

	jobs, err := NewBrassring(f).Fetch(context.Background(), CompanyEntry{Company: "Caterpillar", Board: brassringBoardID})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2: %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "101" || j.Title != "Dir, Process Eng. & Maint." || j.Company != "Caterpillar" {
		t.Errorf("job 0 identity = %+v", j)
	}
	if want := "https://xjobs.brassring.com/TGnewUI/Search/home/HomeWithPreLoad?partnerid=26144&siteid=5496&PageType=JobDetails&jobid=101"; j.URL != want {
		t.Errorf("URL = %q; want %q", j.URL, want)
	}
	if j.Location != "Albertville, Alabama" {
		t.Errorf("Location = %q", j.Location)
	}
	for _, want := range []string{"Job Purpose", "Lead", "engineering", "Second line", "Pay Range", "Great pay"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description lacks %q: %q", want, j.Description)
		}
	}
	if strings.Contains(j.Description, "<!--") || strings.Contains(j.Description, "Empty") {
		t.Errorf("Description carries a note label or an empty field: %q", j.Description)
	}
	if j.PostedAt != nil {
		t.Errorf("PostedAt = %v; the listing states no posting date", j.PostedAt)
	}

	r := jobs[1]
	if r.ExternalID != "102" || r.Title != "Remote Analyst" {
		t.Errorf("job 1 = %+v (empty listed title falls back to the detail's)", r)
	}
	if r.Location != "Remote - US" || !r.Remote {
		t.Errorf("Location/Remote = %q/%v", r.Location, r.Remote)
	}

	// The anti-forgery token rides every POST; the duplicate was detailed once.
	for _, h := range f.headers {
		if h["RFT"] != "TOKEN-1" {
			t.Errorf("POST headers = %v; want RFT=TOKEN-1", h)
		}
	}
	if len(f.detailReq) != 2 {
		t.Errorf("detail requests = %v; want one per distinct posting", f.detailReq)
	}
}

func TestBrassringSeenPostingsAreNotHydrated(t *testing.T) {
	f := newFakeBrassring()
	f.total = 2
	f.pages[1] = []string{"1", "2"}
	f.titles["1"], f.titles["2"] = "Old", "New"
	f.details["2"] = brassringDetailBody("New", [3]string{"Body", "text", "textarea"})

	jobs, err := NewBrassring(f).(brassring).FetchNew(context.Background(), CompanyEntry{Company: "C", Board: brassringBoardID},
		func(id string) bool { return id == "1" })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || !jobs[0].SeenRefresh || jobs[0].ExternalID != "1" || jobs[1].SeenRefresh || jobs[1].Description == "" {
		t.Fatalf("jobs = %+v", jobs)
	}
	if len(f.detailReq) != 1 || f.detailReq[0] != "2" {
		t.Errorf("detail requests = %v; want only the unseen posting", f.detailReq)
	}
}

func TestBrassringLocationFallsBackToALabelledLocation(t *testing.T) {
	p := &brassringPosting{}
	add := func(name, val, typ string) {
		p.Questions = append(p.Questions, struct {
			Name  string `json:"QuestionName"`
			Value string `json:"AnswerValue"`
			Type  string `json:"QuestionType"`
		}{name, val, typ})
	}
	add("", "Chicago", "single-select") // unlabelled: never guessed
	add("Work Location", "Decatur, IL", "text")
	if got := p.location(); got != "Decatur, IL" {
		t.Errorf("location = %q", got)
	}
	add("Country", "United States", "text")
	if got := p.location(); got != "United States" {
		t.Errorf("a labelled country outranks the generic location: %q", got)
	}
	if got := (&brassringPosting{}).location(); got != "" {
		t.Errorf("no labelled field must give no location, got %q", got)
	}
}

func TestBrassringIncompleteListingFails(t *testing.T) {
	// A stated total the walk cannot reach must fail, never pass for a small board.
	f := newFakeBrassring()
	f.total = 5
	f.pages[1] = []string{"1", "2"} // page 2 is empty
	if _, err := NewBrassring(f).Fetch(context.Background(), CompanyEntry{Board: brassringBoardID}); err == nil {
		t.Fatal("want an error for a listing that ends before its stated total")
	}
}

func TestBrassringEmptyBoard(t *testing.T) {
	f := newFakeBrassring() // total 0, Jobs null
	jobs, err := NewBrassring(f).Fetch(context.Background(), CompanyEntry{Board: brassringBoardID})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs, err = %v, %v; want an empty board", jobs, err)
	}
}

func TestBrassringHomePageWithoutSession(t *testing.T) {
	f := newFakeBrassring()
	f.home = `<html><body>Maintenance</body></html>`
	if _, err := NewBrassring(f).Fetch(context.Background(), CompanyEntry{Board: brassringBoardID}); err == nil {
		t.Fatal("want an error for a page that is not a Talent Gateway home")
	}
}

func TestBrassringDetailFailures(t *testing.T) {
	f := newFakeBrassring()
	f.total = 3
	f.pages[1] = []string{"1", "2", "3"}
	f.titles["1"], f.titles["2"], f.titles["3"] = "Good", "Broken", "Gone"
	f.details["1"] = brassringDetailBody("Good", [3]string{"Body", "text", "textarea"})
	f.detailErr["2"] = &StatusError{Method: "POST", Code: http.StatusBadGateway}
	f.detailErr["3"] = &StatusError{Method: "POST", Code: http.StatusNotFound}

	jobs, err := NewBrassring(f).Fetch(context.Background(), CompanyEntry{Company: "C", Board: brassringBoardID})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 2 || byID["1"].Unreadable || !byID["2"].Unreadable {
		t.Fatalf("jobs = %+v; want 1 read, 2 unreadable, 3 (gone) dropped", jobs)
	}
	if _, listed := byID["3"]; listed {
		t.Error("a posting whose detail is 404 is gone and must not be emitted")
	}

	// A detail the platform answers with no posting states nothing about it.
	f2 := newFakeBrassring()
	f2.total = 1
	f2.pages[1] = []string{"9"}
	f2.details["9"] = `{"ServiceResponse":{"Jobdetails":null}}`
	if _, err := NewBrassring(f2).Fetch(context.Background(), CompanyEntry{Board: brassringBoardID}); err == nil {
		t.Error("a crawl that reads none of its new postings must fail")
	}
}

func TestBrassringListingErrorFailsTheCrawl(t *testing.T) {
	f := newFakeBrassring()
	f.home = brassringHome
	f.total = 1
	bad := &failingBrassring{fakeBrassring: f}
	if _, err := NewBrassring(bad).Fetch(context.Background(), CompanyEntry{Board: brassringBoardID}); err == nil {
		t.Fatal("want the listing failure returned")
	}
}

type failingBrassring struct{ *fakeBrassring }

func (failingBrassring) PostJSONWithHeaders(context.Context, string, map[string]string, any, any) error {
	return errors.New("boom")
}
