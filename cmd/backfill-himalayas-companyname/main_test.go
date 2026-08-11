package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/sources"
)

// fakeStore serves one keyset page of himalayas rows and records every company update.
type fakeStore struct {
	jobs    []db.Job
	updates []db.UpdateJobCompanyParams
	served  bool
}

func (f *fakeStore) ListJobsBySourceAfter(_ context.Context, _ db.ListJobsBySourceAfterParams) ([]db.Job, error) {
	if f.served {
		return nil, nil
	}
	f.served = true
	return f.jobs, nil
}

func (f *fakeStore) UpdateJobCompany(_ context.Context, arg db.UpdateJobCompanyParams) (int64, error) {
	f.updates = append(f.updates, arg)
	return 1, nil
}

func TestBackfillRecoversCompanyFromURL(t *testing.T) {
	store := &fakeStore{jobs: []db.Job{
		// sentinel-hit, URL resolves → repaired
		{ID: 1, Source: "himalayas", Company: sources.HimalayasCompanyNameSentinel,
			URL: "https://himalayas.app/companies/freshworks/jobs/channel-sales-manager-east-9054304068"},
		// already clean → left alone
		{ID: 2, Source: "himalayas", Company: "Peroptyx",
			URL: "https://himalayas.app/companies/peroptyx/jobs/data-analyst"},
		// sentinel-hit but url doesn't match the expected shape → unresolved, skipped
		{ID: 3, Source: "himalayas", Company: sources.HimalayasCompanyNameSentinel,
			URL: "https://example.com/not-a-himalayas-url"},
	}}

	total, updated, unresolved, err := backfillAll(context.Background(), store)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if total != 3 || updated != 1 || unresolved != 1 {
		t.Fatalf("total=%d updated=%d unresolved=%d, want 3/1/1", total, updated, unresolved)
	}
	if len(store.updates) != 1 || store.updates[0].ID != 1 {
		t.Fatalf("updates = %+v, want one update of job 1", store.updates)
	}
	u := store.updates[0]
	if u.Company != "freshworks" || u.CompanySlug != "freshworks" {
		t.Errorf("Company=%q CompanySlug=%q, want both %q", u.Company, u.CompanySlug, "freshworks")
	}
	row := store.jobs[0]
	row.Company, row.CompanySlug = "freshworks", "freshworks"
	want := jobhash.OfRow(row, row.Description)
	if !u.ContentHash.Valid || u.ContentHash.String != want {
		t.Errorf("ContentHash = %+v, want %q", u.ContentHash, want)
	}
}

func TestBackfillIsIdempotent(t *testing.T) {
	// A row already carrying a real company (not the sentinel) is left alone entirely — a
	// second run over already-repaired rows updates nothing.
	store := &fakeStore{jobs: []db.Job{
		{ID: 1, Source: "himalayas", Company: "freshworks", URL: "https://himalayas.app/companies/freshworks/jobs/x"},
	}}
	if _, updated, _, err := backfillAll(context.Background(), store); err != nil || updated != 0 {
		t.Fatalf("updated=%d err=%v, want 0 updates, nil err", updated, err)
	}
}
