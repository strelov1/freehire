//go:build integration

// The synchronous fuzzy check at insert (jobdedup.CanonicalForFuzzy) and the batch pass
// (collapseFuzzyDuplicatesForCompany) must agree on the canon, or each reindex would undo the
// other. Run with: go test -tags=integration ./cmd/reindex/
package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobdedup"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func syncPosting(t *testing.T, externalID, description string) db.UpsertJobParams {
	t.Helper()
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source: "lever", ExternalID: externalID, Title: "Platform Engineer",
			Company: "Sync Corp", Location: "Berlin", Description: description,
		},
		URL: "https://jobs.example.com/" + externalID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return j.Fields().UpsertParams()
}

func TestFuzzySync_AgreesWithTheBatchPass(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	var ids []int64
	var params []db.UpsertJobParams
	for i, body := range []string{
		bodyA,
		bodyA + " Salary in Poland is quoted gross per month.",
		bodyA + " Salary in Germany is quoted gross per year.",
	} {
		p := syncPosting(t, string(rune('a'+i)), body)
		row, err := q.UpsertJob(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, row.ID)
		params = append(params, p)
	}

	// The oldest row has no older sibling: nothing to join.
	if _, ok := jobdedup.CanonicalForFuzzy(ctx, q, params[0], ids[0]); ok {
		t.Error("the oldest posting must not be marked")
	}
	// Later rows move onto the lowest id, as the batch picks.
	for i := 1; i < len(ids); i++ {
		canon, ok := jobdedup.CanonicalForFuzzy(ctx, q, params[i], ids[i])
		if !ok || canon != ids[0] {
			t.Fatalf("row %d: sync canon = %d (ok=%v), want %d", i, canon, ok, ids[0])
		}
		if _, err := q.MarkJobDuplicateOfFuzzy(ctx, db.MarkJobDuplicateOfFuzzyParams{
			ID: ids[i], DuplicateOfFuzzy: pgtype.Int8{Int64: canon, Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := collapseFuzzyDuplicatesForCompany(ctx, q, "sync-corp"); err != nil {
		t.Fatalf("batch pass: %v", err)
	}
	for i := 1; i < len(ids); i++ {
		var got int64
		if err := pool.QueryRow(ctx, `SELECT duplicate_of_fuzzy FROM jobs WHERE id = $1`, ids[i]).Scan(&got); err != nil || got != ids[0] {
			t.Errorf("after the batch run row %d: duplicate_of_fuzzy = %d (err %v), want unchanged %d", i, got, err, ids[0])
		}
	}
	var canonMarked bool
	if err := pool.QueryRow(ctx, `SELECT duplicate_of_fuzzy IS NOT NULL FROM jobs WHERE id = $1`, ids[0]).Scan(&canonMarked); err != nil || canonMarked {
		t.Errorf("the canon must stay unmarked (marked=%v, err=%v)", canonMarked, err)
	}
}

// A row asked about while a NEWER similar row exists must not move onto it: the batch picks the
// lowest id, so that marker would be inverted on the next run.
func TestFuzzySync_RefusesANewerCanon(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	older := syncPosting(t, "old", bodyA)
	newer := syncPosting(t, "new", bodyA+" Salary in Poland is quoted gross per month.")
	oldRow, err := q.UpsertJob(ctx, older)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpsertJob(ctx, newer); err != nil {
		t.Fatal(err)
	}
	if canon, ok := jobdedup.CanonicalForFuzzy(ctx, q, older, oldRow.ID); ok {
		t.Errorf("older row got canon %d from a newer sibling, want none", canon)
	}
}
