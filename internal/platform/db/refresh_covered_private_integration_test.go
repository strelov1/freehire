//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

func TestRefreshCoveredJobsLeavesPrivatePostingsUntouched(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	truncate(t, pool)
	_, err := pool.Exec(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, is_private)
		VALUES ('weblink', 'covered-public', 'https://example.test/public', 'Public', 'covered-public', 'public', false),
		       ('weblink', 'covered-private', 'https://example.test/private', 'Private', 'covered-private', 'private', true);
		UPDATE job_activity SET last_seen_at = '2000-01-01T00:00:00Z'
		WHERE job_id IN (SELECT id FROM jobs WHERE source = 'weblink');
	`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := New(pool).RefreshCoveredJobs(ctx, RefreshCoveredJobsParams{
		Source:      "weblink",
		ExternalIds: []string{"covered-public", "covered-private"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Errorf("refreshed %d postings, want only the public posting", len(rows))
	}
	for _, fixture := range []struct {
		externalID string
		fresh      bool
	}{{"covered-public", true}, {"covered-private", false}} {
		var seen time.Time
		if err := pool.QueryRow(ctx, `SELECT a.last_seen_at
			FROM jobs j JOIN job_activity a ON a.job_id = j.id
			WHERE j.source = 'weblink' AND j.external_id = $1`, fixture.externalID).Scan(&seen); err != nil {
			t.Fatal(err)
		}
		if fixture.fresh && time.Since(seen) > time.Minute {
			t.Error("public posting was not refreshed")
		}
		if !fixture.fresh && !seen.Equal(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
			t.Error("private posting's observation changed")
		}
	}
}
