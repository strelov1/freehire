//go:build integration

// Integration test for the write-path fuzzy dedup: a repost whose description differs from its
// sibling's by a localized block (so the exact role fingerprint misses it) is marked at insert,
// and is queued for neither search nor enrichment. The agreement with the batch pass is pinned in
// cmd/reindex. Run with: go test -tags=integration ./cmd/ingest/
package main

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

const fuzzyBody = "Build and operate the payments platform with Golang, TypeScript and Postgres. " +
	"Design resilient APIs, review pull requests, instrument services with metrics and traces, " +
	"run blameless incident retrospectives, mentor junior engineers, shape the quarterly roadmap, " +
	"migrate legacy batch pipelines towards streaming, tighten database indexes and query plans, " +
	"automate deployments through continuous delivery, write architectural decision records, " +
	"improve developer tooling, reduce build times, keep documentation accurate as the platform evolves. " +
	"Our workloads run containerised across several availability zones with asynchronous messaging, " +
	"idempotent consumers and careful backpressure handling whenever traffic surges unexpectedly. " +
	"We practise trunk based development, keep feature flags short lived, prefer boring technology " +
	"where reliability matters more than novelty, and rotate engineers through customer support weeks " +
	"so operational pain reaches whoever can remove it. Compensation reviews happen twice yearly, " +
	"equipment budgets renew annually, conference attendance receives support, remote colleagues gather " +
	"quarterly for planning workshops, and internal mobility between squads remains actively encouraged."

func fuzzyPosting(t *testing.T, externalID, title, description string) job.Job {
	t.Helper()
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source: "lever", ExternalID: externalID, Title: title,
			Company: "Fuzzy Corp", Location: "Berlin", Description: description,
		},
		URL: "https://jobs.example.com/" + externalID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestSave_MarksFuzzyDuplicateAtInsert(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	store := newDBStore(pool, 1, nil, nil, defaultSeenPolicy())

	save := func(id, title, body string) db.Job {
		t.Helper()
		if err := store.Save(ctx, fuzzyPosting(t, id, title, body)); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
		row, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "lever", ExternalID: id})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}

	canon := save("a", "Platform Engineer", fuzzyBody)
	dup := save("b", "Platform Engineer, Warsaw", fuzzyBody+" Salary in Poland is quoted gross per month.")
	other := save("c", "Platform Engineer", "Advise retail clients on supply chain strategy, run stakeholder workshops, prepare board level presentations, build financial models, benchmark competitors, lead procurement negotiations.")

	if canon.DuplicateOf.Valid {
		t.Errorf("the older posting must stay canonical, got duplicate_of = %d", canon.DuplicateOf.Int64)
	}
	if !dup.DuplicateOfFuzzy.Valid || dup.DuplicateOfFuzzy.Int64 != canon.ID || dup.DuplicateOf.Int64 != canon.ID {
		t.Errorf("near-identical repost: duplicate_of_fuzzy = %v, duplicate_of = %v, want canon %d", dup.DuplicateOfFuzzy, dup.DuplicateOf, canon.ID)
	}
	if other.DuplicateOf.Valid {
		t.Errorf("a different role under the same title must not be marked, got %d", other.DuplicateOf.Int64)
	}
	if got := searchOutboxCount(t, pool, dup.ID); got != 0 {
		t.Errorf("search_outbox entries for the fuzzy duplicate = %d, want 0", got)
	}
	if enrichmentQueued(t, pool, dup.ID) {
		t.Error("a fuzzy duplicate must not be queued for enrichment")
	}
	if got := searchOutboxCount(t, pool, canon.ID); got != 1 {
		t.Errorf("search_outbox entries for the canon = %d, want 1", got)
	}
}

// A lookup that fails in SQL aborts the transaction; the savepoint must revive it so the posting
// still commits, unmarked, instead of the whole save failing.
func TestSave_FuzzyLookupSQLFailureStillWritesRow(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	store := newDBStore(pool, 1, nil, nil, defaultSeenPolicy())

	orig := fuzzyCanonicalFn
	t.Cleanup(func() { fuzzyCanonicalFn = orig })
	fuzzyCanonicalFn = func(ctx context.Context, q *db.Queries, p db.UpsertJobParams, id int64) (int64, bool, error) {
		// A negative LIMIT is a real server-side error, which aborts the transaction.
		_, err := q.FuzzyDedupCandidateTitlesForCompanyCapped(ctx, db.FuzzyDedupCandidateTitlesForCompanyCappedParams{Company: p.CompanySlug, MaxRows: -1})
		if err == nil {
			t.Error("expected the forced lookup to fail")
		}
		return 0, false, errors.Join(errors.New("forced"), err)
	}

	if err := store.Save(ctx, fuzzyPosting(t, "x", "Platform Engineer", fuzzyBody)); err != nil {
		t.Fatalf("save must survive a failed fuzzy lookup: %v", err)
	}
	row, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "lever", ExternalID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if row.DuplicateOf.Valid {
		t.Errorf("row must be written unmarked, got duplicate_of = %d", row.DuplicateOf.Int64)
	}
}
