//go:build integration

package main

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestWhatJobsSummaryDedupLifecycle(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()
	fixture := summaryFixture(t)
	ids := make([]int64, 0, len(fixture.titles))
	for _, row := range fixture.titles {
		var id int64
		err := pool.QueryRow(ctx, `INSERT INTO jobs
   (source, external_id, public_slug, url, title, description, company_slug,
    location, category, seniority, posting_language)
   VALUES ($1, $2, $2, 'https://example.test/job', $3, $4, 'myitjob', $5, $6, $7, $8)
   RETURNING id`, row.Source, row.Title, row.Title, fixture.bodies[row.ID],
			row.Location, row.Category, row.Seniority, row.PostingLanguage).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO jobs
 (source, external_id, public_slug, url, title, description, company_slug, role_fingerprint)
 VALUES ('lever','scope-other-1','scope-other-1','https://example.test/1','Platform Engineer','Build platforms','other-company','scope-other-role'),
 ('lever','scope-other-2','scope-other-2','https://example.test/2','Platform Engineer','Build platforms','other-company','scope-other-role')`); err != nil {
		t.Fatal(err)
	}
	previousFlags, previousArgs := flag.CommandLine, os.Args
	flag.CommandLine = flag.NewFlagSet("reindex", flag.ContinueOnError)
	os.Args = []string{"reindex", "-fuzzy-company=myitjob"}
	defer func() { flag.CommandLine, os.Args = previousFlags, previousArgs }()
	t.Setenv("DATABASE_URL", pool.Config().ConnString())
	t.Setenv("REINDEX_DEDUP_ONLY", "1")
	if code := run(); code != 0 {
		t.Fatalf("scoped reindex exit = %d", code)
	}
	var untouched int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE company_slug='other-company' AND duplicate_of IS NULL").Scan(&untouched); err != nil {
		t.Fatal(err)
	}
	if untouched != 2 {
		t.Fatalf("scoped pass changed another company: %d unmarked rows", untouched)
	}
	for _, wantChanges := range []int64{0} {
		changes, err := collapseFuzzyDuplicatesForCompany(ctx, q, "myitjob")
		if err != nil {
			t.Fatal(err)
		}
		if changes != wantChanges {
			t.Fatalf("changed %d markers, want %d", changes, wantChanges)
		}
	}
	var marker pgtype.Int8
	if err := pool.QueryRow(ctx, "SELECT duplicate_of_fuzzy FROM jobs WHERE id=$1", ids[1]).Scan(&marker); err != nil {
		t.Fatal(err)
	}
	if !marker.Valid || marker.Int64 != ids[0] {
		t.Fatalf("summary marker = %+v, want full posting %d", marker, ids[0])
	}
	var queued bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM search_delete_outbox WHERE job_id=$1)", ids[1]).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if !queued {
		t.Fatal("suppressed summary was not queued for search removal")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM search_outbox WHERE job_id=$1", ids[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at=now() WHERE id=$1", ids[0]); err != nil {
		t.Fatal(err)
	}
	changes, err := collapseFuzzyDuplicatesForCompany(ctx, q, "myitjob")
	if err != nil {
		t.Fatal(err)
	}
	if changes != 1 {
		t.Fatalf("released %d markers, want 1", changes)
	}
	if err := pool.QueryRow(ctx, "SELECT duplicate_of_fuzzy FROM jobs WHERE id=$1", ids[1]).Scan(&marker); err != nil {
		t.Fatal(err)
	}
	if marker.Valid {
		t.Fatalf("summary still hidden behind closed full posting: %+v", marker)
	}
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM search_outbox WHERE job_id=$1)", ids[1]).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if !queued {
		t.Fatal("released summary was not queued for search restoration")
	}
}
