package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// careersgovFile mirrors the live file's shapes: an hrp row, a greenhouse row typing the
// same keys as strings (which must not break the decode), and an hrp row with no postingNo.
const careersgovFile = `[
{"platform":"hrp","postingNo":"005056a3-d347","jobId":"18218077","jobTitle":"Lecturer - Wellness","agency":"Institute of Technical Education",
 "startDate":1794528000000,"closingDate":1797206400000,"employmentType":"Fixed Terms","workArrangement":"Full-time","experienceYearsMin":3,
 "jobDescription":"</strong></p>Teach students.<p></p><p><strong>","jobResponsibilities":"</strong></p><p>Run projects.</p><p><strong>","jobRequirements":"</strong></p><p>3 years in sport.</p>"},
{"platform":"greenhouse","postingNo":"","jobId":"4007414201","jobTitle":"AP Manager","agency":"Government Technology Agency",
 "startDate":"1790844080000","closingDate":"None","experienceYearsMin":"0","isNew":"False"},
{"platform":"hrp","postingNo":"","jobId":"1","jobTitle":"No posting no","agency":"MOM","startDate":0,"closingDate":0,"experienceYearsMin":0}
]`

func TestCareersGovRegisteredBoardlessAggregator(t *testing.T) {
	s, ok := All(nil)["careersgov"]
	if !ok {
		t.Fatal("All() should register provider careersgov")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("careersgov should be boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("careersgov should be an aggregator")
	}
	if !slices.Contains(FilterableProviders(), "careersgov") {
		t.Error("FilterableProviders() should include careersgov")
	}
}

func TestCareersGovFetchKeepsOnlyHRPRows(t *testing.T) {
	fake := (&routedHTTP{}).route("careersgovsg-jobs-data", careersgovFile)
	jobs, err := NewCareersGov(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (greenhouse row and id-less hrp row dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "18218077" || j.Company != "Institute of Technical Education" || j.Location != "Singapore" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.URL != "https://jobs.careers.gov.sg/jobs/hrp/18218077/005056a3-d347" {
		t.Errorf("URL = %q", j.URL)
	}
	for _, want := range []string{"Teach students.", "Responsibilities", "Run projects.", "Requirements", "3 years in sport."} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q: %q", want, j.Description)
		}
	}
	if strings.HasPrefix(j.Description, "</") || strings.Contains(j.Description, "<strong><h3>") {
		t.Errorf("Description keeps the cut-off heading tags: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.UnixMilli(1794528000000)) {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if j.ApplicationDeadline == nil || !j.ApplicationDeadline.Equal(time.UnixMilli(1797206400000)) {
		t.Errorf("ApplicationDeadline = %v, want the future closing date unclamped", j.ApplicationDeadline)
	}
	if j.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract (Fixed Terms beats Full-time)", j.EmploymentType)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 3 {
		t.Errorf("ExperienceYearsMin = %v, want 3", j.ExperienceYearsMin)
	}
}

func TestCareersGovFetchFailsWithoutHRPRows(t *testing.T) {
	fake := (&routedHTTP{}).route("careersgovsg-jobs-data", `[{"platform":"greenhouse","jobId":"1"}]`)
	if _, err := NewCareersGov(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a file with no hrp row should fail the crawl, not read as an empty catalogue")
	}
}

func TestCareersGovEmploymentType(t *testing.T) {
	cases := []struct{ employment, arrangement, want string }{
		{"Internship", "", "internship"},
		{"Traineeship", "Full-time", "internship"},
		{"Contract", "Part-time", "contract"},
		{"Permanent", "Full-time", "full_time"},
		{"Casual", "Part-time", "part_time"},
		{"Permanent/Contract", "", ""},
	}
	for _, c := range cases {
		if got := careersgovEmploymentType(c.employment, c.arrangement); got != c.want {
			t.Errorf("careersgovEmploymentType(%q, %q) = %q, want %q", c.employment, c.arrangement, got, c.want)
		}
	}
}
