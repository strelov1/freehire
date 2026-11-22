package ats

import (
	"context"
	"encoding/json"
	"fmt"
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
