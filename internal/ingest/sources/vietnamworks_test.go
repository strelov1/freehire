package sources

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

//go:embed testdata/vietnamworks
var vietnamworksFixtures embed.FS

// The fixtures are trimmed from live responses fetched from the crawl host on 2026-11-22: one
// search page (a salaried IT posting listed twice, as the shuffling ranking does, a non-IT one,
// an anonymous employer and a row with no URL) and posting pages whose RSC flight carries the
// body either inline or as a "$<hex>" text row.

const (
	vwITURL     = "https://www.vietnamworks.com/senior-data-engineer-1793269107067400675-2125488-jv"
	vwOtherURL  = "https://www.vietnamworks.com/sales-engineer-1793359123824611749-2126329-jv"
	vwITID      = "2125488"
	vwOtherID   = "2126329"
	vwAnonymous = "2119084"
)

func vietnamworksFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := vietnamworksFixtures.ReadFile("testdata/vietnamworks/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type vietnamworksFake struct {
	mu     sync.Mutex
	search string
	// searchErr, when set, fails every search request.
	searchErr error
	pages     map[string]string
	errs      map[string]error
	asked     []string
}

func (f *vietnamworksFake) PostJSONWithHeaders(_ context.Context, _ string, headers map[string]string, body, v any) error {
	if headers["Origin"] != "https://www.vietnamworks.com" {
		return errors.New("search without the site's Origin")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	for _, want := range []string{`"filter":[]`, `"ranges":[]`, `"order":[]`, `"hitsPerPage":100`} {
		if !strings.Contains(string(raw), want) {
			return errors.New("request body lacks " + want)
		}
	}
	if !strings.Contains(string(raw), `"alias"`) {
		return errors.New("request must select alias for jobUrl")
	}
	if f.searchErr != nil {
		return f.searchErr
	}
	return json.Unmarshal([]byte(f.search), v)
}

func (f *vietnamworksFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.mu.Lock()
	f.asked = append(f.asked, url)
	f.mu.Unlock()
	if err, ok := f.errs[url]; ok {
		return nil, err
	}
	body, ok := f.pages[url]
	if !ok {
		return nil, &StatusError{Code: 404}
	}
	return html.Parse(strings.NewReader(body))
}

func (f *vietnamworksFake) askedFor(url string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.asked {
		if u == url {
			n++
		}
	}
	return n
}

func newVietnamworksFake(t *testing.T) *vietnamworksFake {
	t.Helper()
	return &vietnamworksFake{
		search: vietnamworksFixture(t, "search.json"),
		pages: map[string]string{
			vwITURL:    vietnamworksFixture(t, "detail_ref.html"),
			vwOtherURL: vietnamworksFixture(t, "detail_inline.html"),
		},
	}
}

func newTestVietnamworks(f *vietnamworksFake) vietnamworks {
	return vietnamworks{http: f, limiter: rate.NewLimiter(rate.Inf, 1)}
}

func vwByID(jobs []Job, id string) (Job, bool) {
	for _, j := range jobs {
		if j.ExternalID == id {
			return j, true
		}
	}
	return Job{}, false
}

func TestVietnamworksFetch(t *testing.T) {
	f := newVietnamworksFake(t)
	jobs, err := newTestVietnamworks(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (anonymous, URL-less and repeated rows dropped): %+v", len(jobs), jobs)
	}
	if _, ok := vwByID(jobs, vwAnonymous); ok {
		t.Error("an anonymous employer's posting must not be ingested")
	}
	if n := f.askedFor(vwITURL); n != 1 {
		t.Errorf("a listed-twice posting was fetched %d times, want once", n)
	}

	it, ok := vwByID(jobs, vwITID)
	if !ok {
		t.Fatal("IT posting missing")
	}
	if it.Title != "Senior Data Engineer" || it.URL != vwITURL || it.Company == "" {
		t.Errorf("identity wrong: %+v", it)
	}
	if !it.IsTechHint {
		t.Error("a posting filed under the site's IT group must carry IsTechHint")
	}
	if it.Location != "Ha Noi, Vietnam" && !strings.HasSuffix(it.Location, ", Vietnam") {
		t.Errorf("location = %q", it.Location)
	}
	if len(it.Countries) != 1 || it.Countries[0] != "vn" {
		t.Errorf("countries = %v", it.Countries)
	}
	if it.EmploymentType != "full_time" {
		t.Errorf("employment type = %q", it.EmploymentType)
	}
	if it.SalaryMin == nil || it.SalaryPeriod != "month" || it.SalaryCurrency == "" {
		t.Errorf("salary = %v %v %q %q", it.SalaryMin, it.SalaryMax, it.SalaryCurrency, it.SalaryPeriod)
	}
	if it.PostedAt == nil || it.ApplicationDeadline == nil {
		t.Errorf("dates: posted %v deadline %v", it.PostedAt, it.ApplicationDeadline)
	}
	// The body is a "$29" text row (with multi-byte characters, so its declared length is bytes)
	// followed by the inline requirements, and must be the posting's own, not the other record's.
	for _, want := range []string{"Dẫn dắt đội phát triển phần mềm", "Plan the workload", "7+ years of experience", "Yêu cầu công việc"} {
		if !strings.Contains(it.Description, want) {
			t.Errorf("description lacks %q: %q", want, it.Description)
		}
	}
	if strings.Contains(it.Description, "other posting") {
		t.Errorf("description carries another posting's record: %q", it.Description)
	}

	other, ok := vwByID(jobs, vwOtherID)
	if !ok {
		t.Fatal("non-IT posting missing")
	}
	if other.IsTechHint {
		t.Error("a posting outside the IT group must not carry IsTechHint")
	}
	if other.SalaryMin != nil || other.SalaryPeriod != "" {
		t.Errorf("a posting hiding its salary carries one: %+v", other)
	}
	if other.EmploymentType != "" {
		t.Errorf("an unnamed typeWorkingId must stay unmapped, got %q", other.EmploymentType)
	}
	if !strings.Contains(other.Description, "Plan the workload") {
		t.Errorf("inline body not read: %q", other.Description)
	}
}

func TestVietnamworksSeenPostingsAreRefreshedWithoutARequest(t *testing.T) {
	f := newVietnamworksFake(t)
	jobs, err := newTestVietnamworks(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == vwITID })
	if err != nil {
		t.Fatal(err)
	}
	it, ok := vwByID(jobs, vwITID)
	if !ok || !it.SeenRefresh || it.Description != "" {
		t.Errorf("seen posting = %+v, want a bare refresh", it)
	}
	if n := f.askedFor(vwITURL); n != 0 {
		t.Errorf("seen posting fetched %d times", n)
	}
	if n := f.askedFor(vwOtherURL); n != 1 {
		t.Errorf("new posting fetched %d times, want 1", n)
	}
}

func TestVietnamworksBudgetDefersTheRest(t *testing.T) {
	f := newVietnamworksFake(t)
	v := newTestVietnamworks(f)
	v.maxNewPerRun = 1
	jobs, err := v.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Errorf("got %d jobs under a budget of 1, want 1", len(jobs))
	}
}

func TestVietnamworksUnreadableDetailKeepsListingIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"transport failure", errors.New("timeout")},
		{"missing body", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newVietnamworksFake(t)
			if tc.err != nil {
				f.errs = map[string]error{vwITURL: tc.err}
			} else {
				f.pages[vwITURL] = vietnamworksFixture(t, "detail_empty.html")
			}
			jobs, err := newTestVietnamworks(f).Fetch(context.Background(), CompanyEntry{})
			if err != nil {
				t.Fatal(err)
			}
			job, ok := vwByID(jobs, vwITID)
			if !ok || !job.Unreadable || job.SeenRefresh || job.Description != "" ||
				job.URL != vwITURL || job.Company != "Home Credit Vietnam - Explore Your Dream Team" {
				t.Errorf("unreadable posting = %+v, want a marker with the listing identity", job)
			}
		})
	}
}

func TestVietnamworksDetailReadsOnlyItsRecord(t *testing.T) {
	for _, tc := range []struct {
		name, flight string
		wantBody     bool
	}{
		{"missing record fields", `27:{"jobId":2125488,"jobTitle":"Target"}`, false},
		{"missing requirements", `27:{"jobTitle":"Target {with braces}","jobId":2125488,"jobDescription":"<p>Target body</p>"}`, true},
		{"missing text row", `27:{"jobId":2125488,"jobDescription":"<p>Target body</p>","jobRequirement":"$29"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newVietnamworksFake(t)
			// A length-prefixed text row can run directly into the target model row.
			flight := "7:Tb,<p>text</p>" + tc.flight + "\n" + `28:{"jobId":1,"jobDescription":"<p>Unrelated body</p>","jobRequirement":"<p>Unrelated requirements</p>"}` + "\n"
			encoded, err := json.Marshal(flight)
			if err != nil {
				t.Fatal(err)
			}
			f.pages[vwITURL] = `<script>self.__next_f.push([1,` + string(encoded) + `])</script>`
			jobs, err := newTestVietnamworks(f).Fetch(context.Background(), CompanyEntry{})
			if err != nil {
				t.Fatal(err)
			}
			job, found := vwByID(jobs, vwITID)
			if !found {
				t.Fatal("the listed posting was omitted")
			}
			if !tc.wantBody {
				if !job.Unreadable || job.Description != "" {
					t.Errorf("got %+v, want an unreadable marker with no body", job)
				}
				return
			}
			if job.Unreadable || !strings.Contains(job.Description, "Target body") || strings.Contains(job.Description, "Unrelated") {
				t.Errorf("got %+v, want only the target body", job)
			}
		})
	}
}

func TestVietnamworksCrawlThatReadsNothingFails(t *testing.T) {
	f := newVietnamworksFake(t)
	f.errs = map[string]error{vwITURL: errors.New("timeout")}
	if _, err := newTestVietnamworks(f).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id == vwOtherID }); err == nil {
		t.Fatal("a crawl whose every body request failed must fail even with a seen refresh")
	}
}

func TestVietnamworksConfirmedGoneDetailsAreOmitted(t *testing.T) {
	f := newVietnamworksFake(t)
	f.errs = map[string]error{
		vwITURL:    &StatusError{Code: http.StatusNotFound},
		vwOtherURL: &StatusError{Code: http.StatusGone},
	}
	jobs, err := newTestVietnamworks(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 0 {
		t.Errorf("confirmed closures = %+v, %v; want no jobs and no error", jobs, err)
	}
}

func TestVietnamworksFailedSearchFailsTheCrawl(t *testing.T) {
	f := newVietnamworksFake(t)
	f.searchErr = errors.New("503")
	if _, err := newTestVietnamworks(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("an unreadable search page must fail the crawl")
	}
	f = newVietnamworksFake(t)
	f.search = `{"meta":{"code":200,"nbHits":0,"nbPages":0},"data":[]}`
	if _, err := newTestVietnamworks(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("an empty listing must fail, not close the catalogue")
	}
}

func TestVietnamworksIsBoardlessAggregator(t *testing.T) {
	s := NewVietnamworks(newVietnamworksFake(t))
	if _, ok := s.(interface{ Boardless() }); !ok {
		t.Error("vietnamworks must be boardless")
	}
	if _, ok := s.(interface{ Aggregator() }); !ok {
		t.Error("vietnamworks must be an aggregator")
	}
}
