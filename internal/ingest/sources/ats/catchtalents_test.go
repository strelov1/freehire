package ats

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources/kit"
)

type atsJSONFixtures map[string]string

func (f atsJSONFixtures) GetJSON(_ context.Context, url string, dst any) error {
	body, exists := f[url]
	if !exists {
		return fmt.Errorf("unexpected URL %s", url)
	}
	return json.Unmarshal([]byte(body), dst)
}

func TestCatchTalentsFetchesPagesAndHydratesUnseenPostings(t *testing.T) {
	listURL := catchTalentsAPI + "companies/259/jobs?"
	http := atsJSONFixtures{
		listURL + "offset=0&limit=25":  `{"total":2,"results":[{"id":15224,"companyID":259,"jobTitle":"Engineer","companyTitle":"Example"}]}`,
		listURL + "offset=1&limit=25":  `{"total":2,"results":[{"id":15225,"companyID":259,"jobTitle":"Designer","companyTitle":"Example"}]}`,
		catchTalentsAPI + "jobs/15225": `{"id":15225,"companyID":259,"jobTitle":"Designer","companyTitle":"Example","open":true,"description":"<p>Design things</p>","workingModel":"hybrid","workLocations":[{"city":"Berlin","countryCode":"DEU"}]}`,
	}
	source := NewCatchTalents(http).(kit.HydratingSource)
	jobs, err := source.FetchNew(context.Background(), kit.CompanyEntry{Board: "259"}, func(id string) bool { return id == "15224" })
	if err != nil || len(jobs) != 2 {
		t.Fatalf("FetchNew returned %v, %v", jobs, err)
	}
	for _, job := range jobs {
		switch job.ExternalID {
		case "15224":
			if !job.SeenRefresh || job.Unreadable {
				t.Fatalf("expected a liveness refresh, got %+v", job)
			}
		case "15225":
			if job.URL != catchTalentsBoard+"259/jobs/15225" || job.Description != "<p>Design things</p>" ||
				job.WorkMode != "hybrid" || job.Location != "Berlin" || len(job.Countries) != 1 || job.Countries[0] != "de" {
				t.Fatalf("unexpected hydrated posting: %+v", job)
			}
		default:
			t.Fatalf("unexpected id %s", job.ExternalID)
		}
	}
}

func TestCatchTalentsRejectsIncompleteListing(t *testing.T) {
	url := catchTalentsAPI + "companies/259/jobs?"
	http := atsJSONFixtures{
		url + "offset=0&limit=25": `{"total":2,"results":[{"id":15224,"companyID":259}]}`,
		url + "offset=1&limit=25": `{"total":2,"results":[]}`,
	}
	if jobs, err := NewCatchTalents(http).Fetch(context.Background(), kit.CompanyEntry{Board: "259"}); err == nil || len(jobs) != 0 {
		t.Fatalf("partial listing returned %v, %v", jobs, err)
	}
}

type detailFailureFixture struct {
	atsJSONFixtures
	failures map[string]error
}

func (f detailFailureFixture) GetJSON(ctx context.Context, url string, dst any) error {
	if err := f.failures[url]; err != nil {
		return err
	}
	return f.atsJSONFixtures.GetJSON(ctx, url, dst)
}

func TestCatchTalentsDistinguishesUnreadableAndGone(t *testing.T) {
	fixture := detailFailureFixture{
		atsJSONFixtures: atsJSONFixtures{
			catchTalentsAPI + "companies/259/jobs?offset=0&limit=25": `{"total":3,"results":[{"id":1,"companyID":259},{"id":2,"companyID":259},{"id":3,"companyID":259}]}`,
			catchTalentsAPI + "jobs/1":                               `{"id":1,"companyID":259,"jobTitle":"Engineer","companyTitle":"Example","open":true,"description":"Build things"}`,
		},
		failures: map[string]error{
			catchTalentsAPI + "jobs/2": context.DeadlineExceeded,
			catchTalentsAPI + "jobs/3": &kit.StatusError{Code: 404},
		},
	}
	source := NewCatchTalents(fixture)
	entry := kit.CompanyEntry{Board: "259"}
	jobs, err := source.Fetch(context.Background(), entry)
	if err != nil || len(jobs) != 2 || !slices.ContainsFunc(jobs, func(j kit.Job) bool { return j.ExternalID == "2" && j.Unreadable }) {
		t.Fatalf("expected one readable and one unreadable job, got %v, %v", jobs, err)
	}
	fixture.failures[catchTalentsAPI+"jobs/1"] = context.DeadlineExceeded
	if jobs, err := source.Fetch(context.Background(), entry); err == nil || len(jobs) != 0 {
		t.Fatalf("accepted all unreadable details: %v, %v", jobs, err)
	}
}
