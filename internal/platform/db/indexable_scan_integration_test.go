//go:build integration

// Integration test for the full rebuild's narrowed keyset scan (ListIndexableJobsByIDAfter
// and its id-only degrade projection). Which rows the predicate admits, and that both
// projections page the same window, are SQL properties verifiable only against Postgres.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"slices"
	"testing"
)

func TestListIndexableJobs_ReadsOnlyOpenCanonicalPublicRows(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	for _, ext := range []string{"open-1", "closed", "dup", "private", "open-2"} {
		mustUpsert(t, q, ingestParams(ext, "Backend Engineer "+ext))
	}
	ids := map[string]int64{}
	rows, err := pool.Query(ctx, `SELECT external_id, id FROM jobs`)
	if err != nil {
		t.Fatalf("read ids: %v", err)
	}
	for rows.Next() {
		var ext string
		var id int64
		if err := rows.Scan(&ext, &id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		ids[ext] = id
	}
	rows.Close()

	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE external_id = 'closed'`); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET is_private = true WHERE external_id = 'private'`); err != nil {
		t.Fatalf("make private: %v", err)
	}
	markDuplicate(t, pool, ids["dup"], ids["open-1"])

	want := []int64{ids["open-1"], ids["open-2"]}
	slices.Sort(want)

	// A batch of one forces the keyset to advance across every excluded row in between.
	var got, gotIDs []int64
	for after := int64(0); ; {
		page, err := q.ListIndexableJobsByIDAfter(ctx, ListIndexableJobsByIDAfterParams{AfterID: after, BatchSize: 1})
		if err != nil {
			t.Fatalf("wide page: %v", err)
		}
		idPage, err := q.ListIndexableJobIDsAfter(ctx, ListIndexableJobIDsAfterParams{AfterID: after, BatchSize: 1})
		if err != nil {
			t.Fatalf("id page: %v", err)
		}
		if len(page) != len(idPage) {
			t.Fatalf("after %d: wide page has %d rows, id page %d — the two predicates disagree", after, len(page), len(idPage))
		}
		if len(page) == 0 {
			break
		}
		got = append(got, page[0].ID)
		gotIDs = append(gotIDs, idPage[0])
		after = page[0].ID
	}

	if !slices.Equal(got, want) {
		t.Errorf("wide scan = %v, want only the open, canonical, public rows %v", got, want)
	}
	if !slices.Equal(gotIDs, want) {
		t.Errorf("id scan = %v, want %v", gotIDs, want)
	}
}
