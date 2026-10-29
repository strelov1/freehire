package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeReconciler struct {
	inserts, deletes [][]string
	failInsert       bool
}

func (f *fakeReconciler) InsertMissingCompaniesForSlugs(_ context.Context, slugs []string) (int64, error) {
	if f.failInsert {
		return 0, errors.New("boom")
	}
	f.inserts = append(f.inserts, slugs)
	return int64(len(slugs)), nil
}

func (f *fakeReconciler) DeleteOrphanCompaniesForSlugs(_ context.Context, slugs []string) (int64, error) {
	f.deletes = append(f.deletes, slugs)
	return 1, nil
}

// The reconcile is many short statements, never one long one: that is the whole fix.
func TestReconcileMovedSlugsChunks(t *testing.T) {
	r := &fakeReconciler{}
	created, orphaned, err := reconcileMovedSlugs(context.Background(), r,
		[]string{"a", "b", "c", "d", "e"}, []string{"x", "y", "z"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	wantIns := [][]string{{"a", "b"}, {"c", "d"}, {"e"}}
	wantDel := [][]string{{"x", "y"}, {"z"}}
	if !reflect.DeepEqual(r.inserts, wantIns) || !reflect.DeepEqual(r.deletes, wantDel) {
		t.Errorf("inserts %v deletes %v; want %v and %v", r.inserts, r.deletes, wantIns, wantDel)
	}
	if created != 5 || orphaned != 2 {
		t.Errorf("created %d orphaned %d; want 5 and 2", created, orphaned)
	}
}

func TestReconcileMovedSlugsNothingMoved(t *testing.T) {
	r := &fakeReconciler{}
	if _, _, err := reconcileMovedSlugs(context.Background(), r, nil, nil, 500); err != nil {
		t.Fatal(err)
	}
	if len(r.inserts)+len(r.deletes) != 0 {
		t.Errorf("a run that moved no slug wrote: %v %v", r.inserts, r.deletes)
	}
}

// A failed insert stops before any delete: dropping the old row while the new one is
// missing would 404 both.
func TestReconcileMovedSlugsStopsOnInsertError(t *testing.T) {
	r := &fakeReconciler{failInsert: true}
	if _, _, err := reconcileMovedSlugs(context.Background(), r, []string{"a"}, []string{"x"}, 500); err == nil {
		t.Fatal("want an error")
	}
	if len(r.deletes) != 0 {
		t.Errorf("deleted after a failed insert: %v", r.deletes)
	}
}

// The pass reports the distinct slugs it moved — the reconcile's scope — not just a count.
func TestBackfillReportsMovedSlugs(t *testing.T) {
	jobs := []db.Job{
		{ID: 1, Title: "Go Developer", Company: "Acme", Source: "manual", ExternalID: "1", CompanySlug: "acme-old"},
		{ID: 2, Title: "Go Developer", Company: "Acme", Source: "manual", ExternalID: "2", CompanySlug: "acme-old"},
		{ID: 3, Title: "Go Developer", Company: "Beta", Source: "manual", ExternalID: "3", CompanySlug: "beta-old"},
	}
	pass, err := backfillPass(context.Background(), &fakeStore{jobs: jobs}, 2, scanWindow{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"acme-old", "beta-old"}; !reflect.DeepEqual(pass.MovedFrom, want) {
		t.Errorf("MovedFrom = %v, want %v", pass.MovedFrom, want)
	}
	if want := []string{"acme", "beta"}; !reflect.DeepEqual(pass.MovedTo, want) {
		t.Errorf("MovedTo = %v, want %v", pass.MovedTo, want)
	}
	if pass.SlugsMoved != 3 {
		t.Errorf("SlugsMoved = %d, want 3 rows", pass.SlugsMoved)
	}
}

// failingStore lets the first derived write through and fails the rest.
type failingStore struct {
	*fakeStore
	*fakeReconciler
	writes int
}

func (f *failingStore) UpdateJobDerived(ctx context.Context, arg db.UpdateJobDerivedParams) error {
	f.writes++
	if f.writes > 1 {
		return errors.New("write failed")
	}
	return f.fakeStore.UpdateJobDerived(ctx, arg)
}

// A pass that fails part-way still reconciles the slugs it DID move: those rows are
// committed, and skipping their reconcile 404s every new slug.
func TestDerivePassReconcilesAfterAFailedPass(t *testing.T) {
	jobs := []db.Job{
		{ID: 1, Title: "Go Developer", Company: "Acme", Source: "manual", ExternalID: "1", CompanySlug: "acme-old"},
		{ID: 2, Title: "Go Developer", Company: "Beta", Source: "manual", ExternalID: "2", CompanySlug: "beta-old"},
	}
	store := &failingStore{fakeStore: &fakeStore{jobs: jobs}, fakeReconciler: &fakeReconciler{}}
	_, _, err := derivePass(context.Background(), store, 1, scanWindow{})
	if err == nil || err.Error() != "write failed" {
		t.Fatalf("err = %v, want the pass's own error", err)
	}
	if want := [][]string{{"acme"}}; !reflect.DeepEqual(store.inserts, want) {
		t.Errorf("inserts = %v, want %v", store.inserts, want)
	}
	if want := [][]string{{"acme-old"}}; !reflect.DeepEqual(store.deletes, want) {
		t.Errorf("deletes = %v, want %v", store.deletes, want)
	}
}
