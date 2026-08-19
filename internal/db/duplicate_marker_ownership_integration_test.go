//go:build integration

// Integration tests for duplicate-marker ownership: each dedup pass writes its own column
// (duplicate_of_aggregator / duplicate_of_role / duplicate_of_fuzzy) and jobs.duplicate_of is
// derived from the three by a trigger, so no pass can clear a marker it did not set. The
// derivation, its precedence, and its refusal to honour a direct write are database behaviour
// verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// setOwned writes one owned marker column directly, the way its pass will. canon is the id to
// point at; pass 0 to clear the column.
func setOwned(t *testing.T, pool *pgxpool.Pool, ext, column string, canon int64) {
	t.Helper()
	var arg any
	if canon != 0 {
		arg = canon
	}
	// The column name is a test-supplied literal, never user input.
	sql := "UPDATE jobs SET " + column + " = $1 WHERE external_id = $2"
	if _, err := pool.Exec(context.Background(), sql, arg, ext); err != nil {
		t.Fatalf("set %s on %s: %v", column, ext, err)
	}
}

// ownedMarkers reads the three owned columns and the derived duplicate_of. Each is -1 when NULL.
func ownedMarkers(t *testing.T, pool *pgxpool.Pool, ext string) (agg, role, fuzzy, derived int64) {
	t.Helper()
	var a, r, f, d *int64
	if err := pool.QueryRow(context.Background(),
		`SELECT duplicate_of_aggregator, duplicate_of_role, duplicate_of_fuzzy, duplicate_of
		 FROM jobs WHERE external_id = $1`, ext).Scan(&a, &r, &f, &d); err != nil {
		t.Fatalf("read markers for %s: %v", ext, err)
	}
	deref := func(p *int64) int64 {
		if p == nil {
			return -1
		}
		return *p
	}
	return deref(a), deref(r), deref(f), deref(d)
}

// TestDuplicateMarkerDerivation_ResolvesInPrecedenceOrder pins the COALESCE order that
// Decision 2 of the change settled on: aggregator, then role, then fuzzy. That order is not
// aesthetic — it reproduces which pass wins a contested row today, measured at 8,279 rows on
// prod. A role-first order would silently repoint every one of them.
func TestDuplicateMarkerDerivation_ResolvesInPrecedenceOrder(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	for _, ext := range []string{"acme:ats", "acme:older", "acme:near", "acme:copy"} {
		if _, err := q.UpsertJob(ctx, withFingerprint(ext, "Staff Engineer", "")); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}
	atsID, _ := dupOf(t, pool, "acme:ats")
	olderID, _ := dupOf(t, pool, "acme:older")
	nearID, _ := dupOf(t, pool, "acme:near")

	// Fuzzy alone.
	setOwned(t, pool, "acme:copy", "duplicate_of_fuzzy", nearID)
	if _, _, _, derived := ownedMarkers(t, pool, "acme:copy"); derived != nearID {
		t.Errorf("fuzzy alone: duplicate_of = %d, want %d", derived, nearID)
	}

	// Role outranks fuzzy.
	setOwned(t, pool, "acme:copy", "duplicate_of_role", olderID)
	if _, _, _, derived := ownedMarkers(t, pool, "acme:copy"); derived != olderID {
		t.Errorf("role over fuzzy: duplicate_of = %d, want %d", derived, olderID)
	}

	// Aggregator outranks role — the contested case.
	setOwned(t, pool, "acme:copy", "duplicate_of_aggregator", atsID)
	if _, _, _, derived := ownedMarkers(t, pool, "acme:copy"); derived != atsID {
		t.Errorf("aggregator over role: duplicate_of = %d, want %d", derived, atsID)
	}

	// Releasing the aggregator marker falls back to role, not to canonical: the whole point
	// of ownership is that one pass releasing its verdict does not erase another's.
	setOwned(t, pool, "acme:copy", "duplicate_of_aggregator", 0)
	agg, role, fuzzy, derived := ownedMarkers(t, pool, "acme:copy")
	if agg != -1 {
		t.Errorf("aggregator column = %d, want NULL", agg)
	}
	if role != olderID || fuzzy != nearID {
		t.Errorf("owned columns disturbed: role = %d (want %d), fuzzy = %d (want %d)", role, olderID, fuzzy, nearID)
	}
	if derived != olderID {
		t.Errorf("after release: duplicate_of = %d, want fallback to role %d", derived, olderID)
	}

	// Clearing every owned column makes the row canonical again.
	setOwned(t, pool, "acme:copy", "duplicate_of_role", 0)
	setOwned(t, pool, "acme:copy", "duplicate_of_fuzzy", 0)
	if _, _, _, derived := ownedMarkers(t, pool, "acme:copy"); derived != -1 {
		t.Errorf("all columns cleared: duplicate_of = %d, want NULL", derived)
	}
}

// TestDuplicateMarkerDerivation_IgnoresDirectWrite is the guarantee that makes ownership
// enforceable rather than a convention: a writer that sets duplicate_of itself does not get to
// put the row into a state the passes disagree with.
func TestDuplicateMarkerDerivation_IgnoresDirectWrite(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	for _, ext := range []string{"acme:canon", "acme:other", "acme:dup"} {
		if _, err := q.UpsertJob(ctx, withFingerprint(ext, "Staff Engineer", "")); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}
	canonID, _ := dupOf(t, pool, "acme:canon")
	otherID, _ := dupOf(t, pool, "acme:other")

	setOwned(t, pool, "acme:dup", "duplicate_of_role", canonID)

	// A direct write pointing somewhere else does not survive.
	if _, err := pool.Exec(ctx,
		"UPDATE jobs SET duplicate_of = $1 WHERE external_id = 'acme:dup'", otherID); err != nil {
		t.Fatalf("direct write: %v", err)
	}
	if _, _, _, derived := ownedMarkers(t, pool, "acme:dup"); derived != canonID {
		t.Errorf("after direct write: duplicate_of = %d, want the owned value %d", derived, canonID)
	}

	// Nor does a direct write trying to make a marked row canonical.
	if _, err := pool.Exec(ctx,
		"UPDATE jobs SET duplicate_of = NULL WHERE external_id = 'acme:dup'"); err != nil {
		t.Fatalf("direct clear: %v", err)
	}
	if _, _, _, derived := ownedMarkers(t, pool, "acme:dup"); derived != canonID {
		t.Errorf("after direct clear: duplicate_of = %d, want the owned value %d", derived, canonID)
	}
}

// TestDuplicateMarkerDerivation_LeavesUnrelatedUpdatesAlone guards the ingest path: jobs is the
// hottest table in the schema and most updates touch no marker at all. The derivation must not
// rewrite duplicate_of, or bump anything, when no owned column moved.
func TestDuplicateMarkerDerivation_LeavesUnrelatedUpdatesAlone(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	for _, ext := range []string{"acme:canon", "acme:dup"} {
		if _, err := q.UpsertJob(ctx, withFingerprint(ext, "Staff Engineer", "")); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}
	canonID, _ := dupOf(t, pool, "acme:canon")
	setOwned(t, pool, "acme:dup", "duplicate_of_role", canonID)

	if _, err := pool.Exec(ctx,
		"UPDATE jobs SET title = 'Staff Engineer II' WHERE external_id = 'acme:dup'"); err != nil {
		t.Fatalf("unrelated update: %v", err)
	}
	if _, _, _, derived := ownedMarkers(t, pool, "acme:dup"); derived != canonID {
		t.Errorf("after unrelated update: duplicate_of = %d, want %d", derived, canonID)
	}

	// A fresh insert with no owned marker is canonical, not accidentally pointed anywhere.
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:fresh", "Solo", "")); err != nil {
		t.Fatalf("upsert fresh: %v", err)
	}
	if _, _, _, derived := ownedMarkers(t, pool, "acme:fresh"); derived != -1 {
		t.Errorf("fresh insert: duplicate_of = %d, want NULL", derived)
	}
}
