package main

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
)

// fakeStore serves one keyset page (AfterID 0 returns all, then empty) and records
// every UpdateJobRoleFingerprint call, guarded for the concurrent worker pool.
type fakeStore struct {
	jobs    []db.Job
	mu      sync.Mutex
	updates []db.UpdateJobRoleFingerprintParams
}

func (f *fakeStore) ListJobsByIDAfter(_ context.Context, arg db.ListJobsByIDAfterParams) ([]db.Job, error) {
	if arg.AfterID != 0 {
		return nil, nil
	}
	return f.jobs, nil
}

func (f *fakeStore) UpdateJobRoleFingerprint(_ context.Context, arg db.UpdateJobRoleFingerprintParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, arg)
	return 1, nil
}

func wantFingerprint(j db.Job) string {
	return jobhash.RoleFingerprint(db.UpsertJobParams{
		CompanySlug: j.CompanySlug,
		Title:       j.Title,
		Description: j.Description,
	})
}

func TestFingerprintUpdate_ChangedWhenStale(t *testing.T) {
	j := db.Job{ID: 7, CompanySlug: "towa", Title: "Senior Engineer, Krakau", Description: "Build things.",
		RoleFingerprint: pgtype.Text{String: "stale-old-hash", Valid: true}}
	params, changed := fingerprintUpdate(j)
	if !changed {
		t.Fatal("stale fingerprint should be reported changed")
	}
	if params.ID != 7 {
		t.Errorf("id = %d, want 7", params.ID)
	}
	if params.RoleFingerprint.String != wantFingerprint(j) {
		t.Errorf("fingerprint = %q, want freshly computed %q", params.RoleFingerprint.String, wantFingerprint(j))
	}
}

func TestFingerprintUpdate_UnchangedWhenAlreadyCurrent(t *testing.T) {
	j := db.Job{ID: 8, CompanySlug: "towa", Title: "Senior Engineer, Wien", Description: "Build things."}
	j.RoleFingerprint = pgtype.Text{String: wantFingerprint(j), Valid: true}
	if _, changed := fingerprintUpdate(j); changed {
		t.Error("a fingerprint already matching the new algorithm must not be rewritten")
	}
}

func TestBackfillAll_WritesOnlyStaleRows(t *testing.T) {
	current := db.Job{ID: 1, CompanySlug: "acme", Title: "Backend Engineer, Berlin", Description: "x"}
	current.RoleFingerprint = pgtype.Text{String: wantFingerprint(current), Valid: true}
	stale := db.Job{ID: 2, CompanySlug: "acme", Title: "Backend Engineer, Munich", Description: "x",
		RoleFingerprint: pgtype.Text{String: "old", Valid: true}}

	store := &fakeStore{jobs: []db.Job{current, stale}}
	scanned, updated, err := backfillAll(context.Background(), store, 1)
	if err != nil {
		t.Fatalf("backfillAll: %v", err)
	}
	if scanned != 2 || updated != 1 {
		t.Fatalf("scanned=%d updated=%d, want scanned=2 updated=1", scanned, updated)
	}
	if len(store.updates) != 1 || store.updates[0].ID != 2 {
		t.Fatalf("updates = %+v, want a single write for the stale row id=2", store.updates)
	}
}
