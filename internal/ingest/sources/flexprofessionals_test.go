package sources

import (
	"context"
	"strings"
	"testing"
)

const flexprofessionalsFixture = `[
{"id":67122,"date_gmt":"2026-11-18T13:53:54","link":"https://www.flexprofessionalsllc.com/jobs/sr-hr-generalist-2/","title":{"rendered":"Accounting &#038; Operations Manager"},
 "content":{"rendered":" <ul class=\"matador-job-meta matador-job-meta-default\"> <li class=\"matador-job-meta-field\"> <span class=\"matador-job-meta-label\">Location:</span> <span class=\"matador-job-meta-value\">Washington , District Of Columbia</span> </li> <li class=\"matador-job-meta-field\"> <span class=\"matador-job-meta-label\">Work Arrangement:</span> <span class=\"matador-job-meta-value\"> <span class=\" matador-job-terms\"> Hybrid </span> </span> </li> <li class=\"matador-job-meta-field\"> <span class=\"matador-job-meta-label\">Type:</span> <span class=\"matador-job-meta-value\"> <span class=\" matador-job-terms\"> Part-time </span> </span> </li> </ul> <p>Run the books.</p>"}},
{"id":0,"link":"","title":{"rendered":""},"content":{"rendered":""}}
]`

func TestFlexProfessionalsRegisteredBoardless(t *testing.T) {
	s, ok := All(nil)["flexprofessionals"]
	if !ok {
		t.Fatal("All() should register provider flexprofessionals")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("flexprofessionals should be boardless")
	}
}

func TestFlexProfessionalsFetch(t *testing.T) {
	fake := (&routedHTTP{}).route("matador-job-listings", flexprofessionalsFixture)
	jobs, err := NewFlexProfessionals(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (id-less row dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "67122" || j.Title != "Accounting & Operations Manager" || j.Company != "FlexProfessionals" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.Location != "Washington, District Of Columbia" || j.WorkMode != "hybrid" || j.EmploymentType != "part_time" {
		t.Errorf("facets = %q %q %q", j.Location, j.WorkMode, j.EmploymentType)
	}
	if !strings.Contains(j.Description, "Run the books.") || strings.Contains(j.Description, "matador") {
		t.Errorf("Description = %q", j.Description)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt not set")
	}
}
