package resume

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/resumeextract"
)

// stamped builds a store whose user 7 has a CV uploaded at t, so a SetStructured stamped
// with the same t is a current (non-superseded) write.
func stamped(t time.Time) (*fakeRepo, *Store) {
	repo := newFakeRepo()
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t, Valid: true}
	return repo, New(&fakeBlobs{objs: map[string][]byte{}}, repo)
}

func TestStore_SetStructuredDerivesGeography(t *testing.T) {
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	repo, s := stamped(t1)

	st := resumeextract.Structured{FullName: "Jane", Location: "Kraków, Poland"}
	if err := s.SetStructured(context.Background(), 7, st, "model-x", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}

	assertGeo(t, repo, 7, []string{"pl"}, []string{"eu"}, []string{"Kraków"})
}

// A CV that states no location is NOT a candidate located nowhere — it is a candidate
// whose location is unknown. That must reach the database as NULL, not as an empty array.
func TestStore_SetStructuredWithNoLocationStoresUnknown(t *testing.T) {
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	repo, s := stamped(t1)

	st := resumeextract.Structured{FullName: "Jane"} // no Location at all
	if err := s.SetStructured(context.Background(), 7, st, "model-x", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}

	got := repo.geo[7]
	if got.Countries != nil || got.Regions != nil || got.Cities != nil {
		t.Errorf("geography = %+v, want all nil (unknown) for a CV stating no location", got)
	}
}

// The other half of the same distinction: the CV DID state a place, and the curated
// dictionary could not resolve it. That is a measurable coverage gap, not an unknown, so
// it must be stored as an empty array rather than NULL.
func TestStore_SetStructuredWithUnresolvableLocationStoresEmpty(t *testing.T) {
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	repo, s := stamped(t1)

	st := resumeextract.Structured{Location: "Greater Philadelphia"}
	if err := s.SetStructured(context.Background(), 7, st, "model-x", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}

	got := repo.geo[7]
	if got.Countries == nil || len(got.Countries) != 0 {
		t.Errorf("countries = %#v, want an empty non-nil slice (stated but unresolved)", got.Countries)
	}
	if got.Regions == nil || len(got.Regions) != 0 {
		t.Errorf("regions = %#v, want an empty non-nil slice", got.Regions)
	}
}

// A bare remote marker is a stated location that resolves to no PLACE. It must never
// arrive as the global region — that is the job reading of the string, not the candidate
// one — and it must not be mistaken for an unstated location either.
func TestStore_SetStructuredNeverStoresGlobalForARemoteCandidate(t *testing.T) {
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	repo, s := stamped(t1)

	st := resumeextract.Structured{Location: "REMOTE · WORLDWIDE"}
	if err := s.SetStructured(context.Background(), 7, st, "model-x", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}

	got := repo.geo[7]
	for _, r := range got.Regions {
		if r == "global" {
			t.Fatalf("regions = %v, want no global region for a candidate", got.Regions)
		}
	}
	if got.Regions == nil {
		t.Errorf("regions = nil, want an empty non-nil slice — the CV did state a location")
	}
}

// The geography rides in the same statement as the structure, so it inherits that
// statement's monotonic guard: a background derivation that completes for a CV which has
// since been replaced must write NEITHER.
func TestStore_SupersededStampWritesNeitherStructureNorGeography(t *testing.T) {
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	repo, s := stamped(t1)

	// A newer CV lands while the extraction for the older one is still running.
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1.Add(time.Hour), Valid: true}

	st := resumeextract.Structured{FullName: "From the superseded CV", Location: "Kraków, Poland"}
	if err := s.SetStructured(context.Background(), 7, st, "model-x", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}

	if len(repo.structured[7]) != 0 {
		t.Errorf("structure was written for a superseded CV: %s", repo.structured[7])
	}
	if got := repo.geo[7]; got.Countries != nil || got.Regions != nil || got.Cities != nil {
		t.Errorf("geography = %+v, want nothing written for a superseded CV", got)
	}
}

func TestStore_DeleteClearsGeography(t *testing.T) {
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	s := New(&fakeBlobs{objs: map[string][]byte{"resumes/7": []byte("cv")}}, repo)

	st := resumeextract.Structured{Location: "Kraków, Poland"}
	if err := s.SetStructured(context.Background(), 7, st, "m", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}
	if err := s.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if got := repo.geo[7]; got.Countries != nil || got.Regions != nil || got.Cities != nil {
		t.Errorf("geography after Delete = %+v, want cleared — it must not outlive the CV", got)
	}
}

func assertGeo(t *testing.T, repo *fakeRepo, userID int64, countries, regions, cities []string) {
	t.Helper()
	got := repo.geo[userID]
	if !equalStrings(got.Countries, countries) {
		t.Errorf("countries = %v, want %v", got.Countries, countries)
	}
	if !equalStrings(got.Regions, regions) {
		t.Errorf("regions = %v, want %v", got.Regions, regions)
	}
	if !equalStrings(got.Cities, cities) {
		t.Errorf("cities = %v, want %v", got.Cities, cities)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Geography is served under the SAME staleness rule as the structure it was derived
// from. A geography that outlived its CV would answer "where is this candidate" from a
// document that has been replaced.
func TestStore_GeographyServedOnlyWhileCurrent(t *testing.T) {
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	repo, s := stamped(t1)

	st := resumeextract.Structured{Location: "Kraków, Poland"}
	if err := s.SetStructured(context.Background(), 7, st, "m", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}

	got, ok, err := s.Geography(context.Background(), 7)
	if err != nil || !ok {
		t.Fatalf("Geography = (_, %v, %v), want (_, true, nil)", ok, err)
	}
	if len(got.Countries) != 1 || got.Countries[0] != "pl" {
		t.Errorf("countries = %v, want [pl]", got.Countries)
	}

	// A newer CV lands; the derived geography still carries the older stamp.
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1.Add(time.Hour), Valid: true}
	if _, ok, err := s.Geography(context.Background(), 7); ok || err != nil {
		t.Fatalf("Geography after re-upload = (_, %v, %v), want (_, false, nil) — stale is absent", ok, err)
	}
}

func TestStore_GeographyAbsentWhenNone(t *testing.T) {
	s := New(&fakeBlobs{objs: map[string][]byte{}}, newFakeRepo())
	if _, ok, err := s.Geography(context.Background(), 99); ok || err != nil {
		t.Fatalf("Geography for user with none = (_, %v, %v), want (_, false, nil)", ok, err)
	}
}
