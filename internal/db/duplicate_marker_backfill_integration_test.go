//go:build integration

// Integration tests for the owned-marker backfill (cmd/backfill-duplicate-marker-owner): seeding
// the per-pass columns from the single duplicate_of that predates them. Provenance is not
// recoverable from a stored marker, so the seed goes by shape, and the test pins which shape lands
// where — including the case the shape rule deliberately gets "wrong" and lets the first refresh
// correct.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// legacyMarker writes duplicate_of the way the pre-0114 world did, with the derivation switched
// off for the duration. Nothing in production does this — the trigger is exactly what stops it —
// but the backfill's whole job is to convert rows that predate the trigger, and this is the only
// honest way to produce one in a database where the trigger already exists.
func legacyMarker(t *testing.T, pool *pgxpool.Pool, ext string, canon int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `ALTER TABLE jobs DISABLE TRIGGER jobs_derive_duplicate_of`); err != nil {
		t.Fatalf("disable derivation: %v", err)
	}
	defer func() {
		if _, err := pool.Exec(ctx, `ALTER TABLE jobs ENABLE TRIGGER jobs_derive_duplicate_of`); err != nil {
			t.Fatalf("re-enable derivation: %v", err)
		}
	}()
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET duplicate_of = $1 WHERE external_id = $2`, canon, ext); err != nil {
		t.Fatalf("legacy marker on %s: %v", ext, err)
	}
}

// seedOwners runs the backfill over the whole id range in one chunk, as the worker does in slices.
func seedOwners(t *testing.T, q *Queries) int64 {
	t.Helper()
	ctx := context.Background()
	bounds, err := q.DuplicateMarkerOwnerBackfillBounds(ctx)
	if err != nil {
		t.Fatalf("bounds: %v", err)
	}
	n, err := q.BackfillDuplicateMarkerOwnerChunk(ctx, BackfillDuplicateMarkerOwnerChunkParams{
		Aggregators: aggregators,
		FromID:      bounds.MinID,
		ToID:        bounds.MaxID + 1,
	})
	if err != nil {
		t.Fatalf("backfill chunk: %v", err)
	}
	return n
}

// TestBackfillSeedsOwnedMarkersByShape pins the seeding rule of Decision 4: an aggregator row
// pointing at a non-aggregator canon is the suppression pass's, everything else is seeded as the
// role pass's.
func TestBackfillSeedsOwnedMarkersByShape(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, atsJob("acme:ats", "Platform Engineer", []string{"US"}))
	mustUpsert(t, q, aggJob("acme:agg", "Platform Engineer", []string{"US"}))
	mustUpsert(t, q, atsJob("acme:role1", "Staff Engineer", []string{"US"}))
	mustUpsert(t, q, atsJob("acme:role2", "Staff Engineer", []string{"US"}))
	mustUpsert(t, q, atsJob("acme:solo", "Solo Engineer", []string{"US"}))

	atsID, _ := dupOf(t, pool, "acme:ats")
	role1ID, _ := dupOf(t, pool, "acme:role1")

	// The pre-migration starting state: duplicate_of set, all three owned columns empty.
	legacyMarker(t, pool, "acme:agg", atsID)
	legacyMarker(t, pool, "acme:role2", role1ID)

	if n := seedOwners(t, q); n != 2 {
		t.Fatalf("backfill seeded %d rows, want 2", n)
	}

	agg, role, fuzzy, derived := ownedMarkers(t, pool, "acme:agg")
	if agg != atsID {
		t.Errorf("aggregator row: duplicate_of_aggregator = %d, want %d", agg, atsID)
	}
	if role != -1 || fuzzy != -1 {
		t.Errorf("aggregator row: role = %d, fuzzy = %d, want both NULL", role, fuzzy)
	}
	if derived != atsID {
		t.Errorf("aggregator row: duplicate_of = %d, want %d — the seed must not change what a "+
			"reader sees", derived, atsID)
	}

	agg, role, fuzzy, derived = ownedMarkers(t, pool, "acme:role2")
	if role != role1ID {
		t.Errorf("role row: duplicate_of_role = %d, want %d", role, role1ID)
	}
	if agg != -1 || fuzzy != -1 {
		t.Errorf("role row: aggregator = %d, fuzzy = %d, want both NULL", agg, fuzzy)
	}
	if derived != role1ID {
		t.Errorf("role row: duplicate_of = %d, want %d", derived, role1ID)
	}

	// An unmarked row is left entirely alone.
	agg, role, fuzzy, derived = ownedMarkers(t, pool, "acme:solo")
	if agg != -1 || role != -1 || fuzzy != -1 || derived != -1 {
		t.Errorf("canonical row seeded: aggregator = %d, role = %d, fuzzy = %d, derived = %d, want all NULL",
			agg, role, fuzzy, derived)
	}
}

// TestBackfillIsIdempotent is what makes the pass safe to stop, resume, and re-run after the
// trigger lands — which is how the reconcile sweep of the migration plan is performed, rather than
// by a separate mode.
func TestBackfillIsIdempotent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, atsJob("acme:canon", "Staff Engineer", []string{"US"}))
	mustUpsert(t, q, atsJob("acme:dup", "Staff Engineer", []string{"US"}))
	canonID, _ := dupOf(t, pool, "acme:canon")
	legacyMarker(t, pool, "acme:dup", canonID)

	if n := seedOwners(t, q); n != 1 {
		t.Fatalf("first run seeded %d rows, want 1", n)
	}
	if n := seedOwners(t, q); n != 0 {
		t.Errorf("second run seeded %d rows, want 0 — the pass is not idempotent", n)
	}
	if _, role, _, derived := ownedMarkers(t, pool, "acme:dup"); role != canonID || derived != canonID {
		t.Errorf("after re-run: role = %d, derived = %d, want both %d", role, derived, canonID)
	}
}
