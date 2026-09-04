package sources

import (
	"context"
	"slices"
	"testing"
)

func TestSolidJobsProvider(t *testing.T) {
	if got := NewSolidJobs(nil).Provider(); got != "solidjobs" {
		t.Errorf("Provider() = %q, want solidjobs", got)
	}
}

func TestSolidJobsIsAggregatorNotBoardless(t *testing.T) {
	s := NewSolidJobs(nil)
	if _, ok := s.(boardless); ok {
		t.Error("solidjobs should NOT implement the boardless marker (board is a required division slug)")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("solidjobs should implement the aggregator marker")
	}
}

func TestSolidJobsRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["solidjobs"]; !ok {
		t.Error("All() should register provider solidjobs")
	}
	if !slices.Contains(FilterableProviders(), "solidjobs") {
		t.Error("FilterableProviders() should include solidjobs")
	}
}

func TestSolidJobsFetchRejectsBlankBoard(t *testing.T) {
	_, err := NewSolidJobs(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Company: "SolidJobs"})
	if err == nil {
		t.Fatal("Fetch: want error for a blank division board, got nil")
	}
}

func TestSolidJobsFetchMapsFields(t *testing.T) {
	body := `{"pageIndex":0,"pageSize":500,"totalCount":2,"totalPages":1,"jobs":[
{"jobOfferKey":"3aa5b098-aa6d-4f6c-950a-b92a81363cf8","title":"DevOps Engineer","company":"Sollers Consulting","locations":["Bydgoszcz"],"description":"<p>Build.</p>","url":"https://solid.jobs/o/s6evgozq/freehire","isRemote":false,"isHybrid":true,"contractTime":"full_time","experienceLevel":"Junior","validFrom":"2026-08-10T17:45:00.8735683+02:00","skills":[{"level":"Basic","name":"Docker"},{"level":"Basic","name":"Kubernetes"}]},
{"jobOfferKey":"","title":"NoID","company":"Ghost"}
]}`
	fake := (&routedHTTP{}).route("offers/it", body)
	jobs, err := NewSolidJobs(fake).Fetch(context.Background(), CompanyEntry{Company: "SolidJobs — IT", Board: "it"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (blank jobOfferKey dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "3aa5b098-aa6d-4f6c-950a-b92a81363cf8" || j.Company != "Sollers Consulting" || j.Title != "DevOps Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.Location != "Bydgoszcz" {
		t.Errorf("Location = %q, want Bydgoszcz", j.Location)
	}
	if j.Remote || j.WorkMode != "hybrid" {
		t.Errorf("Remote=%v WorkMode=%q, want Remote=false WorkMode=hybrid", j.Remote, j.WorkMode)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.Seniority != "junior" {
		t.Errorf("Seniority = %q, want junior", j.Seniority)
	}
	if !slices.Contains(j.Skills, "docker") || !slices.Contains(j.Skills, "kubernetes") {
		t.Errorf("Skills = %v, want canonicalized docker and kubernetes", j.Skills)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed RFC3339 timestamp")
	}
}

func TestSolidJobsSeniority(t *testing.T) {
	cases := map[string]string{
		"Intern":  "intern",
		"Junior":  "junior",
		"Regular": "middle",
		"Senior":  "senior",
		"Expert":  "",
		"":        "",
	}
	for in, want := range cases {
		if got := solidjobsSeniority(in); got != want {
			t.Errorf("solidjobsSeniority(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSolidJobsEmploymentType(t *testing.T) {
	cases := map[string]string{
		"full_time": "full_time",
		"part_time": "part_time",
		"contract":  "",
		"":          "",
	}
	for in, want := range cases {
		if got := solidjobsEmploymentType(in); got != want {
			t.Errorf("solidjobsEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}
