package main

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobderive"
)

// fakeStore serves one page of jobs (keyset paging: AfterID 0 returns all, then
// empty) and records every UpdateJobFacets call. UpdateJobFacets is guarded so the
// concurrent worker pool can call it in parallel without a data race.
type fakeStore struct {
	jobs    []db.Job
	mu      sync.Mutex
	updates []db.UpdateJobFacetsParams
}

func (f *fakeStore) ListJobsByIDAfter(_ context.Context, arg db.ListJobsByIDAfterParams) ([]db.Job, error) {
	if arg.AfterID != 0 {
		return nil, nil
	}
	return f.jobs, nil
}

func (f *fakeStore) UpdateJobFacets(_ context.Context, arg db.UpdateJobFacetsParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, arg)
	return nil
}

// expectedFacets is the UpdateJobFacets the runner should write for a job: every
// dictionary facet from jobderive.Derive (the six original plus the four synthetic
// enrichment facets), and no slug fields.
func expectedFacets(j db.Job) db.UpdateJobFacetsParams {
	d := jobderive.Derive(jobderive.Input{
		Title: j.Title, Company: j.Company, Source: j.Source, ExternalID: j.ExternalID,
		Location: j.Location, Description: j.Description, WorkMode: j.WorkMode,
	})
	return db.UpdateJobFacetsParams{
		ID: j.ID, Countries: d.Countries, Regions: d.Regions, Cities: d.Cities, WorkMode: d.WorkMode,
		Skills: d.Skills, Seniority: d.Seniority, Category: d.Category,
		PostingLanguage:    d.PostingLanguage,
		EmploymentType:     d.EmploymentType,
		EducationLevel:     d.EducationLevel,
		ExperienceYearsMin: toInt4(d.ExperienceYearsMin),
	}
}

// backfillJobDescription triggers both the original facets (skills) and the synthetic
// ones (English language, full-time, bachelor, 5 years) so the test verifies all of them.
const backfillJobDescription = "We use Go, PostgreSQL and Kubernetes. This is a " +
	"full-time role. A Bachelor's degree and 5+ years of experience are required."

func TestBackfill_RewritesAllFacetsInOnePass(t *testing.T) {
	job := db.Job{
		ID: 7, Title: "Senior Go Developer", Company: "Acme",
		Source: "manual", ExternalID: "x", Location: "Berlin, Germany",
		Description: backfillJobDescription,
		// facet columns empty → the derived values differ → a write happens.
	}
	store := &fakeStore{jobs: []db.Job{job}}

	scanned, updated, err := backfillAll(context.Background(), store, 1)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 1 || updated != 1 {
		t.Fatalf("scanned=%d updated=%d, want 1/1", scanned, updated)
	}
	if len(store.updates) != 1 {
		t.Fatalf("got %d UpdateJobFacets calls, want 1", len(store.updates))
	}
	want := expectedFacets(job)
	if !reflect.DeepEqual(store.updates[0], want) {
		t.Errorf("UpdateJobFacets = %+v, want %+v", store.updates[0], want)
	}
	// Guard that the synthetic facets were actually derived (so this test can't pass
	// with everything at zero values).
	got := store.updates[0]
	if got.PostingLanguage != "en" || got.EmploymentType != "full_time" ||
		got.EducationLevel != "bachelor" || !got.ExperienceYearsMin.Valid {
		t.Errorf("synthetic facets not derived: lang=%q type=%q edu=%q exp=%v",
			got.PostingLanguage, got.EmploymentType, got.EducationLevel, got.ExperienceYearsMin)
	}
}

func TestBackfill_IsIdempotent(t *testing.T) {
	job := db.Job{
		ID: 7, Title: "Senior Go Developer", Company: "Acme",
		Source: "manual", ExternalID: "x", Location: "Berlin, Germany",
		Description: backfillJobDescription,
	}
	// Seed the columns with what the derivation already produces — a second pass
	// must rewrite nothing.
	d := expectedFacets(job)
	job.Countries, job.Regions, job.WorkMode = d.Countries, d.Regions, d.WorkMode
	job.Cities = d.Cities
	job.Skills, job.Seniority, job.Category = d.Skills, d.Seniority, d.Category
	job.PostingLanguage, job.EmploymentType = d.PostingLanguage, d.EmploymentType
	job.EducationLevel, job.ExperienceYearsMin = d.EducationLevel, d.ExperienceYearsMin

	store := &fakeStore{jobs: []db.Job{job}}
	scanned, updated, err := backfillAll(context.Background(), store, 1)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 1 || updated != 0 {
		t.Fatalf("scanned=%d updated=%d, want 1/0 (unchanged row skipped)", scanned, updated)
	}
	if len(store.updates) != 0 {
		t.Errorf("expected no writes for an unchanged row, got %d", len(store.updates))
	}
}

func TestBackfill_PreservesSetWorkMode(t *testing.T) {
	// A location with no work-mode hint plus an already-set work_mode: the derived
	// value must keep the set work_mode, not blank it.
	job := db.Job{
		ID: 7, Title: "Developer", Company: "Acme", Source: "manual", ExternalID: "x",
		Location: "Berlin, Germany", WorkMode: "hybrid",
	}
	store := &fakeStore{jobs: []db.Job{job}}
	if _, _, err := backfillAll(context.Background(), store, 1); err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	for _, u := range store.updates {
		if u.WorkMode != "hybrid" {
			t.Errorf("WorkMode = %q, want hybrid (preserved)", u.WorkMode)
		}
	}
}

// The worker pool must process every row exactly once regardless of concurrency:
// each of N distinct jobs needing a write is updated exactly once, order aside.
// Run with -race to catch a store or counter data race.
func TestBackfill_Concurrent(t *testing.T) {
	const n = 200
	jobs := make([]db.Job, n)
	for i := range jobs {
		jobs[i] = db.Job{
			ID: int64(i + 1), Title: "Senior Go Developer", Company: "Acme",
			Source: "manual", ExternalID: "x", Location: "Berlin, Germany",
			Description: backfillJobDescription,
		}
	}
	store := &fakeStore{jobs: jobs}

	scanned, updated, err := backfillAll(context.Background(), store, 8)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != n || updated != n {
		t.Fatalf("scanned=%d updated=%d, want %d/%d", scanned, updated, n, n)
	}
	seen := make(map[int64]int, n)
	for _, u := range store.updates {
		seen[u.ID]++
	}
	if len(seen) != n {
		t.Fatalf("distinct updated ids = %d, want %d", len(seen), n)
	}
	for id, c := range seen {
		if c != 1 {
			t.Errorf("job id %d written %d times, want exactly 1", id, c)
		}
	}
}
