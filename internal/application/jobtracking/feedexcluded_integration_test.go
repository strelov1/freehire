//go:build integration

// Integration test for FeedExcludedJobIDs over the real user_jobs and applications rows.
// Run with: go test -tags=integration ./internal/application/jobtracking/ -run FeedExcluded
package jobtracking_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestFeedExcludedJobIDs(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	repo := jobtracking.NewQueriesRepository(db.New(pool), pool)

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('f@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	job := func(n string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug)
			VALUES ('t', $1, '', 'T', 'Acme', 'acme', $1) RETURNING id`, n).Scan(&id); err != nil {
			t.Fatalf("seed job: %v", err)
		}
		return id
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	before := time.Now().Truncate(time.Second)
	ago := func(d time.Duration) time.Time { return before.Add(-d) }

	saved, dismissed, applied := job("saved"), job("dismissed"), job("applied")
	late, viewed := job("late"), job("viewed")
	exec(`INSERT INTO user_jobs (user_id, job_id, saved_at) VALUES ($1, $2, $3)`, userID, saved, ago(3*time.Hour))
	exec(`INSERT INTO user_jobs (user_id, job_id, dismissed_at) VALUES ($1, $2, $3)`, userID, dismissed, ago(2*time.Hour))
	exec(`INSERT INTO applications (user_id, job_id, applied_at, created_at) VALUES ($1, $2, $3, $3)`, userID, applied, ago(time.Hour))
	exec(`INSERT INTO user_jobs (user_id, job_id, saved_at) VALUES ($1, $2, $3)`, userID, late, before.Add(time.Minute))
	exec(`INSERT INTO user_jobs (user_id, job_id) VALUES ($1, $2)`, userID, viewed)
	exec(`INSERT INTO applications (user_id, job_id, applied_at, created_at) VALUES ($1, NULL, $2, $2)`, userID, ago(time.Minute))

	got, err := repo.FeedExcludedJobIDs(ctx, userID, before, 100)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{applied, dismissed, saved}; !slices.Equal(got, want) {
		t.Fatalf("ids = %v, want %v (newest action first; late save, viewed row and NULL job_id left out)", got, want)
	}
	if got, err = repo.FeedExcludedJobIDs(ctx, userID, before, 2); err != nil || !slices.Equal(got, []int64{applied, dismissed}) {
		t.Fatalf("capped ids = %v, %v; want the two newest", got, err)
	}
}
