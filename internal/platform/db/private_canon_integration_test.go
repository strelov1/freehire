//go:build integration

package db

import (
	"context"
	"testing"
)

// A private posting (a pasted or scraped JD, visible only to its creator) can predate the public
// posting of the same role: the employer's board enters the catalogue after a user already pasted
// one of its JDs. The batch recompute must not then elect the private row canon, or the public
// posting is marked a duplicate and leaves search for everyone. CanonicalJobForRole, the write-time
// half of the same decision, already excludes private rows for exactly this reason.
func TestRecomputeRoleDuplicates_NeverElectsAPrivateCanon(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	const fp = "role-private-first"
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:pasted", "Staff Engineer", fp)); err != nil {
		t.Fatalf("upsert private: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET is_private = true WHERE external_id = $1", "acme:pasted"); err != nil {
		t.Fatalf("mark private: %v", err)
	}
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:board", "Staff Engineer", fp)); err != nil {
		t.Fatalf("upsert public: %v", err)
	}

	recomputeDuplicates(t, q)

	if _, dup := dupOf(t, pool, "acme:board"); dup != -1 {
		t.Errorf("public posting duplicate_of = %d, want NULL: a private row was elected canon", dup)
	}
}
