//go:build integration

// Integration test for the per-subscription claim cap: one subscription with a huge
// backlog must not starve the others. The unit tests cover the delivery branches against
// a fake Store; only a real Postgres can prove the CLAIM itself — the LATERAL, the row
// lock and the cap live in SQL. Run with:
// go test -tags=integration ./internal/engage/notify/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package notify

import (
	"context"
	"strconv"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// The claim used to be a flat `ORDER BY subscription_id, matched_at LIMIT batch_size`,
// which reads as "oldest first" but is really "lowest subscription id first". Measured on
// prod 2026-09-04: one subscription whose saved search had an EMPTY query held 248k
// pending matches, the queue was 1.14M deep, and every subscription above it had
// attempts=0 since the day it was created — never delivered, never even attempted.
//
// So the property under test is not "the claim returns rows". It is that a subscription
// with more pending matches than a whole batch takes only its own share.
func TestClaimSpreadsAcrossSubscriptionsRatherThanLowestIDFirst(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "fair-claim@example.test")

	// The hoarder is created FIRST so it holds the lowest subscription id — the exact
	// shape that starved everyone under the old ordering.
	hoarderSearch := insertNotifySavedSearch(t, pool, userID, "Everything", "")
	hoarderID := insertNotifyPushSubscription(t, pool, userID, hoarderSearch)
	const hoarderBacklog = 400
	for i := 0; i < hoarderBacklog; i++ {
		jobID := insertNotifyJob(t, pool, sequentialExternalID("fair-hoard", i),
			"Engineer", sequentialSlug("fair-hoard", i))
		insertNotifyPendingMatch(t, pool, hoarderID, jobID)
	}

	// Two ordinary subscriptions, created after, each with a couple of matches.
	var ordinary []int64
	for s := 0; s < 2; s++ {
		search := insertNotifySavedSearch(t, pool, userID, "Backend "+strconv.Itoa(s), "seniority=senior")
		subID := insertNotifyPushSubscription(t, pool, userID, search)
		ordinary = append(ordinary, subID)
		for i := 0; i < 2; i++ {
			jobID := insertNotifyJob(t, pool, sequentialExternalID("fair-ord", s*10+i),
				"Backend Engineer", sequentialSlug("fair-ord", s*10+i))
			insertNotifyPendingMatch(t, pool, subID, jobID)
		}
	}

	const perSubscription = 50
	claimed, err := queries.ClaimSubscriptionMatches(ctx, db.ClaimSubscriptionMatchesParams{
		LeaseSeconds:    600,
		PerSubscription: perSubscription,
		// Deliberately smaller than the hoarder's backlog: under the old query this
		// bound alone was what the hoarder consumed.
		BatchSize: 200,
	})
	if err != nil {
		t.Fatalf("ClaimSubscriptionMatches: %v", err)
	}

	bySub := map[int64]int{}
	for _, c := range claimed {
		bySub[c.SubscriptionID]++
	}

	if got := bySub[hoarderID]; got != perSubscription {
		t.Errorf("hoarding subscription claimed %d matches, want exactly the %d cap "+
			"(it has %d pending)", got, perSubscription, hoarderBacklog)
	}
	for _, subID := range ordinary {
		if got := bySub[subID]; got != 2 {
			t.Errorf("subscription %d claimed %d matches, want its 2 — a backlog on a "+
				"lower id must not consume the batch", subID, got)
		}
	}
}

// A second pass must move the hoarder forward rather than re-serving the same rows: the
// first pass's claim is leased, and the lease is what keeps two overlapping passes from
// sending one digest twice.
func TestASecondClaimTakesTheNextSliceNotTheSameOne(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "fair-claim-second@example.test")
	search := insertNotifySavedSearch(t, pool, userID, "Everything", "")
	subID := insertNotifyPushSubscription(t, pool, userID, search)
	for i := 0; i < 30; i++ {
		jobID := insertNotifyJob(t, pool, sequentialExternalID("fair-second", i),
			"Engineer", sequentialSlug("fair-second", i))
		insertNotifyPendingMatch(t, pool, subID, jobID)
	}

	arg := db.ClaimSubscriptionMatchesParams{LeaseSeconds: 600, PerSubscription: 10, BatchSize: 200}
	first, err := queries.ClaimSubscriptionMatches(ctx, arg)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	second, err := queries.ClaimSubscriptionMatches(ctx, arg)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(first) != 10 || len(second) != 10 {
		t.Fatalf("claims returned %d and %d, want 10 each", len(first), len(second))
	}

	seen := map[int64]bool{}
	for _, c := range first {
		seen[c.JobID] = true
	}
	for _, c := range second {
		if seen[c.JobID] {
			t.Errorf("job %d was claimed by both passes — the lease must keep a digest "+
				"from being sent twice", c.JobID)
		}
	}
}

// sequentialExternalID and sequentialSlug keep the bulk fixtures above unique.
func sequentialExternalID(prefix string, i int) string { return prefix + "-ext-" + strconv.Itoa(i) }
func sequentialSlug(prefix string, i int) string       { return prefix + "-slug-" + strconv.Itoa(i) }
