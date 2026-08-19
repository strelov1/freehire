//go:build integration

// Integration tests for routing duplicate-status changes onto the two facet-index queues: a
// posting that becomes a duplicate is queued for removal, one that becomes canonical again is
// queued for indexing, and a duplicate that merely changes canon is queued nowhere.
//
// The case worth reading first is TestMarkerOutbox_OnePassReleasesWhileAnotherHolds. An
// implementation that branches on the pass's OWN column passes every other test here and gets
// that one wrong, by putting a posting that is still a duplicate back into search.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// queued reads the job ids sitting on one of the two queues.
func queued(t *testing.T, pool *pgxpool.Pool, table string) map[int64]bool {
	t.Helper()
	out := map[int64]bool{}
	// table is a test-supplied literal, never user input.
	rows, err := pool.Query(context.Background(), "SELECT job_id FROM "+table)
	if err != nil {
		t.Fatalf("read %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan %s: %v", table, err)
		}
		out[id] = true
	}
	return out
}

// clearQueues empties both queues so a test can assert on what ONE pass produced rather than
// on everything the fixture happened to enqueue.
func clearQueues(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE search_outbox, search_delete_outbox`); err != nil {
		t.Fatalf("clear queues: %v", err)
	}
}

// TestMarkerOutbox_BecomingADuplicateQueuesRemoval covers the transition that costs a user
// something: until this existed, a repost stayed searchable as its own vacancy until the next
// rebuild, up to six hours.
func TestMarkerOutbox_BecomingADuplicateQueuesRemoval(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, withRoleFP(atsJob("acme:canon", "Staff Engineer", []string{"US"}), "fp-role"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:repost", "Staff Engineer", []string{"US"}), "fp-role"))
	canonID, _ := dupOf(t, pool, "acme:canon")
	repostID, _ := dupOf(t, pool, "acme:repost")
	clearQueues(t, pool)

	recomputeDuplicates(t, q)

	if _, dup := dupOf(t, pool, "acme:repost"); dup != canonID {
		t.Fatalf("fixture: repost duplicate_of = %d, want canon %d", dup, canonID)
	}
	if !queued(t, pool, "search_delete_outbox")[repostID] {
		t.Error("the new duplicate was not queued for removal; its document would sit in the index until the next rebuild")
	}
	if queued(t, pool, "search_outbox")[repostID] {
		t.Error("the new duplicate was queued for indexing; the claim would skip it anyway, but queueing it is wrong")
	}
	if queued(t, pool, "search_delete_outbox")[canonID] {
		t.Error("the canon was queued for removal")
	}
}

// TestMarkerOutbox_BecomingCanonicalQueuesIndexing is the same transition backwards: a canon
// closes, its repost is promoted, and the promoted row has to come back into search.
func TestMarkerOutbox_BecomingCanonicalQueuesIndexing(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	mustUpsert(t, q, withRoleFP(atsJob("acme:canon", "Staff Engineer", []string{"US"}), "fp-role"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:repost", "Staff Engineer", []string{"US"}), "fp-role"))
	canonID, _ := dupOf(t, pool, "acme:canon")
	repostID, _ := dupOf(t, pool, "acme:repost")

	recomputeDuplicates(t, q)
	if _, dup := dupOf(t, pool, "acme:repost"); dup != canonID {
		t.Fatalf("fixture: repost is not a duplicate")
	}

	// The canon closes; the repost is the only open row of its role and becomes canonical.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, canonID); err != nil {
		t.Fatalf("close canon: %v", err)
	}
	clearQueues(t, pool)

	recomputeDuplicates(t, q)

	if _, dup := dupOf(t, pool, "acme:repost"); dup != -1 {
		t.Fatalf("fixture: repost duplicate_of = %d, want NULL after its canon closed", dup)
	}
	if !queued(t, pool, "search_outbox")[repostID] {
		t.Error("the promoted posting was not queued for indexing; it would stay out of search until the next rebuild")
	}
	if queued(t, pool, "search_delete_outbox")[repostID] {
		t.Error("the promoted posting was queued for removal")
	}

	// The queue orders by job_posted_at, so the enqueue must carry it rather than leave NULL.
	var postedAt *string
	if err := pool.QueryRow(ctx,
		`SELECT job_posted_at::text FROM search_outbox WHERE job_id = $1`, repostID).Scan(&postedAt); err != nil {
		t.Fatalf("read job_posted_at: %v", err)
	}
	if postedAt == nil {
		t.Error("job_posted_at is NULL; the claim orders by it, so a NULL sorts the posting to the back of the queue")
	}
}

// TestMarkerOutbox_RepointingADuplicateQueuesNothing keeps the volume proportional to status
// changes rather than to marker writes. The document is already absent; removing it again is a
// no-op in Meilisearch and pure cost at ~200 documents a push.
func TestMarkerOutbox_RepointingADuplicateQueuesNothing(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	mustUpsert(t, q, withRoleFP(atsJob("acme:first", "Staff Engineer", []string{"US"}), "fp-role"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:second", "Staff Engineer", []string{"US"}), "fp-role"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:third", "Staff Engineer", []string{"US"}), "fp-role"))
	firstID, _ := dupOf(t, pool, "acme:first")
	thirdID, _ := dupOf(t, pool, "acme:third")

	recomputeDuplicates(t, q)
	if _, dup := dupOf(t, pool, "acme:third"); dup != firstID {
		t.Fatalf("fixture: third points at %d, want the min-id canon %d", dup, firstID)
	}

	// Close the canon: third stays a duplicate but now points at second.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, firstID); err != nil {
		t.Fatalf("close canon: %v", err)
	}
	clearQueues(t, pool)

	recomputeDuplicates(t, q)

	if _, dup := dupOf(t, pool, "acme:third"); dup == -1 || dup == firstID {
		t.Fatalf("fixture: third duplicate_of = %d, want a new canon", dup)
	}
	if queued(t, pool, "search_delete_outbox")[thirdID] || queued(t, pool, "search_outbox")[thirdID] {
		t.Error("a duplicate that only changed canon was queued; it was a duplicate before and after, so its document never moved")
	}
}

// TestMarkerOutbox_OnePassReleasesWhileAnotherHolds is why the decision reads the DERIVED
// marker. The aggregator pass releases its suppression while the role pass still marks the
// posting a repost: the aggregator's OWN column goes non-NULL to NULL, which looks exactly like
// "became canonical" — but the posting is still a duplicate, and queueing it for indexing would
// put a repost back into search.
func TestMarkerOutbox_OnePassReleasesWhileAnotherHolds(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Getting a row to carry BOTH markers takes care, and the constraint is instructive: the
	// aggregator pass skips a row the role pass pointed at another AGGREGATOR, so the role
	// canon has to be a non-aggregator — and a DIFFERENT one from the ATS twin the aggregator
	// pass matches by title, or closing the twin would release both markers at once.
	//
	//   acme:role-canon  ATS, "Backend Engineer",  fp-shared  <- role canon, stays open
	//   acme:agg         aggregator, "Platform Engineer", fp-shared  <- carries both markers
	//   acme:ats-twin    ATS, "Platform Engineer", fp-twin    <- the aggregator twin, closes
	mustUpsert(t, q, withRoleFP(atsJob("acme:role-canon", "Backend Engineer", []string{"US"}), "fp-shared"))
	mustUpsert(t, q, withRoleFP(aggJob("acme:agg", "Platform Engineer", []string{"US"}), "fp-shared"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:ats-twin", "Platform Engineer", []string{"US"}), "fp-twin"))
	roleCanonID, _ := dupOf(t, pool, "acme:role-canon")
	atsID, _ := dupOf(t, pool, "acme:ats-twin")
	agg2ID, _ := dupOf(t, pool, "acme:agg")

	recomputeDuplicates(t, q)
	suppressAggregators(t, q)

	agg, role, _, derived := ownedMarkers(t, pool, "acme:agg")
	if agg != atsID || role != roleCanonID {
		t.Fatalf("fixture: agg must carry BOTH markers, got aggregator=%d (want %d) role=%d (want %d)",
			agg, atsID, role, roleCanonID)
	}
	if derived != atsID {
		t.Fatalf("fixture: derived = %d, want the aggregator verdict %d (it outranks role)", derived, atsID)
	}

	// The ATS twin closes, so the aggregator pass releases its suppression. The role pass
	// still marks the aggregator row a repost of acme:role-canon.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, atsID); err != nil {
		t.Fatalf("close ATS twin: %v", err)
	}
	clearQueues(t, pool)

	suppressAggregators(t, q)

	agg, role, _, derived = ownedMarkers(t, pool, "acme:agg")
	if agg != -1 {
		t.Fatalf("fixture: the suppression was not released, aggregator column = %d", agg)
	}
	if role != roleCanonID || derived != roleCanonID {
		t.Fatalf("fixture: the row should still be a duplicate by role, got role=%d derived=%d, want %d",
			role, derived, roleCanonID)
	}
	if queued(t, pool, "search_outbox")[agg2ID] {
		t.Error("a posting that is STILL a duplicate was queued for indexing — the decision read the " +
			"pass's own column instead of the derived one, and this repost would return to search")
	}
	if queued(t, pool, "search_delete_outbox")[agg2ID] {
		t.Error("a posting whose duplicate status did not change was queued for removal")
	}
}

// TestMarkerOutbox_UnchangedRefreshQueuesNothing pairs with the ownership change's idempotence
// test: a refresh that writes no markers must also queue nothing.
func TestMarkerOutbox_UnchangedRefreshQueuesNothing(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	mustUpsert(t, q, withRoleFP(atsJob("acme:canon", "Staff Engineer", []string{"US"}), "fp-role"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:repost", "Staff Engineer", []string{"US"}), "fp-role"))
	mustUpsert(t, q, withRoleFP(atsJob("acme:ats", "Platform Engineer", []string{"US"}), "fp-ats"))
	mustUpsert(t, q, withRoleFP(aggJob("acme:agg", "Platform Engineer", []string{"US"}), "fp-agg"))

	recomputeDuplicates(t, q)
	suppressAggregators(t, q)
	clearQueues(t, pool)

	// Second refresh over an unchanged catalogue.
	recomputeDuplicates(t, q)
	suppressAggregators(t, q)

	if n := len(queued(t, pool, "search_delete_outbox")); n != 0 {
		t.Errorf("an unchanged refresh queued %d removals, want 0", n)
	}
	if n := len(queued(t, pool, "search_outbox")); n != 0 {
		t.Errorf("an unchanged refresh queued %d index pushes, want 0", n)
	}
}
