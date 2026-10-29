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
	aliases          []db.InsertDerivedCompanySlugAliasesParams
	calls            []string
	failInsert       bool
}

func (f *fakeReconciler) InsertDerivedCompanySlugAliases(_ context.Context, arg db.InsertDerivedCompanySlugAliasesParams) (int64, error) {
	f.calls = append(f.calls, "alias")
	f.aliases = append(f.aliases, arg)
	return int64(len(arg.AliasSlugs)), nil
}

func (f *fakeReconciler) InsertMissingCompaniesForSlugs(_ context.Context, slugs []string) (int64, error) {
	f.calls = append(f.calls, "insert")
	if f.failInsert {
		return 0, errors.New("boom")
	}
	f.inserts = append(f.inserts, slugs)
	return int64(len(slugs)), nil
}

func (f *fakeReconciler) DeleteOrphanCompaniesForSlugs(_ context.Context, slugs []string) (int64, error) {
	f.calls = append(f.calls, "delete")
	f.deletes = append(f.deletes, slugs)
	return 1, nil
}

// The reconcile is many short statements, never one long one: that is the whole fix.
func TestReconcileMovedSlugsChunks(t *testing.T) {
	r := &fakeReconciler{}
	rec, err := reconcileMovedSlugs(context.Background(), r, backfillRun{
		MovedTo: []string{"a", "b", "c", "d", "e"}, MovedFrom: []string{"x", "y", "z"},
	}, 2)
	created, orphaned := rec.created, rec.orphaned
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
	if _, err := reconcileMovedSlugs(context.Background(), r, backfillRun{}, 500); err != nil {
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
	if _, err := reconcileMovedSlugs(context.Background(), r, backfillRun{MovedTo: []string{"a"}, MovedFrom: []string{"x"}}, 500); err == nil {
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

// The order is what keeps every URL answering: the canonical row exists before a redirect
// points at it, and the redirect exists before the old row goes.
func TestReconcileMovedSlugsOrder(t *testing.T) {
	r := &fakeReconciler{}
	_, err := reconcileMovedSlugs(context.Background(), r, backfillRun{
		MovedTo: []string{"acme"}, MovedFrom: []string{"acme-pvt"},
		Aliases: []slugAlias{{Alias: "acme-pvt", Canonical: "acme", FoldedKey: "acme"}},
	}, 500)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"insert", "alias", "delete"}; !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
	want := db.InsertDerivedCompanySlugAliasesParams{
		AliasSlugs: []string{"acme-pvt"}, CanonicalSlugs: []string{"acme"}, FoldedKeys: []string{"acme"},
	}
	if len(r.aliases) != 1 || !reflect.DeepEqual(r.aliases[0], want) {
		t.Errorf("aliases = %+v, want %+v", r.aliases, want)
	}
}

// A slug the rule moved under an unchanged name earns a redirect. One whose rows say the
// employer was renamed, or that split between two new slugs, does not.
func TestMovedSlugsAliasEligibility(t *testing.T) {
	m := newMovedSlugs()
	// Pure rule change: "Acme Pvt Ltd" was minted acme-pvt and now keys at acme.
	m.add("acme-pvt", "acme", "Acme Pvt Ltd")
	m.add("acme-pvt", "acme", "Acme Pvt Ltd")
	// Renamed: the stored slug is not what any rule made of this name.
	m.add("oldco", "newco", "NewCo")
	// Split: one old slug, two new ones.
	m.add("beta-pvt", "beta", "Beta Pvt Ltd")
	m.add("beta-pvt", "beta-labs", "Beta Labs Pvt Ltd")
	// Mixed: a qualifying row and a renamed one share the old slug.
	m.add("gamma-pvt", "gamma", "Gamma Pvt Ltd")
	m.add("gamma-pvt", "delta", "Delta")

	// Same folded key, two canons: "Zeta Pvt Ltd" and "Ze Ta Pvt Ltd" both key at "zeta"
	// but slug apart, so neither redirect is recorded.
	m.add("zeta-pvt", "zeta", "Zeta Pvt Ltd")
	m.add("ze-ta-pvt", "ze-ta", "Ze Ta Pvt Ltd")

	got := m.qualifiedAliases()
	want := []slugAlias{{Alias: "acme-pvt", Canonical: "acme", FoldedKey: "acme"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("aliases = %+v, want %+v", got, want)
	}
}

// The pass hands its eligible redirects to the reconcile.
func TestBackfillReportsEarnedAliases(t *testing.T) {
	jobs := []db.Job{
		{ID: 1, Title: "Go Developer", Company: "Acme Pvt Ltd", Source: "manual", ExternalID: "1", CompanySlug: "acme-pvt"},
		{ID: 2, Title: "Go Developer", Company: "NewCo", Source: "manual", ExternalID: "2", CompanySlug: "oldco"},
	}
	pass, err := backfillPass(context.Background(), &fakeStore{jobs: jobs}, 2, scanWindow{})
	if err != nil {
		t.Fatal(err)
	}
	want := []slugAlias{{Alias: "acme-pvt", Canonical: "acme", FoldedKey: "acme"}}
	if !reflect.DeepEqual(pass.Aliases, want) {
		t.Errorf("Aliases = %+v, want %+v", pass.Aliases, want)
	}
}
