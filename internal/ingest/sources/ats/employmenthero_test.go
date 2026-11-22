package ats

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources/kit"
)

func TestEmploymentHeroReadsAllPagesWithDescriptions(t *testing.T) {
	const api = employmentHeroAPI + "acme/jobs?"
	const first = `{"job_id":"id-1","friendly_id":"acme-engineer","organisation_friendly_id":"acme","organisation_name":"Acme","title":"Engineer","description":"<p>Build software.</p>","workplace_type":"remote_anywhere"}`
	const second = `{"job_id":"id-2","friendly_id":"acme-designer","organisation_friendly_id":"acme","organisation_name":"Acme","title":"Designer","description":"<p>Design software.</p>"}`
	fake := atsJSONFixtures{
		api + "page_index=1&item_per_page=50": `{"data":{"page_index":1,"total_items":2,"items":[` + first + `]}}`,
		api + "page_index=2&item_per_page=50": `{"data":{"page_index":"2","total_items":"2","items":[` + second + `]}}`,
	}
	jobs, err := NewEmploymentHero(fake).Fetch(context.Background(), kit.CompanyEntry{Board: "acme"})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("Fetch returned %v, %v", jobs, err)
	}
	if job := jobs[0]; job.ExternalID != "id-1" || !job.Remote || job.Description != "<p>Build software.</p>" ||
		job.URL != employmentHeroJobURL+"acme-engineer/" {
		t.Fatalf("unexpected posting %+v", job)
	}
	delete(fake, api+"page_index=2&item_per_page=50")
	if jobs, err := NewEmploymentHero(fake).Fetch(context.Background(), kit.CompanyEntry{Board: "acme"}); err == nil || len(jobs) != 0 {
		t.Fatalf("incomplete listing returned %v, %v", jobs, err)
	}
}
