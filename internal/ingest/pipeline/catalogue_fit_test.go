package pipeline

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// unfiledNonTechTitle names a role from the non-tech list that NO category files: the
// catalogue once refused it at ingest (#4412 lifted that), so search now indexes it under "other".
const unfiledNonTechTitle = "Trust & Safety Specialist / Social Worker"

// A posting no category reaches is stored like any other: search indexes it under "other".
func TestRunStoresUnfiledNonTechPostings(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: unfiledNonTechTitle, Company: "Acme"},
		{ExternalID: "2", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 2 {
		t.Fatalf("saved = %d postings, want both", len(store.saved))
	}
	if got := stats.Total(); got.Ingested != 2 {
		t.Errorf("stats = %+v, want Ingested=2", got)
	}
}
