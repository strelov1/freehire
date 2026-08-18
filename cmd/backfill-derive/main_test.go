package main

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/pgconv"
)

// fakeStore serves one page of jobs (keyset paging: AfterID 0 returns all, then
// empty) and records every UpdateJobDerived call. UpdateJobDerived is guarded so the
// concurrent worker pool can call it in parallel without a data race.
type fakeStore struct {
	jobs []db.Job
	// corrupt marks ids whose row cannot be read: the wide batch over any window
	// containing one fails with XX001, and so does fetching it singly.
	corrupt map[int64]bool
	mu      sync.Mutex
	updates []db.UpdateJobDerivedParams
}

// page is the keyset window the three read methods share, so the degrade path sees
// exactly the rows the wide batch would have returned.
func (f *fakeStore) page(afterID int64, batchSize int32) []db.Job {
	var out []db.Job
	for _, j := range f.jobs {
		if j.ID <= afterID {
			continue
		}
		out = append(out, j)
		if len(out) == int(batchSize) {
			break
		}
	}
	return out
}

// corruptedRow is the SQLSTATE a damaged TOAST pointer raises. A wide SELECT over a
// window containing one such row fails entirely; the id-only projection does not
// detoast, so it still answers.
func corruptedRow() error { return &pgconn.PgError{Code: "XX001", Message: "missing chunk number 0"} }

func (f *fakeStore) ListJobsByIDAfter(_ context.Context, arg db.ListJobsByIDAfterParams) ([]db.Job, error) {
	rows := f.page(arg.AfterID, arg.BatchSize)
	for _, j := range rows {
		if f.corrupt[j.ID] {
			return nil, corruptedRow()
		}
	}
	return rows, nil
}

func (f *fakeStore) ListJobIDsAfter(_ context.Context, arg db.ListJobIDsAfterParams) ([]int64, error) {
	rows := f.page(arg.AfterID, arg.BatchSize)
	ids := make([]int64, len(rows))
	for i, j := range rows {
		ids[i] = j.ID
	}
	return ids, nil
}

func (f *fakeStore) GetJob(_ context.Context, id int64) (db.Job, error) {
	if f.corrupt[id] {
		return db.Job{}, corruptedRow()
	}
	for _, j := range f.jobs {
		if j.ID == id {
			return j, nil
		}
	}
	return db.Job{}, pgx.ErrNoRows
}

// ListCompanySlugAliases: this fake catalogue holds no merges, so the registry is empty and
// the derivation is unchanged by it. A test that wants a merged row hands deriveRow its own.
func (f *fakeStore) ListCompanySlugAliases(context.Context) ([]db.ListCompanySlugAliasesRow, error) {
	return nil, nil
}

func (f *fakeStore) UpdateJobDerived(_ context.Context, arg db.UpdateJobDerivedParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, arg)
	return nil
}

// expectedDerived is the UpdateJobDerived the runner should write for a job: every
// dictionary facet from jobderive.Derive (the six original plus the four synthetic
// enrichment facets), the role_fingerprint computed from the freshly derived
// company_slug, and the recomputed public_slug/company_slug.
func expectedDerived(j db.Job) db.UpdateJobDerivedParams {
	d := jobderive.Derive(jobderive.Input{
		Title: j.Title, Company: j.Company, Source: j.Source, ExternalID: j.ExternalID,
		Location: j.Location, Description: j.Description, WorkMode: j.WorkMode,
	})
	fingerprint := jobhash.RoleFingerprint(db.UpsertJobParams{
		CompanySlug: d.CompanySlug, Title: j.Title, Description: j.Description,
	})
	return db.UpdateJobDerivedParams{
		ID: j.ID, Countries: d.Countries, Regions: d.Regions, Cities: d.Cities, WorkMode: d.WorkMode,
		Skills: d.Skills, Seniority: d.Seniority, Category: d.Category,
		IsTech:             pgconv.Bool(d.IsTech),
		PostingLanguage:    d.PostingLanguage,
		EmploymentType:     d.EmploymentType,
		EducationLevel:     d.EducationLevel,
		EnglishLevel:       d.EnglishLevel,
		ExperienceYearsMin: pgconv.Int4(d.ExperienceYearsMin),
		RoleFingerprint:    pgtype.Text{String: fingerprint, Valid: true},
		PublicSlug:         d.PublicSlug,
		CompanySlug:        d.CompanySlug,
	}
}

// seedDerived stamps a job with the exact values its own derivation produces, so a
// pass over it must rewrite nothing (idempotence precondition).
func seedDerived(j db.Job) db.Job {
	d := expectedDerived(j)
	j.Countries, j.Regions, j.Cities, j.WorkMode = d.Countries, d.Regions, d.Cities, d.WorkMode
	j.Skills, j.Seniority, j.Category, j.IsTech = d.Skills, d.Seniority, d.Category, d.IsTech
	j.PostingLanguage, j.EmploymentType = d.PostingLanguage, d.EmploymentType
	j.EducationLevel, j.EnglishLevel, j.ExperienceYearsMin = d.EducationLevel, d.EnglishLevel, d.ExperienceYearsMin
	j.RoleFingerprint, j.PublicSlug, j.CompanySlug = d.RoleFingerprint, d.PublicSlug, d.CompanySlug
	return j
}

// backfillJobDescription triggers both the original facets (skills) and the synthetic
// ones (English language, full-time, bachelor, 5 years) so the test verifies all of them.
const backfillJobDescription = "We use Go, PostgreSQL and Kubernetes. This is a " +
	"full-time role. A Bachelor's degree and 5+ years of experience are required."

func TestBackfill_RewritesAllDerivedInOnePass(t *testing.T) {
	job := db.Job{
		ID: 7, Title: "Senior Go Developer", Company: "Acme",
		Source: "manual", ExternalID: "x", Location: "Berlin, Germany",
		Description: backfillJobDescription,
		// facet/slug/fingerprint columns empty → the derived values differ → a write happens.
	}
	store := &fakeStore{jobs: []db.Job{job}}

	scanned, updated, slugsMoved, err := backfillAll(context.Background(), store, 1)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 1 || updated != 1 || slugsMoved != 1 {
		t.Fatalf("scanned=%d updated=%d slugsMoved=%d, want 1/1/1", scanned, updated, slugsMoved)
	}
	if len(store.updates) != 1 {
		t.Fatalf("got %d UpdateJobDerived calls, want 1", len(store.updates))
	}
	want := expectedDerived(job)
	if !reflect.DeepEqual(store.updates[0], want) {
		t.Errorf("UpdateJobDerived = %+v, want %+v", store.updates[0], want)
	}
	// Guard that the synthetic facets, the fingerprint, and the slugs were actually
	// derived (so this test can't pass with everything at zero values).
	got := store.updates[0]
	if got.PostingLanguage != "en" || got.EmploymentType != "full_time" ||
		got.EducationLevel != "bachelor" || !got.ExperienceYearsMin.Valid {
		t.Errorf("synthetic facets not derived: lang=%q type=%q edu=%q exp=%v",
			got.PostingLanguage, got.EmploymentType, got.EducationLevel, got.ExperienceYearsMin)
	}
	if got.RoleFingerprint.String == "" || got.PublicSlug == "" || got.CompanySlug == "" {
		t.Errorf("fingerprint/slugs not derived: fp=%q public=%q company=%q",
			got.RoleFingerprint.String, got.PublicSlug, got.CompanySlug)
	}
}

func TestBackfill_IsIdempotent(t *testing.T) {
	job := seedDerived(db.Job{
		ID: 7, Title: "Senior Go Developer", Company: "Acme",
		Source: "manual", ExternalID: "x", Location: "Berlin, Germany",
		Description: backfillJobDescription,
	})

	store := &fakeStore{jobs: []db.Job{job}}
	scanned, updated, slugsMoved, err := backfillAll(context.Background(), store, 1)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 1 || updated != 0 || slugsMoved != 0 {
		t.Fatalf("scanned=%d updated=%d slugsMoved=%d, want 1/0/0 (unchanged row skipped)", scanned, updated, slugsMoved)
	}
	if len(store.updates) != 0 {
		t.Errorf("expected no writes for an unchanged row, got %d", len(store.updates))
	}
}

// A row whose facets and fingerprint are already current but whose slug is stale (e.g.
// after a slug-builder change) must still be rewritten AND counted as a slug move, so
// the caller knows to reconcile the companies catalogue.
func TestBackfill_CountsSlugMove(t *testing.T) {
	job := seedDerived(db.Job{
		ID: 7, Title: "Senior Go Developer", Company: "Acme",
		Source: "manual", ExternalID: "x", Location: "Berlin, Germany",
		Description: backfillJobDescription,
	})
	job.CompanySlug = "stale-company-slug" // only the slug is out of date

	store := &fakeStore{jobs: []db.Job{job}}
	scanned, updated, slugsMoved, err := backfillAll(context.Background(), store, 1)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 1 || updated != 1 || slugsMoved != 1 {
		t.Fatalf("scanned=%d updated=%d slugsMoved=%d, want 1/1/1", scanned, updated, slugsMoved)
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
	if _, _, _, err := backfillAll(context.Background(), store, 1); err != nil {
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

	scanned, updated, slugsMoved, err := backfillAll(context.Background(), store, 8)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != n || updated != n || slugsMoved != n {
		t.Fatalf("scanned=%d updated=%d slugsMoved=%d, want %d/%d/%d", scanned, updated, slugsMoved, n, n, n)
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

// A full pass rewrites five million rows and, until now, said nothing at all between
// "starting" and "done" — a silence measured in hours during which a run that was
// working and a run that had wedged looked identical. The only way to tell them apart
// was pg_stat_activity, and the only way to answer "how much longer" was to guess.
//
// cmd/prune's scan already learned this ("the first prod run was impossible to
// distinguish from a hang"); this is the same fix on the other long worker.
func TestBackfill_ReportsProgressWhileItRuns(t *testing.T) {
	jobs := make([]db.Job, 0, 25)
	for i := 1; i <= 25; i++ {
		jobs = append(jobs, db.Job{
			ID: int64(i), Title: "Senior Go Developer", Company: "Acme",
			Source: "manual", ExternalID: "x", Location: "Berlin, Germany",
			Description: backfillJobDescription,
		})
	}
	store := &fakeStore{jobs: jobs}

	var mu sync.Mutex
	var seen []int64
	// One worker, so the cadence is deterministic; the counter itself is atomic, so
	// exactly one goroutine observes each multiple whatever the concurrency.
	scanned, _, _, err := backfillProgress(context.Background(), store, 1, 10,
		func(n, updated, slugs int64) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, n)
		})
	if err != nil {
		t.Fatalf("backfillProgress: %v", err)
	}
	if scanned != 25 {
		t.Fatalf("scanned=%d, want 25", scanned)
	}
	if len(seen) != 2 {
		t.Fatalf("reported at %v, want two reports (at 10 and 20) for 25 rows every 10", seen)
	}
	if seen[0] != 10 || seen[1] != 20 {
		t.Errorf("reported at %v, want [10 20] — the report must fire ON the multiple", seen)
	}
}

// Progress reporting must not change what a pass does. The default entry point keeps
// its signature and its counts.
func TestBackfill_ProgressDoesNotChangeTheResult(t *testing.T) {
	job := db.Job{
		ID: 7, Title: "Senior Go Developer", Company: "Acme",
		Source: "manual", ExternalID: "x", Location: "Berlin, Germany",
		Description: backfillJobDescription,
	}
	store := &fakeStore{jobs: []db.Job{job}}
	scanned, updated, slugsMoved, err := backfillAll(context.Background(), store, 1)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 1 || updated != 1 || slugsMoved != 1 {
		t.Errorf("scanned=%d updated=%d slugsMoved=%d, want 1/1/1", scanned, updated, slugsMoved)
	}
}

// The backfill records no resume point, so an aborting scan is not one failed run: every
// later run restarts at id 0 and dies at the same row, leaving every deterministic column
// past it stale forever. Skipping the unreadable row is what makes the pass finishable.
func TestBackfill_SkipsACorruptedRowAndFinishesTheScan(t *testing.T) {
	jobs := make([]db.Job, 0, 3)
	for i := 1; i <= 3; i++ {
		jobs = append(jobs, db.Job{
			ID: int64(i), Title: "Senior Go Developer", Company: "Acme",
			Source: "manual", ExternalID: "x", Location: "Berlin, Germany",
			Description: backfillJobDescription,
		})
	}
	store := &fakeStore{jobs: jobs, corrupt: map[int64]bool{2: true}}

	scanned, updated, _, err := backfillAll(context.Background(), store, 1)
	if err != nil {
		t.Fatalf("backfillAll: %v — a corrupted row must not end the run", err)
	}
	if scanned != 2 || updated != 2 {
		t.Errorf("scanned=%d updated=%d, want 2/2 (the readable rows either side of the damaged one)", scanned, updated)
	}
	for _, u := range store.updates {
		if u.ID == 2 {
			t.Error("the corrupted row was written")
		}
	}
	// The row AFTER the damaged one is the point: an abort would have left it stale.
	var sawThird bool
	for _, u := range store.updates {
		if u.ID == 3 {
			sawThird = true
		}
	}
	if !sawThird {
		t.Error("the scan stopped at the corrupted row instead of continuing past it")
	}
}

// The exhaustion signal is the keyset cursor, not the page size. A page that skipped a
// corrupted row is legitimately SHORT, so a "fewer rows than the batch → end of table"
// test would stop here and report a complete pass — silently leaving every row after the
// first damaged one stale. That is worse than the abort this change replaces, because
// nothing in the run says so.
func TestBackfill_ShortDegradedPageDoesNotEndTheScan(t *testing.T) {
	const n = backfillBatchSize + 1
	jobs := make([]db.Job, 0, n)
	for i := 1; i <= n; i++ {
		jobs = append(jobs, db.Job{
			ID: int64(i), Title: "Go Developer", Company: "Acme",
			Source: "manual", ExternalID: "x", Location: "Berlin, Germany",
		})
	}
	// Inside the FIRST page, so that page comes back one row short of the batch size.
	store := &fakeStore{jobs: jobs, corrupt: map[int64]bool{250: true}}

	scanned, _, _, err := backfillAll(context.Background(), store, 4)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != n-1 {
		t.Fatalf("scanned=%d, want %d — the scan ended at the short page instead of following the cursor", scanned, n-1)
	}
	// The row past the first page is the proof: it is only reachable on a second page.
	var sawLast bool
	for _, u := range store.updates {
		if u.ID == int64(n) {
			sawLast = true
		}
	}
	if !sawLast {
		t.Errorf("row %d (page 2) was never scanned", n)
	}
}
