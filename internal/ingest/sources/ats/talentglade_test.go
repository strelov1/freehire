package ats

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources/kit"
)

func TestTalentgladeRejectsReplacementForClosedPosting(t *testing.T) {
	const api = "https://acme.talentglade.com/__careers?company_slug=acme"
	const first = "51365c15aac4436792c0c1f2eada8b04"
	const second = "4b8983d02cab48fdbe1e21cfd8883024"
	const company = `{"slug":"acme","name":"Acme"}`
	http := atsJSONFixtures{
		api + "&type=jobs":                 `{"total":2,"company":` + company + `,"jobs":[{"public_id":"` + first + `"},{"public_id":"` + second + `"}]}`,
		api + "&type=job&job_id=" + first:  `{"public_id":"` + first + `","company":` + company + `,"title":"Engineer","description_html":"<p>Build it</p>","workplace_type":"remote"}`,
		api + "&type=job&job_id=" + second: `{"public_id":"` + first + `","company":` + company + `,"closed_title":"Old job","title":"Engineer","description_html":"<p>Build it</p>"}`,
	}
	jobs, err := NewTalentglade(http).Fetch(context.Background(), kit.CompanyEntry{Board: "acme"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch returned %v, %v", jobs, err)
	}
	if job := jobs[0]; job.ExternalID != first || !job.Remote || job.Description != "<p>Build it</p>" {
		t.Fatalf("unexpected posting %+v", job)
	}
}

func TestTalentgladeRefreshSkipsDetail(t *testing.T) {
	const api = "https://acme.talentglade.com/__careers?company_slug=acme"
	const first = "51365c15aac4436792c0c1f2eada8b04"
	http := atsJSONFixtures{
		api + "&type=jobs": `{"total":1,"company":{"slug":"acme","name":"Acme"},"jobs":[{"public_id":"` + first + `"}]}`,
	}
	source := NewTalentglade(http).(kit.HydratingSource)
	jobs, err := source.FetchNew(context.Background(), kit.CompanyEntry{Board: "acme"}, func(id string) bool { return id == first })
	if err != nil || !slices.ContainsFunc(jobs, func(job kit.Job) bool { return job.ExternalID == first && job.SeenRefresh }) {
		t.Fatalf("FetchNew returned %v, %v", jobs, err)
	}
}
