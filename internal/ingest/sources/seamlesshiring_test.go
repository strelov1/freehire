package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSeamlessHiringProvider(t *testing.T) {
	if got := NewSeamlessHiring(nil).Provider(); got != "seamlesshiring" {
		t.Errorf("Provider() = %q, want seamlesshiring", got)
	}
}

func TestSeamlessHiringRegisteredAsFullAndEmptyBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["seamlesshiring"] {
		t.Error("FullBoardListingProviders(All(nil)) should include seamlesshiring")
	}
	if !EmptyBoardListingProviders(All(nil))["seamlesshiring"] {
		t.Error("an emptied tenant answers data: [] and an unknown host does not resolve, so seamlesshiring should claim emptyBoardListing")
	}
}

func TestSeamlessHiringFetchListsPostings(t *testing.T) {
	fake := &seamlesshiringFake{pages: map[int]string{
		1: seamlesshiringEnvelope(2, 1, 1, []string{
			`{
				"id": 11286,
				"title": "Financial Crime Prevention Manager",
				"summary": "About the role",
				"details": "<p>Keep the books.</p>",
				"job_type": "full-time",
				"work_style": "onsite",
				"city": "Windhoek",
				"location": "Khomas Region",
				"created_at": "2026-10-26T07:48:09.000000Z",
				"post_date": "2020-01-01",
				"company": {"name": "Letshego Financial Services Namibia"},
				"minimum_remuneration": "1000000",
				"maximum_remuneration": "1500000",
				"currency_id": 74,
				"show_remuneration": 1,
				"location_details": {"country": {"country_code": "NA", "name": "Namibia"}}
			}`,
			`{
				"id": 11285,
				"title": "Head of Internal Audit",
				"job_type": "part-time",
				"work_style": "hybrid",
				"city": "Windhoek",
				"location": "windhoek",
				"post_date": "2026-10-01",
				"company": {"name": ""},
				"location_details": {"country": {"country_code": "GB", "name": "United Kingdom"}}
			}`,
		}),
	}}
	jobs, err := NewSeamlessHiring(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Catalogue Co", Board: "Letshego",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len = %d, want 2", len(jobs))
	}
	if len(fake.urls) != 1 || !strings.HasPrefix(fake.urls[0], "https://letshego.seamlesshiring.com/v2/jobs/job-list?") {
		t.Fatalf("urls = %v", fake.urls)
	}
	got, _ := url.Parse(fake.urls[0])
	if got.Query().Get("limit") != "100" || got.Query().Get("page") != "1" {
		t.Fatalf("query = %s", got.RawQuery)
	}
	first := jobs[0]
	if first.ExternalID != "11286" || first.Title != "Financial Crime Prevention Manager" {
		t.Fatalf("first identity = %+v", first)
	}
	if first.URL != "https://letshego.seamlesshiring.com/job/view/11286" {
		t.Errorf("url = %s", first.URL)
	}
	if first.Company != "Letshego Financial Services Namibia" || first.Location != "Windhoek, Khomas Region" {
		t.Errorf("company/location = %q / %q", first.Company, first.Location)
	}
	if first.EmploymentType != "full_time" || first.WorkMode != "onsite" {
		t.Errorf("facets = %s %s", first.EmploymentType, first.WorkMode)
	}
	if first.Countries != nil {
		t.Errorf("countries = %v, want none: NA is not in the location dictionary", first.Countries)
	}
	if first.SalaryMin != nil || first.SalaryCurrency != "" {
		t.Errorf("salary stored from an internal currency id: %+v %s", first.SalaryMin, first.SalaryCurrency)
	}
	if !strings.Contains(first.Description, "About the role") || !strings.Contains(first.Description, "Keep the books") {
		t.Errorf("description = %s", first.Description)
	}
	wantPosted := time.Date(2026, 10, 26, 7, 48, 9, 0, time.UTC)
	if first.PostedAt == nil || !first.PostedAt.Equal(wantPosted) {
		t.Errorf("posted = %v", first.PostedAt)
	}
	second := jobs[1]
	if second.Company != "Catalogue Co" || second.Location != "Windhoek" {
		t.Errorf("fallback company/location = %q / %q", second.Company, second.Location)
	}
	if second.EmploymentType != "part_time" || second.WorkMode != "hybrid" {
		t.Errorf("second facets = %s %s", second.EmploymentType, second.WorkMode)
	}
	if len(second.Countries) != 1 || second.Countries[0] != "gb" {
		t.Errorf("countries = %v", second.Countries)
	}
	if second.PostedAt == nil || second.PostedAt.Format("2006-01-02") != "2026-10-01" {
		t.Errorf("posted = %v", second.PostedAt)
	}
}

func TestSeamlessHiringFetchWalksEveryPage(t *testing.T) {
	const total = seamlesshiringPageSize + 1
	rows := make([]string, total)
	for i := range rows {
		rows[i] = fmt.Sprintf(`{"id":%d,"title":"Role %d"}`, i+1, i+1)
	}
	fake := &seamlesshiringFake{pages: map[int]string{
		1: seamlesshiringEnvelope(total, 1, 2, rows[:seamlesshiringPageSize]),
		2: seamlesshiringEnvelope(total, 2, 2, rows[seamlesshiringPageSize:]),
	}}
	jobs, err := NewSeamlessHiring(fake).Fetch(context.Background(), CompanyEntry{Board: "goldenoiltd"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != total {
		t.Fatalf("len = %d, want %d", len(jobs), total)
	}
	if len(fake.urls) != 2 {
		t.Fatalf("urls = %v", fake.urls)
	}
	for _, raw := range fake.urls {
		if strings.HasPrefix(raw, "http://") {
			t.Errorf("followed an http next_page_url: %s", raw)
		}
	}
	if jobs[total-1].ExternalID != strconv.Itoa(total) || jobs[total-1].WorkMode != "" {
		t.Errorf("last = %+v", jobs[total-1])
	}
}

func TestSeamlessHiringFetchRejectsAShortOrUnstableListing(t *testing.T) {
	cases := []struct {
		name  string
		pages map[int]string
	}{
		{"short page", map[int]string{1: seamlesshiringEnvelope(2, 1, 1, []string{`{"id":1,"title":"A"}`})}},
		{"last page disagrees", map[int]string{1: seamlesshiringEnvelope(1, 1, 2, []string{`{"id":1,"title":"A"}`})}},
		{"duplicate id", map[int]string{1: seamlesshiringEnvelope(2, 1, 1, []string{`{"id":1,"title":"A"}`, `{"id":1,"title":"B"}`})}},
		{"missing title", map[int]string{1: seamlesshiringEnvelope(1, 1, 1, []string{`{"id":1,"title":" "}`})}},
		{"over the cap", map[int]string{1: `{"status":"success","status_code":200,"data":{"jobs":{"data":[],"total":50001,"current_page":1,"last_page":501,"per_page":100}}}`}},
		{"status not success", map[int]string{1: `{"status":"error","status_code":200,"data":{"jobs":{"data":[],"total":0,"current_page":1,"last_page":1,"per_page":100}}}`}},
		{"total changes", map[int]string{
			1: seamlesshiringEnvelope(seamlesshiringPageSize+1, 1, 2, repeatJobs(seamlesshiringPageSize, 1)),
			2: seamlesshiringEnvelope(seamlesshiringPageSize, 2, 2, []string{`{"id":99999,"title":"Late"}`}),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewSeamlessHiring(&seamlesshiringFake{pages: tc.pages}).Fetch(context.Background(), CompanyEntry{Board: "letshego"}); err == nil {
				t.Fatal("Fetch succeeded, want an error")
			}
		})
	}
}

func TestSeamlessHiringFetchRejectsABadBoardWithoutARequest(t *testing.T) {
	fake := &seamlesshiringFake{}
	if _, err := NewSeamlessHiring(fake).Fetch(context.Background(), CompanyEntry{Board: "../x"}); err == nil {
		t.Fatal("Fetch succeeded, want an error")
	}
	if len(fake.urls) != 0 {
		t.Fatalf("urls = %v", fake.urls)
	}
}

type seamlesshiringFake struct {
	pages map[int]string
	urls  []string
}

func (f *seamlesshiringFake) GetJSON(_ context.Context, raw string, v any) error {
	f.urls = append(f.urls, raw)
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	page, _ := strconv.Atoi(u.Query().Get("page"))
	body, ok := f.pages[page]
	if !ok {
		return fmt.Errorf("no page %d", page)
	}
	return json.Unmarshal([]byte(body), v)
}

func seamlesshiringEnvelope(total, page, last int, rows []string) string {
	return fmt.Sprintf(
		`{"status":"success","status_code":200,"data":{"jobs":{"data":[%s],"total":%d,"current_page":%d,"last_page":%d,"per_page":%d}}}`,
		strings.Join(rows, ","), total, page, last, seamlesshiringPageSize,
	)
}

func repeatJobs(n, start int) []string {
	rows := make([]string, n)
	for i := range rows {
		rows[i] = fmt.Sprintf(`{"id":%d,"title":"Role %d"}`, start+i, start+i)
	}
	return rows
}
