package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// livehireFake serves the POST search listing and the GET detail the adapter calls. The
// adapter fans its detail fetches out across a worker pool, so the recorder is lock-guarded.
type livehireFake struct {
	pages      map[int]string    // page number (1-based) -> search response JSON
	listErr    bool              // the listing fetch fails
	detail     map[string]string // "<urlCode>/<advertUrlCode>" -> detail response JSON
	detailErr  map[string]bool   // "<urlCode>/<advertUrlCode>" -> detail fetch fails
	mu         sync.Mutex
	detailURLs []string // detail URLs fetched, for asserting new-only hydration
}

func (f *livehireFake) PostJSON(_ context.Context, _ string, body, v any) error {
	if f.listErr {
		return errors.New("list boom")
	}
	page := body.(livehireSearchRequest).Page
	return json.Unmarshal([]byte(f.pages[page]), v)
}

func (f *livehireFake) GetJSON(_ context.Context, url string, v any) error {
	// .../job/<board>/<urlCode>/<advertUrlCode>
	parts := strings.Split(url, "/")
	key := parts[len(parts)-2] + "/" + parts[len(parts)-1]
	f.mu.Lock()
	f.detailURLs = append(f.detailURLs, url)
	f.mu.Unlock()
	if f.detailErr[key] {
		return errors.New("detail boom")
	}
	return json.Unmarshal([]byte(f.detail[key]), v)
}

// livehireDetailJSON builds a detail body carrying the given description HTML.
func livehireDetailJSON(desc string) string {
	return fmt.Sprintf(`{"roleName":"Engineer","descriptionHtml":%q,
		"physicalLocation":"Melbourne VIC, Australia","locationCountryIsoCode":"AU",
		"workType":"FullTimePermanent","flexibleWorkOption":"Hybrid",
		"currencyIsoCode":"AUD","applicationsClosed":false}`, desc)
}

// livehireListingJSON builds one listing page naming the given urlCode/advertUrlCode pairs.
func livehireListingJSON(hasMore bool, pairs ...[2]string) string {
	var jobs []string
	for _, p := range pairs {
		jobs = append(jobs, fmt.Sprintf(
			`{"segmentCode":"flybuys","urlCode":%q,"advertUrlCode":%q,
			  "roleName":"Engineer %s","summary":"s","workType":"FullTimePermanent",
			  "flexibleWorkOption":"Hybrid","physicalLocation":"Melbourne VIC, Australia",
			  "countryIso":"AU","countryName":"Australia","salaryRange":"",
			  "postedFrom":"2026-10-22T00:06:42.117Z","internalOnly":false}`,
			p[0], p[1], p[0]))
	}
	return fmt.Sprintf(`{"jobs":[%s],"totalCount":%d,"hasMoreResults":%t}`,
		strings.Join(jobs, ","), len(jobs), hasMore)
}

var _ = time.Now // keep time imported for the fixtures' RFC3339 shapes above

func TestLiveHireFetchMapsListing(t *testing.T) {
	fake := &livehireFake{pages: map[int]string{
		1: livehireListingJSON(false, [2]string{"EYFKM", "2ECWIFSUWD"}),
	}}
	jobs, err := NewLiveHire(fake).Fetch(context.Background(), CompanyEntry{Board: "flybuys"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "flybuys_EYFKM_2ECWIFSUWD" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	wantURL := "https://www.livehire.com/careers/flybuys/job/EYFKM/2ECWIFSUWD/engineer-eyfkm"
	if j.URL != wantURL {
		t.Errorf("URL = %q, want %q", j.URL, wantURL)
	}
	if j.Title != "Engineer EYFKM" || j.Location != "Melbourne VIC, Australia" {
		t.Errorf("title/location = %q/%q", j.Title, j.Location)
	}
	if j.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid", j.WorkMode)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "au" {
		t.Errorf("Countries = %v, want [au]", j.Countries)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
}

func TestLiveHireFetchDropsUnusable(t *testing.T) {
	fake := &livehireFake{pages: map[int]string{1: `{"jobs":[
		{"segmentCode":"flybuys","roleName":"No codes"},
		{"segmentCode":"flybuys","urlCode":"I1","advertUrlCode":"A1","roleName":"Internal only","internalOnly":true},
		{"segmentCode":"flybuys","urlCode":"O1","advertUrlCode":"A2","roleName":"Fine"}
	],"totalCount":3,"hasMoreResults":false}`}}
	jobs, err := NewLiveHire(fake).Fetch(context.Background(), CompanyEntry{Board: "flybuys"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "flybuys_O1_A2" {
		t.Fatalf("want only flybuys_O1_A2, got %+v", jobs)
	}
}

func TestLiveHireFetchNewHydratesUnseenOnly(t *testing.T) {
	seen := map[string]bool{"flybuys_SEEN_1": true}
	fake := &livehireFake{
		pages: map[int]string{
			1: livehireListingJSON(false, [2]string{"SEEN", "1"}, [2]string{"NEW", "2"}),
		},
		detail: map[string]string{
			"NEW/2": livehireDetailJSON("<p>About us</p>"),
		},
	}
	jobs, err := NewLiveHire(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "flybuys"}, func(id string) bool { return seen[id] })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(fake.detailURLs) != 1 || !strings.Contains(fake.detailURLs[0], "/job/flybuys/NEW/2") {
		t.Fatalf("detail should be fetched for NEW/2 only, got %v", fake.detailURLs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	seenJob, ok := byID["flybuys_SEEN_1"]
	if !ok || !seenJob.SeenRefresh {
		t.Errorf("seen posting must be a SeenRefresh, got %+v", seenJob)
	}
	if seenJob.Description != "" {
		t.Errorf("seen posting must not carry a description")
	}
	newJob := byID["flybuys_NEW_2"]
	if newJob.Description != "<p>About us</p>" {
		t.Errorf("new posting description = %q", newJob.Description)
	}
	if newJob.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", newJob.EmploymentType)
	}
}

func TestLiveHireFetchNewDetailFailureIngestsListOnly(t *testing.T) {
	fake := &livehireFake{
		pages:     map[int]string{1: livehireListingJSON(false, [2]string{"BAD", "9"})},
		detailErr: map[string]bool{"BAD/9": true},
	}
	jobs, err := NewLiveHire(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "flybuys"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Description != "" {
		t.Fatalf("a failed detail must still ingest list-only, got %+v", jobs)
	}
}

func TestLiveHireDetailClosedMarksRemoved(t *testing.T) {
	fake := &livehireFake{
		pages: map[int]string{1: livehireListingJSON(false, [2]string{"GONE", "5"})},
		detail: map[string]string{
			"GONE/5": `{"roleName":"Old role","descriptionHtml":"<p>x</p>","applicationsClosed":true}`,
		},
	}
	jobs, err := NewLiveHire(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "flybuys"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].Removed {
		t.Fatalf("a closed posting must be marked Removed, got %+v", jobs)
	}
}

func TestLiveHirePaginationAndDedup(t *testing.T) {
	// The endpoint re-serves the full set on later pages regardless of the requested page
	// (verified live), so a repeated posting must not double-count and the walk must end.
	fake := &livehireFake{pages: map[int]string{
		1: livehireListingJSON(true, [2]string{"A", "1"}, [2]string{"B", "2"}),
		2: livehireListingJSON(false, [2]string{"A", "1"}, [2]string{"B", "2"}, [2]string{"C", "3"}),
	}}
	jobs, err := NewLiveHire(fake).Fetch(context.Background(), CompanyEntry{Board: "flybuys"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("want 3 unique postings, got %d", len(jobs))
	}
}

func TestLiveHireListErrorFailsBoard(t *testing.T) {
	fake := &livehireFake{listErr: true}
	if _, err := NewLiveHire(fake).Fetch(context.Background(), CompanyEntry{Board: "flybuys"}); err == nil {
		t.Fatal("a failed first listing page must fail the board")
	}
}

func TestLiveHireSalaryMapping(t *testing.T) {
	fake := &livehireFake{
		pages: map[int]string{1: livehireListingJSON(false, [2]string{"PAY", "7"})},
		detail: map[string]string{
			"PAY/7": `{"roleName":"Paid role","descriptionHtml":"<p>x</p>","minSalary":100000,
				"maxSalary":130000,"currencyIsoCode":"AUD","applicationsClosed":false}`,
		},
	}
	jobs, err := NewLiveHire(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "flybuys"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	j := jobs[0]
	if j.SalaryMin == nil || *j.SalaryMin != 100000 || j.SalaryMax == nil || *j.SalaryMax != 130000 {
		t.Errorf("salary = %v-%v, want 100000-130000", j.SalaryMin, j.SalaryMax)
	}
	if j.SalaryCurrency != "AUD" {
		t.Errorf("SalaryCurrency = %q", j.SalaryCurrency)
	}
}

func TestLiveHireEmploymentTypeVocabulary(t *testing.T) {
	for raw, want := range map[string]string{
		"FullTimePermanent": "full_time",
		"PartTimePermanent": "part_time",
		"Casual":            "contract",
		"FixedTerm":         "contract",
		"Internship":        "internship",
		"SomethingNew":      "",
	} {
		if got := livehireEmploymentType(raw); got != want {
			t.Errorf("livehireEmploymentType(%q) = %q, want %q", raw, got, want)
		}
	}
}
