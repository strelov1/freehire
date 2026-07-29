package sources

import (
	"context"
	"strings"
	"testing"
)

func TestBambooHRProvider(t *testing.T) {
	if got := NewBambooHR(nil).Provider(); got != "bamboohr" {
		t.Errorf("Provider() = %q, want %q", got, "bamboohr")
	}
}

func bambooDetail(id, name string) string {
	return `{"result": {"jobOpening": {
		"jobOpeningName": "` + name + `",
		"jobOpeningShareUrl": "https://acme.bamboohr.com/careers/` + id + `",
		"description": "<p>Do the work.</p>",
		"location": {"city": "Yerevan", "state": null, "addressCountry": "Armenia"},
		"datePosted": "2025-01-14"
	}}}`
}

func TestBambooHRFetchListsAndFetchesDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/careers/52/detail", bambooDetail("52", "Senior Designer")).
		route("/careers/53/detail", bambooDetail("53", "Remote Engineer")).
		route("/careers/list", `{"result": [
			{"id": "52", "jobOpeningName": "Senior Designer", "isRemote": false},
			{"id": "53", "jobOpeningName": "Remote Engineer", "isRemote": true}
		]}`)

	jobs, err := NewBambooHR(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "bamboohr", Board: "acme",
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
	j, ok := byID["52"]
	if !ok {
		t.Fatal("job 52 missing")
	}
	if j.Title != "Senior Designer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://acme.bamboohr.com/careers/52" {
		t.Errorf("URL = %q, want the share url from detail", j.URL)
	}
	if j.Location != "Yerevan, Armenia" {
		t.Errorf("Location = %q, want city/country joined (null state skipped)", j.Location)
	}
	if !strings.Contains(j.Description, "Do the work.") {
		t.Errorf("Description = %q", j.Description)
	}
	if j.Remote {
		t.Error("Remote = true, want false from list isRemote")
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2025 {
		t.Errorf("PostedAt = %v, want parsed datePosted (2025)", j.PostedAt)
	}
	if !byID["53"].Remote {
		t.Error("job 53 isRemote=true should set Remote = true")
	}
}

func TestBambooHRFetchSkipsFailedDetail(t *testing.T) {
	// id 53 has no detail route -> its detail fetch errors and the posting is skipped.
	fake := (&routedHTTP{}).
		route("/careers/52/detail", bambooDetail("52", "Designer")).
		route("/careers/list", `{"result": [
			{"id": "52", "jobOpeningName": "Designer", "isRemote": false},
			{"id": "53", "jobOpeningName": "Broken", "isRemote": false}
		]}`)

	jobs, err := NewBambooHR(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "bamboohr", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort the board on one failed detail: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "52" {
		t.Fatalf("want only 52 to survive, got %d jobs", len(jobs))
	}
}

// BambooHR leaves isRemote null on the public careers list and carries the real signal in
// locationType: "0" onsite, "1" remote, "2" hybrid.
func TestBambooHRFetchReadsLocationType(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/careers/60/detail", bambooDetail("60", "Office Manager")).
		route("/careers/61/detail", bambooDetail("61", "Account Manager")).
		route("/careers/62/detail", bambooDetail("62", "Security Engineer")).
		route("/careers/list", `{"result": [
			{"id": "60", "jobOpeningName": "Office Manager", "isRemote": null, "locationType": "0"},
			{"id": "61", "jobOpeningName": "Account Manager", "isRemote": null, "locationType": "1"},
			{"id": "62", "jobOpeningName": "Security Engineer", "isRemote": null, "locationType": "2"}
		]}`)

	jobs, err := NewBambooHR(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "bamboohr", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	for id, want := range map[string]string{"60": "onsite", "61": "remote", "62": "hybrid"} {
		if got := byID[id].WorkMode; got != want {
			t.Errorf("job %s WorkMode = %q, want %q", id, got, want)
		}
	}
	if !byID["61"].Remote {
		t.Error("locationType 1 should set Remote = true")
	}
	if byID["62"].Remote {
		t.Error("locationType 2 (hybrid) should leave Remote = false")
	}
}

// A board that omits locationType falls back to the list's isRemote flag.
func TestBambooHRFetchFallsBackToIsRemote(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/careers/70/detail", bambooDetail("70", "Support Lead")).
		route("/careers/list", `{"result": [
			{"id": "70", "jobOpeningName": "Support Lead", "isRemote": true}
		]}`)

	jobs, err := NewBambooHR(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "bamboohr", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if jobs[0].WorkMode != "remote" || !jobs[0].Remote {
		t.Errorf("WorkMode = %q, Remote = %v; want remote/true from the isRemote fallback",
			jobs[0].WorkMode, jobs[0].Remote)
	}
}
