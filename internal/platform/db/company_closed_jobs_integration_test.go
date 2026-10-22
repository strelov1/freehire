//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

// TestListClosedJobsByCompany pins the closed-postings list: only this company's CLOSED,
// canonical, public rows, newest posting first — the same exclusions the open list makes,
// with the closed_at predicate inverted.
func TestListClosedJobsByCompany(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	now := time.Now().UTC()
	seed := func(ext, company string, posted time.Time, closed, private bool) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, posted_at, closed_at, is_private)
			 VALUES ('test', $1, 'https://e.test/'||$1, 'Engineer', $1, $2, $3,
			         CASE WHEN $4::bool THEN now() END, $5)
			 RETURNING id`,
			ext, company, posted, closed, private).Scan(&id); err != nil {
			t.Fatalf("seed %s: %v", ext, err)
		}
		return id
	}

	older := seed("closed-old", "acme", now.Add(-72*time.Hour), true, false)
	newer := seed("closed-new", "acme", now.Add(-24*time.Hour), true, false)
	seed("open", "acme", now, false, false)
	seed("closed-private", "acme", now, true, true)
	seed("closed-other", "globex", now, true, false)
	dup := seed("closed-dup", "acme", now, true, false)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2`, newer, dup); err != nil {
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
	if len(got) != 2 || got[0] != newer || got[1] != older {
		t.Fatalf("ids = %v, want [%d %d] (closed, canonical, public, newest posting first)", got, newer, older)
	}
}
