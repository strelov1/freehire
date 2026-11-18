//go:build integration

package db

import (
	"context"
	"strings"
	"testing"
)

func TestFindLinkedInJobByURLUsesIndexIncludingClosedHistory(t *testing.T) {
	pool, ctx := startPostgres(t), context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO jobs(source,external_id,url,title,public_slug,closed_at,closed_reason)
		SELECT 'linkedin','legacy:'||n,'https://www.linkedin.com/jobs/view/'||n||'/','Engineer','li-index-'||n,now(),'unseen'
		FROM generate_series(1,2000)n`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE jobs; ANALYZE job_activity"); err != nil {
		t.Fatal(err)
	}
	const raw = "https://www.linkedin.com/jobs/view/1234/"
	posting, err := New(pool).FindLinkedInJobByURL(ctx, raw)
	if err != nil || posting.ExternalID != "legacy:1234" || !posting.ClosedAt.Valid {
		t.Fatalf("closed legacy identity: key=%q closed=%t error=%v", posting.ExternalID, posting.ClosedAt.Valid, err)
	}
	rows, err := pool.Query(ctx, "EXPLAIN "+findLinkedInJobByURL, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "jobs_linkedin_normalized_url_idx") {
		t.Fatalf("legacy history lookup has no compatible URL index:\n%s", plan.String())
	}
}
