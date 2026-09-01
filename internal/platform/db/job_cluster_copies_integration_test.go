//go:build integration

// Integration test for ListJobCopies: the "openings across cities" list a collapsed job
// exposes — every open posting the anchor's OWNER represents, each with its own location,
// ordered by location.
//
// Membership is the duplicate closure, not a shared role_fingerprint, and it must be the same
// closure the geography union uses. A posting whose city the canon claims in search but whose
// row this list omits is a location a candidate can filter to and then not reach — which is
// half of what issue #2225 reported.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setLocation(t *testing.T, pool *pgxpool.Pool, ext, loc string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE jobs SET location = $1 WHERE external_id = $2", loc, ext); err != nil {
		t.Fatalf("set location %s: %v", ext, err)
	}
}

// copiesOf is the call under test, spelled once so the tests read as assertions.
func copiesOf(t *testing.T, q *Queries, anchorID int64) []ListJobCopiesRow {
	t.Helper()
	rows, err := q.ListJobCopies(context.Background(),
		ListJobCopiesParams{JobID: anchorID, RowLimit: 100, RowOffset: 0})
	if err != nil {
		t.Fatalf("ListJobCopies(%d): %v", anchorID, err)
	}
	return rows
}

func TestListJobCopies_ReturnsTheOwnersOpenClosureByLocation(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	const fp = "role-dup"
	cities := map[string]string{"acme:1": "Moscow", "acme:2": "Kazan", "acme:3": "Perm"}
	for ext, city := range cities {
		mustUpsert(t, q, withFingerprint(ext, "Staff Engineer", fp))
		setLocation(t, pool, ext, city)
	}
	// An unrelated role and a closed member must not appear.
	mustUpsert(t, q, withFingerprint("acme:other", "Designer", "role-xyz"))
	mustUpsert(t, q, withFingerprint("acme:closed", "Staff Engineer", fp))

	anchorID, _ := dupOf(t, pool, "acme:1")
	for _, ext := range []string{"acme:2", "acme:3", "acme:closed"} {
		id, _ := dupOf(t, pool, ext)
		markDuplicate(t, pool, id, anchorID)
	}
	closedID, _ := dupOf(t, pool, "acme:closed")
	closeJob(t, pool, closedID)

	copies := copiesOf(t, q, anchorID)
	// The three open members, ordered by location (Kazan, Moscow, Perm), each with its own
	// location — the anchor itself included, the closed member excluded.
	if len(copies) != 3 {
		t.Fatalf("got %d copies, want 3 (open closure members)", len(copies))
	}
	if copies[0].Total != 3 {
		t.Errorf("total = %d, want 3 (whole open closure, pre-limit)", copies[0].Total)
	}
	for i, want := range []string{"Kazan", "Moscow", "Perm"} {
		if copies[i].Location != want {
			t.Errorf("copies[%d].Location = %q, want %q (ordered by location)", i, copies[i].Location, want)
		}
	}
}

// The case a role-fingerprint grouping could not express, and the one issue #2225 asked for
// by name: "expose all locations through its copies endpoint". A fuzzy-suppressed posting has
// a DIFFERENT fingerprint from its canon by construction — that is why the exact pass left it
// for the fuzzy pass — so grouping by fingerprint listed the canon alone.
func TestListJobCopies_IncludesAFuzzySuppressedPosting(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, withFingerprint("cobs:calgary", "Sales Assistant", "fp-calgary"))
	mustUpsert(t, q, withFingerprint("cobs:chestermere", "Sales Assistant", "fp-chestermere"))
	setLocation(t, pool, "cobs:calgary", "Calgary")
	setLocation(t, pool, "cobs:chestermere", "Chestermere")

	canonID, _ := dupOf(t, pool, "cobs:calgary")
	hiddenID, _ := dupOf(t, pool, "cobs:chestermere")
	markFuzzy(t, q, testCompany, hiddenID, canonID)

	copies := copiesOf(t, q, canonID)
	if len(copies) != 2 {
		t.Fatalf("got %d copies, want 2 — the fuzzy-suppressed posting must be listed", len(copies))
	}
	for i, want := range []string{"Calgary", "Chestermere"} {
		if copies[i].Location != want {
			t.Errorf("copies[%d].Location = %q, want %q", i, copies[i].Location, want)
		}
	}
}

// A candidate can arrive on a SUPPRESSED posting by direct link — it stays readable by slug,
// which is exactly how issue #2225 was reported. Asking for its copies must answer with the
// whole group it belongs to, not with the fragment its own marker points at.
func TestListJobCopies_FromASuppressedPostingResolvesToItsOwner(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	// C --role--> B --fuzzy--> A, all open: a two-hop chain, so resolving only one hop up
	// would answer with B's fragment instead of A's whole group.
	mustUpsert(t, q, withFingerprint("chain:a", "Operations Coordinator", "fp-a"))
	mustUpsert(t, q, withFingerprint("chain:b", "Operations Coordinator", "fp-b"))
	mustUpsert(t, q, withFingerprint("chain:c", "Operations Coordinator", "fp-b"))
	setLocation(t, pool, "chain:a", "Toronto")
	setLocation(t, pool, "chain:b", "Belleville")
	setLocation(t, pool, "chain:c", "Scarborough")

	aID, _ := dupOf(t, pool, "chain:a")
	bID, _ := dupOf(t, pool, "chain:b")
	cID, _ := dupOf(t, pool, "chain:c")
	markDuplicate(t, pool, cID, bID)
	markFuzzy(t, q, testCompany, bID, aID)

	for _, anchor := range []struct {
		name string
		id   int64
	}{{"the owner", aID}, {"a one-hop member", bID}, {"a two-hop member", cID}} {
		t.Run(anchor.name, func(t *testing.T) {
			copies := copiesOf(t, q, anchor.id)
			if len(copies) != 3 {
				t.Fatalf("got %d copies, want the owner's whole closure (3)", len(copies))
			}
			for i, want := range []string{"Belleville", "Scarborough", "Toronto"} {
				if copies[i].Location != want {
					t.Errorf("copies[%d].Location = %q, want %q", i, copies[i].Location, want)
				}
			}
		})
	}
}

// A row that represents nobody answers with itself. The detail page gates its "other
// locations" tab on a total above one, so one row reads as "no other locations" there — the
// same outcome the old fingerprint grouping produced by returning nothing, with a simpler rule.
func TestListJobCopies_ARowRepresentingNobodyReturnsItself(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, withFingerprint("acme:solo", "Untagged", ""))
	setLocation(t, pool, "acme:solo", "Lisbon")

	soloID, _ := dupOf(t, pool, "acme:solo")
	copies := copiesOf(t, q, soloID)
	if len(copies) != 1 || copies[0].Location != "Lisbon" {
		t.Fatalf("copies = %+v, want just the row itself", copies)
	}
	if copies[0].Total != 1 {
		t.Errorf("total = %d, want 1", copies[0].Total)
	}
}

// An anchor inside a marker cycle has no owner to resolve to. Unlike the closure geography
// queries — whose seed is rows that are nobody's duplicate, which makes a cycle unreachable —
// this one is HANDED an arbitrary id, so the depth bound is what has to stop the upward walk.
func TestListJobCopies_AMarkerCycleTerminates(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, withFingerprint("loop:a", "Staff Engineer", "fp-loop-a"))
	mustUpsert(t, q, withFingerprint("loop:b", "Staff Engineer", "fp-loop-b"))
	aID, _ := dupOf(t, pool, "loop:a")
	bID, _ := dupOf(t, pool, "loop:b")
	markFuzzy(t, q, testCompany, aID, bID)
	markFuzzy(t, q, testCompany, bID, aID)

	if copies := copiesOf(t, q, aID); len(copies) != 0 {
		t.Errorf("a cycle resolves to no owner and must list nothing; got %d copies", len(copies))
	}
}

// An offset past the end is an empty page, never an error. The handler clamps into int32 via
// pageParamsBounded (TestOffsetIsParsedOnlyByTheSharedHelper pins that it must), but the query
// has to survive the clamped extreme too — this endpoint is public and unauthenticated, and it
// once answered 500 where every other list endpoint answers with nothing.
func TestListJobCopies_AnOffsetPastTheEndIsAnEmptyPage(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, withFingerprint("acme:1", "Staff Engineer", "role-page"))
	mustUpsert(t, q, withFingerprint("acme:2", "Staff Engineer", "role-page"))
	anchorID, _ := dupOf(t, pool, "acme:1")
	otherID, _ := dupOf(t, pool, "acme:2")
	markDuplicate(t, pool, otherID, anchorID)

	for _, offset := range []int32{2, 1 << 30, 1<<31 - 1} {
		rows, err := q.ListJobCopies(context.Background(),
			ListJobCopiesParams{JobID: anchorID, RowLimit: 100, RowOffset: offset})
		if err != nil {
			t.Fatalf("offset %d must yield an empty page, not an error: %v", offset, err)
		}
		if len(rows) != 0 {
			t.Errorf("offset %d returned %d rows, want none", offset, len(rows))
		}
	}
}

// A private job (jd-tailor-intake: a pasted JD or an unrecognized-URL scrape) must never
// surface in a PUBLIC job's copies list, even when it is inside the same closure — that would
// hand the private job's slug and location/url to anyone browsing an unrelated public posting,
// defeating the point of never listing or indexing it.
func TestListJobCopies_ExcludesPrivateJobs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	const fp = "role-priv"
	mustUpsert(t, q, withFingerprint("acme:pub", "Staff Engineer", fp))
	mustUpsert(t, q, withFingerprint("acme:priv", "Staff Engineer", fp))
	if _, err := pool.Exec(ctx, "UPDATE jobs SET is_private = true WHERE external_id = $1", "acme:priv"); err != nil {
		t.Fatalf("mark private: %v", err)
	}

	anchorID, _ := dupOf(t, pool, "acme:pub")
	privID, _ := dupOf(t, pool, "acme:priv")
	markDuplicate(t, pool, privID, anchorID)

	copies := copiesOf(t, q, anchorID)
	if len(copies) != 1 {
		t.Fatalf("copies = %+v, want only the public job", copies)
	}
	var isPrivate bool
	if err := pool.QueryRow(ctx,
		"SELECT is_private FROM jobs WHERE public_slug = $1", copies[0].PublicSlug).Scan(&isPrivate); err != nil {
		t.Fatalf("read %s: %v", copies[0].PublicSlug, err)
	}
	if isPrivate {
		t.Errorf("copies included a private job (%s)", copies[0].PublicSlug)
	}
}
