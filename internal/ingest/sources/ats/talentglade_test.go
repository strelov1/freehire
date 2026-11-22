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

func TestTalentgladeRejectsTruncationAndMismatchedDetails(t *testing.T) {
	const api = "https://acme.talentglade.com/__careers?company_slug=acme"
	const a = "51365c15aac4436792c0c1f2eada8b04"
	const b = "4b8983d02cab48fdbe1e21cfd8883024"
	const c = "46cba1199ae94ad99997f09b62edf004"
	const company = `{"slug":"acme","name":"Acme"}`
	const posts = `"company":` + company + `,"jobs":[{"public_id":"` + a + `"},{"public_id":"` + b + `"}]`
	fixture := atsJSONFixtures{
		api + "&type=jobs":            `{"total":3,` + posts + `}`,
		api + "&type=job&job_id=" + a: `{"public_id":"` + a + `","company":` + company + `,"title":"Engineer","description_html":"<p>Build it</p>"}`,
		api + "&type=job&job_id=" + b: `{"public_id":"` + c + `","company":` + company + `,"title":"Designer","description_html":"<p>Design it</p>"}`,
	}
	source := NewTalentglade(fixture)
	entry := kit.CompanyEntry{Board: "acme"}
	if jobs, err := source.Fetch(context.Background(), entry); err == nil || len(jobs) != 0 {
		t.Fatalf("accepted truncated listing: %v, %v", jobs, err)
	}
	fixture[api+"&type=jobs"] = `{"total":2,` + posts + `}`
	jobs, err := source.Fetch(context.Background(), entry)
	if err != nil || len(jobs) != 2 || !slices.ContainsFunc(jobs, func(j kit.Job) bool { return j.ExternalID == b && j.Unreadable }) {
		t.Fatalf("expected mismatched detail to stay unreadable, got %v, %v", jobs, err)
	}
	delete(fixture, api+"&type=job&job_id="+a)
	if jobs, err := source.Fetch(context.Background(), entry); err == nil || len(jobs) != 0 {
		t.Fatalf("accepted all unreadable details: %v, %v", jobs, err)
	}
}
