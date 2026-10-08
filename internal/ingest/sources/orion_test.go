package sources

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

func orionPageJSON(total, to, page int, items ...string) string {
	itemsJSON := "["
	for i, it := range items {
		if i > 0 {
			itemsJSON += ","
		}
		itemsJSON += it
	}
	itemsJSON += "]"
	return `{"items":` + itemsJSON + `,"pagination":{"total":` + strconv.Itoa(total) +
		`,"to":` + strconv.Itoa(to) + `,"page":` + strconv.Itoa(page) + `}}`
}

func orionItemJSON(id int, slug, title, description, location, postdate string) string {
	return `{"id":` + strconv.Itoa(id) + `,"url":"/job/` + slug + `/",` +
		`"slug":{"value":"` + slug + `"},` +
		`"title":{"value":"` + title + `"},` +
		`"postdate":{"value":"` + postdate + `"},` +
		`"description":{"value":"` + description + `"},` +
		`"locationtext":{"value":"` + location + `"},` +
		`"employment_type":[{"value":"Contract"}]}`
}

func TestOrionFetchMapsListingDirectly(t *testing.T) {
	item := orionItemJSON(4398, "biomarker-sample-ops-manager",
		"Biomarker Sample Ops Manager", "<p>Job details.</p>",
		"United States, Illinois, North Chicago", "08/10/2026")
	page := orionPageJSON(1, 1, 1, item)

	fake := (&routedHTTP{}).route("page=1", page)

	jobs, err := NewOrion(fake).Fetch(context.Background(), CompanyEntry{Provider: "orion"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "4398" {
		t.Errorf("ExternalID = %q, want 4398", j.ExternalID)
	}
	if j.Title != "Biomarker Sample Ops Manager" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://www.orionjobs.com/job/biomarker-sample-ops-manager/" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Orion Group" {
		t.Errorf("Company = %q, want Orion Group", j.Company)
	}
	if j.Location != "United States, Illinois, North Chicago" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.Description != "<p>Job details.</p>" {
		t.Errorf("Description = %q", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-08" {
		t.Errorf("PostedAt = %v, want 2026-10-08", j.PostedAt)
	}
}

func TestOrionFetchFollowsPaginationToExhaustion(t *testing.T) {
	item1 := orionItemJSON(1, "job-one", "Job One", "d1", "UK", "01/10/2026")
	item2 := orionItemJSON(2, "job-two", "Job Two", "d2", "UK", "02/10/2026")
	page1 := orionPageJSON(2, 1, 1, item1)
	page2 := orionPageJSON(2, 2, 2, item2)

	fake := (&routedHTTP{}).
		route("page=1", page1).
		route("page=2", page2)

	jobs, err := NewOrion(fake).Fetch(context.Background(), CompanyEntry{Provider: "orion"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (both pages)", len(jobs))
	}
}

func TestOrionFetchStopsOnEmptyPage(t *testing.T) {
	item1 := orionItemJSON(1, "job-one", "Job One", "d1", "UK", "01/10/2026")
	page1 := orionPageJSON(1, 1, 1, item1)
	page2 := orionPageJSON(1, 1, 2) // no items: an inconsistent total must not loop forever

	fake := (&routedHTTP{}).
		route("page=1", page1).
		route("page=2", page2)

	jobs, err := NewOrion(fake).Fetch(context.Background(), CompanyEntry{Provider: "orion"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (page 2 is empty, stop there)", len(jobs))
	}
}

func TestOrionFirstPageErrorFailsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("page=1", errors.New("origin down"))
	_, err := NewOrion(fake).Fetch(context.Background(), CompanyEntry{Provider: "orion"})
	if err == nil {
		t.Fatal("Fetch: want error on a broken first page, got nil")
	}
}

func TestOrionEmploymentTypeMapsToVocab(t *testing.T) {
	cases := map[string]string{
		"Contract":   "contract",
		"Full Time":  "full_time",
		"Part Time":  "part_time",
		"Internship": "internship",
		"Volunteer":  "",
		"":           "",
	}
	for in, want := range cases {
		if got := orionEmploymentType(in); got != want {
			t.Errorf("orionEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOrionProviderAndBoardless(t *testing.T) {
	s := NewOrion(nil)
	if s.Provider() != "orion" {
		t.Errorf("Provider() = %q, want orion", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("orion must implement boardless")
	}
}
