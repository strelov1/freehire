//go:build integration

// Integration tests for the duplicate-closure geography union: the set of open rows a
// SEARCHABLE row represents, and the union of their countries/regions/cities. It replaces the
// (company_slug, role_fingerprint) union, which could only ever see the exact role pass's
// clusters — the fuzzy-description and aggregator passes act on rows whose fingerprints
// DIFFER, so their members' geography left the index with them (issue #2225).
//
// The traversal, the open-rows-only scope, the chain across passes and the cycle behaviour are
// SQL properties verifiable only against a real Postgres.
//
// Chains are built from the two markers the sibling test files already define: markDuplicate
// (fuzzy_dedup) writes duplicate_of_role as an exact pass would, and markFuzzy
// (duplicate_marker_no_clobber) drives the real MarkFuzzyDuplicatesForCompany.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"slices"
	"testing"
)

// closureRow is one posting in its own geography. Fingerprints are varied deliberately: the
// closure must NOT depend on them, since a fuzzy-suppressed row never shares its canon's.
func closureRow(externalID, fingerprint string, countries, regions, cities []string) UpsertJobParams {
	p := withFingerprint(externalID, "Senior Full Stack Engineer", fingerprint)
	p.Countries = countries
	p.Regions = regions
	p.Cities = cities
	return p
}

// testCompany is the slug ingestParams mints; the fuzzy marker query scopes itself to it.
const testCompany = "acme"

// closureGeo is the shape both queries answer in, so one finder serves both. That the two
// generated row types convert to it — and to each other — is the point: the wave query is the
// whole-catalogue one narrowed by a seed, not a second answer.
type closureGeo struct {
	OwnerID                    int64
	Countries, Regions, Cities []string
}

func allClosures(rows []DuplicateClosureGeoAllRow) []closureGeo {
	out := make([]closureGeo, len(rows))
	for i, r := range rows {
		out[i] = closureGeo(r)
	}
	return out
}

func forClosures(rows []DuplicateClosureGeoForRow) []closureGeo {
	out := make([]closureGeo, len(rows))
	for i, r := range rows {
		out[i] = closureGeo(r)
	}
	return out
}

// closureOf returns one owner's union, or nil when the owner is absent.
func closureOf(rows []closureGeo, owner int64) *closureGeo {
	for i := range rows {
		if rows[i].OwnerID == owner {
			return &rows[i]
		}
	}
	return nil
}

func TestDuplicateClosureGeo_UnionsARoleCluster(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	const fp = "fp-fullstack"
	mustUpsert(t, q, closureRow("c-1", fp, []string{"de"}, []string{"eu"}, []string{"Düsseldorf"}))
	mustUpsert(t, q, closureRow("c-2", fp, []string{"pl"}, []string{"eu"}, []string{"Kraków"}))
	mustUpsert(t, q, closureRow("c-3", fp, []string{"at"}, []string{"eu"}, []string{"Wien"}))
	// A different role at the same company must not bleed in.
	mustUpsert(t, q, closureRow("other-1", "fp-backend", []string{"us"}, []string{"north_america"}, []string{"Austin"}))

	canonID, _ := dupOf(t, pool, "c-1")
	for _, ext := range []string{"c-2", "c-3"} {
		id, _ := dupOf(t, pool, ext)
		markDuplicate(t, pool, id, canonID)
	}

	rows, err := q.DuplicateClosureGeoAll(ctx)
	if err != nil {
		t.Fatalf("DuplicateClosureGeoAll: %v", err)
	}
	all := allClosures(rows)
	got := closureOf(all, canonID)
	if got == nil {
		t.Fatalf("owner %d absent from the union; got %d rows", canonID, len(rows))
	}
	if want := []string{"at", "de", "pl"}; !slices.Equal(got.Countries, want) {
		t.Errorf("countries = %v, want %v", got.Countries, want)
	}
	if want := []string{"eu"}; !slices.Equal(got.Regions, want) {
		t.Errorf("regions = %v, want %v", got.Regions, want)
	}
	if want := []string{"Düsseldorf", "Kraków", "Wien"}; !slices.Equal(got.Cities, want) {
		t.Errorf("cities = %v, want %v", got.Cities, want)
	}

	// The unrelated role represents nobody, so it is absent rather than present with its own
	// geography — that absence is what keeps the rebuild's lookup map small.
	otherID, _ := dupOf(t, pool, "other-1")
	if closureOf(all, otherID) != nil {
		t.Errorf("owner %d represents nobody and must be absent from the union", otherID)
	}
}

func TestDuplicateClosureGeo_CarriesAFuzzySuppressedCity(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// The shape from issue #2225: the fuzzy pass suppresses a per-city variant whose
	// description names its city, so the two rows have DIFFERENT fingerprints by construction
	// and no fingerprint-keyed union could ever see the pair.
	mustUpsert(t, q, closureRow("cobs-calgary", "fp-calgary", []string{"ca"}, []string{"north_america"}, []string{"Calgary"}))
	mustUpsert(t, q, closureRow("cobs-chestermere", "fp-chestermere", []string{"ca"}, []string{"north_america"}, []string{"Chestermere"}))

	canonID, _ := dupOf(t, pool, "cobs-calgary")
	hiddenID, _ := dupOf(t, pool, "cobs-chestermere")
	markFuzzy(t, q, testCompany, hiddenID, canonID)

	rows, err := q.DuplicateClosureGeoAll(ctx)
	if err != nil {
		t.Fatalf("DuplicateClosureGeoAll: %v", err)
	}
	got := closureOf(allClosures(rows), canonID)
	if got == nil {
		t.Fatalf("fuzzy canon %d absent from the union; got %d rows", canonID, len(rows))
	}
	if want := []string{"Calgary", "Chestermere"}; !slices.Equal(got.Cities, want) {
		t.Errorf("cities = %v, want %v — the suppressed city must stay searchable", got.Cities, want)
	}
}

func TestDuplicateClosureGeo_FollowsAChainAcrossPasses(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// C --role--> B --fuzzy--> A. B is a role canon a later pass suppressed, so its cluster's
	// geography is stranded on an unsearchable row unless the walk follows the chain.
	mustUpsert(t, q, closureRow("chain-a", "fp-a", []string{"ca"}, []string{"north_america"}, []string{"Toronto"}))
	mustUpsert(t, q, closureRow("chain-b", "fp-b", []string{"ca"}, []string{"north_america"}, []string{"Belleville"}))
	mustUpsert(t, q, closureRow("chain-c", "fp-b", []string{"ca"}, []string{"north_america"}, []string{"Scarborough"}))

	aID, _ := dupOf(t, pool, "chain-a")
	bID, _ := dupOf(t, pool, "chain-b")
	cID, _ := dupOf(t, pool, "chain-c")
	markDuplicate(t, pool, cID, bID)
	markFuzzy(t, q, testCompany, bID, aID)

	rows, err := q.DuplicateClosureGeoAll(ctx)
	if err != nil {
		t.Fatalf("DuplicateClosureGeoAll: %v", err)
	}
	all := allClosures(rows)
	got := closureOf(all, aID)
	if got == nil {
		t.Fatalf("owner %d absent from the union; got %d rows", aID, len(rows))
	}
	if want := []string{"Belleville", "Scarborough", "Toronto"}; !slices.Equal(got.Cities, want) {
		t.Errorf("cities = %v, want %v — a two-hop chain must not strand geography", got.Cities, want)
	}
	// B is not searchable, so it must not appear as an owner of its own.
	if closureOf(all, bID) != nil {
		t.Errorf("row %d is itself a duplicate and must not be an owner", bID)
	}
}

func TestDuplicateClosureGeo_ExcludesClosedMembers(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	mustUpsert(t, q, closureRow("open-canon", "fp-open", []string{"de"}, []string{"eu"}, []string{"Düsseldorf"}))
	mustUpsert(t, q, closureRow("open-dup", "fp-open2", []string{"pl"}, []string{"eu"}, []string{"Kraków"}))
	mustUpsert(t, q, closureRow("closed-dup", "fp-open3", []string{"at"}, []string{"eu"}, []string{"Wien"}))

	canonID, _ := dupOf(t, pool, "open-canon")
	openDupID, _ := dupOf(t, pool, "open-dup")
	closedDupID, _ := dupOf(t, pool, "closed-dup")
	markFuzzy(t, q, testCompany, openDupID, canonID)
	markFuzzy(t, q, testCompany, closedDupID, canonID)
	closeJob(t, pool, closedDupID)

	rows, err := q.DuplicateClosureGeoAll(ctx)
	if err != nil {
		t.Fatalf("DuplicateClosureGeoAll: %v", err)
	}
	got := closureOf(allClosures(rows), canonID)
	if got == nil {
		t.Fatalf("owner %d absent from the union", canonID)
	}
	// A closed member's city must not resurrect a location the role is no longer open in.
	if want := []string{"Düsseldorf", "Kraków"}; !slices.Equal(got.Cities, want) {
		t.Errorf("cities = %v, want %v (the closed member must not contribute)", got.Cities, want)
	}
}

func TestDuplicateClosureGeo_ClosedOwnerIsNotAnOwner(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	mustUpsert(t, q, closureRow("gone-canon", "fp-gone", []string{"de"}, []string{"eu"}, []string{"Düsseldorf"}))
	mustUpsert(t, q, closureRow("orphan-dup", "fp-gone2", []string{"pl"}, []string{"eu"}, []string{"Kraków"}))

	canonID, _ := dupOf(t, pool, "gone-canon")
	dupID, _ := dupOf(t, pool, "orphan-dup")
	markFuzzy(t, q, testCompany, dupID, canonID)
	closeJob(t, pool, canonID)

	rows, err := q.DuplicateClosureGeoAll(ctx)
	if err != nil {
		t.Fatalf("DuplicateClosureGeoAll: %v", err)
	}
	all := allClosures(rows)
	// A closed owner is not searchable, so it is not a seed; its member is a duplicate and
	// never a seed either. Re-pointing the orphan is the marker refresh's job, not this read's.
	if closureOf(all, canonID) != nil {
		t.Errorf("closed row %d must not be an owner", canonID)
	}
	if closureOf(all, dupID) != nil {
		t.Errorf("row %d is a duplicate and must not be an owner", dupID)
	}
}

func TestDuplicateClosureGeo_AClosedIntermediateCutsTheWalk(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// C --role--> B(CLOSED) --fuzzy--> A. The walk follows open rows only, so C contributes to
	// nobody. That is the documented behaviour, not an oversight: C carries a marker and is out
	// of the index anyway, and re-pointing it belongs to the marker refresh — the role recompute
	// picks min(id) among a cluster's OPEN rows, and the fuzzy pass releases a marker whose canon
	// closed. This test exists so the walk cannot start following closed rows unnoticed.
	mustUpsert(t, q, closureRow("cut-a", "fp-cut-a", []string{"ca"}, []string{"north_america"}, []string{"Toronto"}))
	mustUpsert(t, q, closureRow("cut-b", "fp-cut-b", []string{"ca"}, []string{"north_america"}, []string{"Belleville"}))
	mustUpsert(t, q, closureRow("cut-c", "fp-cut-b", []string{"ca"}, []string{"north_america"}, []string{"Scarborough"}))

	aID, _ := dupOf(t, pool, "cut-a")
	bID, _ := dupOf(t, pool, "cut-b")
	cID, _ := dupOf(t, pool, "cut-c")
	markDuplicate(t, pool, cID, bID)
	markFuzzy(t, q, testCompany, bID, aID)
	closeJob(t, pool, bID)

	rows, err := q.DuplicateClosureGeoAll(ctx)
	if err != nil {
		t.Fatalf("DuplicateClosureGeoAll: %v", err)
	}
	// A now represents no OPEN row at all, so it is absent rather than carrying Scarborough.
	if got := closureOf(allClosures(rows), aID); got != nil {
		t.Errorf("owner %d has only a closed member and must be absent; got cities %v", aID, got.Cities)
	}
}

func TestDuplicateClosureGeo_CycleTerminates(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Two rows marked onto each other. No pass produces this today, but the walk must not
	// hang on it: the seed is rows that are nobody's duplicate, so a cycle has no entry point.
	mustUpsert(t, q, closureRow("loop-a", "fp-loop-a", []string{"de"}, []string{"eu"}, []string{"Düsseldorf"}))
	mustUpsert(t, q, closureRow("loop-b", "fp-loop-b", []string{"pl"}, []string{"eu"}, []string{"Kraków"}))

	aID, _ := dupOf(t, pool, "loop-a")
	bID, _ := dupOf(t, pool, "loop-b")
	markFuzzy(t, q, testCompany, aID, bID)
	markFuzzy(t, q, testCompany, bID, aID)

	rows, err := q.DuplicateClosureGeoAll(ctx)
	if err != nil {
		t.Fatalf("DuplicateClosureGeoAll must terminate on a marker cycle: %v", err)
	}
	all := allClosures(rows)
	if closureOf(all, aID) != nil || closureOf(all, bID) != nil {
		t.Errorf("a cycle has no searchable row and must yield no owner; got %d rows", len(rows))
	}

	// The wave query seeds the same way, so the structural argument covers it too — a caller
	// that names a cycle member by id still gets nothing rather than a walk bounded only by
	// the depth backstop. Asserted here because the two seeds are copies, not one shared body.
	forRows, err := q.DuplicateClosureGeoFor(ctx, []int64{aID, bID})
	if err != nil {
		t.Fatalf("DuplicateClosureGeoFor must terminate on a marker cycle: %v", err)
	}
	if len(forRows) != 0 {
		t.Errorf("cycle members are not canonical and must not seed the wave query; got %d rows", len(forRows))
	}
}

func TestDuplicateClosureGeoFor_AnswersOnlyTheRequestedOwners(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	mustUpsert(t, q, closureRow("wave-canon", "fp-wave", []string{"de"}, []string{"eu"}, []string{"Düsseldorf"}))
	mustUpsert(t, q, closureRow("wave-dup", "fp-wave2", []string{"pl"}, []string{"eu"}, []string{"Kraków"}))
	mustUpsert(t, q, closureRow("wave-other", "fp-other", []string{"us"}, []string{"north_america"}, []string{"Austin"}))

	canonID, _ := dupOf(t, pool, "wave-canon")
	dupID, _ := dupOf(t, pool, "wave-dup")
	otherID, _ := dupOf(t, pool, "wave-other")
	markFuzzy(t, q, testCompany, dupID, canonID)

	rows, err := q.DuplicateClosureGeoFor(ctx, []int64{canonID, otherID, 999999})
	if err != nil {
		t.Fatalf("DuplicateClosureGeoFor: %v", err)
	}
	forRows := forClosures(rows)
	got := closureOf(forRows, canonID)
	if got == nil {
		t.Fatalf("requested owner %d absent; got %d rows", canonID, len(rows))
	}
	if want := []string{"Düsseldorf", "Kraków"}; !slices.Equal(got.Cities, want) {
		t.Errorf("cities = %v, want %v", got.Cities, want)
	}

	// Unlike the whole-catalogue query, the wave variant answers for a row that represents
	// nobody: the caller asked about it, and its self-union is a documented no-op merge. That
	// keeps the caller to one error branch instead of telling "absent" apart from a failure.
	if self := closureOf(forRows, otherID); self == nil {
		t.Errorf("owner %d was requested and must be answered", otherID)
	} else if want := []string{"Austin"}; !slices.Equal(self.Cities, want) {
		t.Errorf("cities = %v, want %v (its own geography)", self.Cities, want)
	}

	if closureOf(forRows, 999999) != nil {
		t.Errorf("an unknown id must yield no row")
	}
}
