//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

// TestListClosedJobsByCompany pins the closed-postings list: only this company's CLOSED,
// canonical, public rows, most recently CLOSED first — the same exclusions the open list
// makes, with the closed_at predicate inverted.
//
// The two listed rows are seeded with posted_at and closed_at in OPPOSITE orders, so the
// assertion fails if the query ever falls back to ordering by posted_at.
func TestListClosedJobsByCompany(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	now := time.Now().UTC()
	seed := func(ext, company string, posted time.Time, closed *time.Time, private bool) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, posted_at, closed_at, is_private)
			 VALUES ('test', $1, 'https://e.test/'||$1, 'Engineer', $1, $2, $3, $4, $5)
			 RETURNING id`,
			ext, company, posted, closed, private).Scan(&id); err != nil {
			t.Fatalf("seed %s: %v", ext, err)
		}
		return id
	}
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }

	// Posted first, closed last: must lead.
	closedRecently := seed("closed-recently", "acme", now.Add(-90*24*time.Hour), at(-1*time.Hour), false)
	// Posted last, closed long ago: must trail.
	closedLongAgo := seed("closed-long-ago", "acme", now.Add(-24*time.Hour), at(-12*time.Hour), false)
	seed("open", "acme", now, nil, false)
	seed("closed-private", "acme", now, at(0), true)
	seed("closed-other", "globex", now, at(0), false)
	dup := seed("closed-dup", "acme", now, at(0), false)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2`, closedRecently, dup); err != nil {
		t.Fatalf("mark duplicate: %v", err)
	}

	rows, err := q.ListClosedJobsByCompany(ctx, ListClosedJobsByCompanyParams{CompanySlug: "acme", Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var got []int64
	for _, r := range rows {
		got = append(got, r.ID)
	}
	if len(got) != 2 || got[0] != closedRecently || got[1] != closedLongAgo {
		t.Fatalf("ids = %v, want [%d %d] (closed, canonical, public, most recently closed first)",
			got, closedRecently, closedLongAgo)
	}
}
