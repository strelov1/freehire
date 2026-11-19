package ojcp

import "testing"

func searchPage() []JobPosting {
	return []JobPosting{projector().JobPosting(openPosting(), nil)}
}

func TestSearchResponseCarriesTheEnvelopeTheStandardRequires(t *testing.T) {
	resp := SearchJobsResponse{
		Query:        "go engineer",
		TotalResults: 431,
		Offset:       40,
		Jobs:         searchPage(),
	}.Finalize()

	if resp.OJCPVersion != Version {
		t.Errorf("ojcp_version = %q, want %q", resp.OJCPVersion, Version)
	}
	if resp.Returned != 1 {
		t.Errorf("returned = %d, want the page length", resp.Returned)
	}
	if err := validateAgainstSchema(t, schemaSearchJobsResponse, resp); err != nil {
		t.Fatalf("search response rejected: %v", err)
	}
}

func TestEveryJobInASearchPageIsAConformingPosting(t *testing.T) {
	// The search response schema INLINES a laxer job shape instead of $ref-ing
	// job-posting.json, so validating the envelope does not validate what is in it. Each
	// element is checked against the posting schema separately or the projection goes
	// untested on the busiest surface of all.
	resp := SearchJobsResponse{Jobs: searchPage()}.Finalize()

	for i, job := range resp.Jobs {
		if err := validateAgainstSchema(t, schemaJobPosting, job); err != nil {
			t.Fatalf("jobs[%d] is not a conforming JobPosting: %v", i, err)
		}
	}
}

func TestSearchResponseSaysWhatItCouldNotHonour(t *testing.T) {
	// The standard's response has nowhere to report a dropped filter, and its extensibility
	// rule lets us add one. Staying silent would hand an agent that asked for jobs within 20
	// miles the whole catalogue, with nothing in the answer admitting the narrowing never
	// happened.
	resp := SearchJobsResponse{
		Jobs:          searchPage(),
		IgnoredParams: []string{"location.radius_miles"},
	}.Finalize()

	if len(resp.IgnoredParams) != 1 || resp.IgnoredParams[0] != "location.radius_miles" {
		t.Errorf("ignored_params = %v, want the dropped filter named", resp.IgnoredParams)
	}
	if err := validateAgainstSchema(t, schemaSearchJobsResponse, resp); err != nil {
		t.Fatalf("response with ignored_params rejected: %v", err)
	}
}

func TestSearchResponseWithNoResultsIsStillWellFormed(t *testing.T) {
	// An empty page is an answer, not an error. `jobs` is required by the schema, so it must
	// serialise as [] rather than null.
	resp := SearchJobsResponse{Query: "nothing matches this"}.Finalize()

	if resp.Jobs == nil {
		t.Error("jobs is nil; the schema requires the array to be present")
	}
	if err := validateAgainstSchema(t, schemaSearchJobsResponse, resp); err != nil {
		t.Fatalf("empty search response rejected: %v", err)
	}
}

func TestJobDetailResponseCarriesThePosting(t *testing.T) {
	resp := JobDetailResponse{Job: projector().JobPosting(openPosting(), nil)}.Finalize()

	if resp.OJCPVersion != Version {
		t.Errorf("ojcp_version = %q", resp.OJCPVersion)
	}
	if err := validateAgainstSchema(t, schemaJobDetailResponse, resp); err != nil {
		t.Fatalf("job detail response rejected: %v", err)
	}
}
