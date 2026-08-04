//go:build integration

// Integration test for ListRoleClusterCopies: the "openings across cities" list a
// collapsed job exposes — every open posting sharing the anchor's role cluster
// (company_slug + role_fingerprint), each with its own location, ordered by location.
// Run with: go test -tags=integration ./internal/db/
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

func TestListRoleClusterCopies_ReturnsOpenClusterByLocation(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	const fp = "role-dup"
	cities := map[string]string{"acme:1": "Moscow", "acme:2": "Kazan", "acme:3": "Perm"}
	for ext, city := range cities {
		if _, err := q.UpsertJob(ctx, withFingerprint(ext, "Staff Engineer", fp)); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
		setLocation(t, pool, ext, city)
	}
	// An unrelated role and a closed cluster member must not appear.
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:other", "Designer", "role-xyz")); err != nil {
		t.Fatalf("upsert other: %v", err)
	}
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:closed", "Staff Engineer", fp)); err != nil {
		t.Fatalf("upsert closed: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE external_id = $1", "acme:closed"); err != nil {
		t.Fatalf("close: %v", err)
	}

	anchorID, _ := dupOf(t, pool, "acme:1")
	copies, err := q.ListRoleClusterCopies(ctx, ListRoleClusterCopiesParams{JobID: anchorID, RowLimit: 100, RowOffset: 0})
	if err != nil {
		t.Fatalf("ListRoleClusterCopies: %v", err)
	}

	// The three open cluster members, ordered by location (Kazan, Moscow, Perm), each
	// with its own location — the anchor itself included, the closed member excluded.
	if len(copies) != 3 {
		t.Fatalf("got %d copies, want 3 (open cluster members)", len(copies))
	}
	if copies[0].Total != 3 {
		t.Errorf("total = %d, want 3 (whole open cluster, pre-limit)", copies[0].Total)
	}
	wantOrder := []string{"Kazan", "Moscow", "Perm"}
	for i, want := range wantOrder {
		if copies[i].Location != want {
			t.Errorf("copies[%d].Location = %q, want %q (ordered by location)", i, copies[i].Location, want)
		}
	}

	// An empty-fingerprint anchor clusters with no one → no copies.
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:nofp", "Untagged", "")); err != nil {
		t.Fatalf("upsert nofp: %v", err)
	}
	nofpID, _ := dupOf(t, pool, "acme:nofp")
	empty, err := q.ListRoleClusterCopies(ctx, ListRoleClusterCopiesParams{JobID: nofpID, RowLimit: 100, RowOffset: 0})
	if err != nil {
		t.Fatalf("ListRoleClusterCopies (nofp): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("empty-fp anchor returned %d copies, want 0", len(empty))
	}
}

// A private job (jd-tailor-intake: a pasted JD or an unrecognized-URL scrape) must never
// surface in a PUBLIC job's copies list, even when it coincidentally shares a role cluster
// (company_slug + role_fingerprint) with one — that would hand the private job's slug and
// location/url to anyone browsing an unrelated public posting, defeating the point of never
// listing or indexing it.
func TestListRoleClusterCopies_ExcludesPrivateJobs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	const fp = "role-priv"
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:pub", "Staff Engineer", fp)); err != nil {
		t.Fatalf("upsert public: %v", err)
	}
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:priv", "Staff Engineer", fp)); err != nil {
		t.Fatalf("upsert private: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET is_private = true WHERE external_id = $1", "acme:priv"); err != nil {
		t.Fatalf("mark private: %v", err)
	}

	anchorID, _ := dupOf(t, pool, "acme:pub")
	copies, err := q.ListRoleClusterCopies(ctx, ListRoleClusterCopiesParams{JobID: anchorID, RowLimit: 100, RowOffset: 0})
	if err != nil {
		t.Fatalf("ListRoleClusterCopies: %v", err)
	}
	if len(copies) != 1 || copies[0].PublicSlug == "" {
		t.Fatalf("copies = %+v, want only the public job", copies)
	}
	for _, c := range copies {
		if c.PublicSlug != "" {
			var isPrivate bool
			if err := pool.QueryRow(ctx, "SELECT is_private FROM jobs WHERE public_slug = $1", c.PublicSlug).Scan(&isPrivate); err != nil {
				t.Fatalf("read %s: %v", c.PublicSlug, err)
			}
			if isPrivate {
				t.Errorf("copies included a private job (%s)", c.PublicSlug)
			}
		}
	}
}
