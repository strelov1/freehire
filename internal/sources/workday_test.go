package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkdayEmploymentType(t *testing.T) {
	cases := map[string]string{
		"Full time": "full_time",
		"full time": "full_time",
		"Part time": "part_time",
		"part time": "part_time",
		"":          "",
		"Seasonal":  "",
	}
	for in, want := range cases {
		if got := workdayEmploymentType(in); got != want {
			t.Errorf("workdayEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorkdayProvider(t *testing.T) {
	if got := NewWorkday(nil).Provider(); got != "workday" {
		t.Errorf("Provider() = %q, want %q", got, "workday")
	}
}

func TestWorkdayFetchListsAndFetchesDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/Careers/jobs", `{"total": 2, "jobPostings": [
			{"title": "Backend Engineer", "externalPath": "/job/Berlin/Backend_JR-1", "locationsText": "Berlin, Germany"},
			{"title": "Data Engineer", "externalPath": "/job/Remote/Data_JR-2", "locationsText": "Remote, US"}
		]}`).
		route("Backend_JR-1", `{"jobPostingInfo": {
			"title": "Backend Engineer",
			"jobDescription": "<p>Build the backend.</p>",
			"location": "Berlin, Germany",
			"startDate": "2024-06-11",
			"externalUrl": "https://acme.wd1.myworkdayjobs.com/en-US/Careers/job/Berlin/Backend_JR-1",
			"remoteType": "On-site",
			"timeType": "Full time"
		}}`).
		route("Data_JR-2", `{"jobPostingInfo": {
			"title": "Data Engineer",
			"jobDescription": "<p>Crunch data.</p>",
			"location": "Remote, US",
			"startDate": "2024-06-12",
			"remoteType": "Remote"
		}}`)

	jobs, err := NewWorkday(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workday", Board: "acme.wd1.myworkdayjobs.com/Careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["/job/Berlin/Backend_JR-1"]
	if !ok {
		t.Fatal("posting Backend_JR-1 missing")
	}
	if j.Title != "Backend Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Location != "Berlin, Germany" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.URL != "https://acme.wd1.myworkdayjobs.com/en-US/Careers/job/Berlin/Backend_JR-1" {
		t.Errorf("URL = %q, want the detail externalUrl", j.URL)
	}
	if !strings.Contains(j.Description, "Build the backend.") {
		t.Errorf("Description = %q", j.Description)
	}
	if j.Remote {
		t.Error("Remote = true, want false for an on-site role")
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2024 {
		t.Errorf("PostedAt = %v, want parsed startDate (2024)", j.PostedAt)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time (from timeType)", j.EmploymentType)
	}

	d := byID["/job/Remote/Data_JR-2"]
	if !d.Remote {
		t.Error("Remote = false, want true from remoteType")
	}
	if d.URL != "https://acme.wd1.myworkdayjobs.com/Careers/job/Remote/Data_JR-2" {
		t.Errorf("URL = %q, want the path constructed from host+site when externalUrl is absent", d.URL)
	}
}

func TestWorkdayFetchSkipsFailedDetail(t *testing.T) {
	// JR-2 has no detail route -> its detail fetch errors and the posting is skipped,
	// but JR-1 still comes through.
	fake := (&routedHTTP{}).
		route("/Careers/jobs", `{"total": 2, "jobPostings": [
			{"title": "Engineer", "externalPath": "/job/X/JR-1", "locationsText": "Berlin"},
			{"title": "Broken", "externalPath": "/job/Y/JR-2", "locationsText": "NYC"}
		]}`).
		route("/job/X/JR-1", `{"jobPostingInfo": {"title": "Engineer", "jobDescription": "<p>ok</p>", "location": "Berlin"}}`)

	jobs, err := NewWorkday(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workday", Board: "acme.wd1.myworkdayjobs.com/Careers",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort the board on one failed detail: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "/job/X/JR-1" {
		t.Fatalf("want only JR-1 to survive, got %d jobs", len(jobs))
	}
}

// pagedWorkday returns a canned list page per call (in order), delegating detail
// GetJSONs to the embedded routedHTTP. It mimics pg.wd5.myworkdayjobs.com, which
// reports the real `total` only on the first page and `total:0` thereafter.
type pagedWorkday struct {
	*routedHTTP
	pages []string
	call  int
}

func (p *pagedWorkday) PostJSON(_ context.Context, _ string, _, v any) error {
	body := `{"total":0,"jobPostings":[]}`
	if p.call < len(p.pages) {
		body = p.pages[p.call]
	}
	p.call++
	return json.Unmarshal([]byte(body), v)
}

func workdayDetailBody(title string) string {
	return `{"jobPostingInfo":{"title":"` + title + `","jobDescription":"<p>x</p>","location":"Berlin, Germany","startDate":"2024-06-11"}}`
}

// A board reporting total only on its first page must still be drained fully:
// stopping at a later page's total:0 drops the postings past it, and a dropped
// posting loses its last_seen_at stamp and is closed by the 48h unseen sweep.
func TestWorkdayPagesByFirstPageTotal(t *testing.T) {
	fake := &pagedWorkday{
		routedHTTP: (&routedHTTP{}).
			route("A_JR-1", workdayDetailBody("A")).
			route("B_JR-2", workdayDetailBody("B")).
			route("C_JR-3", workdayDetailBody("C")).
			route("D_JR-4", workdayDetailBody("D")),
		pages: []string{
			`{"total":4,"jobPostings":[{"title":"A","externalPath":"/job/X/A_JR-1"},{"title":"B","externalPath":"/job/X/B_JR-2"}]}`,
			`{"total":0,"jobPostings":[{"title":"C","externalPath":"/job/X/C_JR-3"}]}`,
			`{"total":0,"jobPostings":[{"title":"D","externalPath":"/job/X/D_JR-4"}]}`,
		},
	}
	jobs, err := NewWorkday(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workday", Board: "acme.wd1.myworkdayjobs.com/Careers",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("len(jobs) = %d, want 4 (all pages drained despite total:0 after page one)", len(jobs))
	}
	got := map[string]bool{}
	for _, j := range jobs {
		got[j.ExternalID] = true
	}
	if !got["/job/X/D_JR-4"] {
		t.Error("posting on page three missing — pagination stopped early on a later page's total:0")
	}
}

func TestParseWorkdayBoardRejectsMalformed(t *testing.T) {
	for _, board := range []string{"", "no-slash-host", "/onlysite", "host-no-dot/site"} {
		if _, err := parseWorkdayBoard(board); err == nil {
			t.Errorf("parseWorkdayBoard(%q) = nil error, want error", board)
		}
	}
}
