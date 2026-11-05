//go:build integration

package notify

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// The worker judges a query, then pauses its subscriptions. An owner who fixed the
// saved search in between must keep the alert on: the pause holds only while the
// stored query is still the one judged.
func TestDeactivateSubscriptionsPausesOnlyTheQueryItJudged(t *testing.T) {
	pool := testdb.Pool(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertNotifyIntegrationUser(t, pool, "pause-judged@example.test")
	searchID := insertNotifySavedSearch(t, pool, userID, "Fixed meanwhile", "seniority=senior")
	subID := insertNotifyPushSubscription(t, pool, userID, searchID)
	ids := []int64{subID}

	n, err := queries.DeactivateSubscriptions(ctx, db.DeactivateSubscriptionsParams{Ids: ids, Query: "q=a+OR+b+OR+c+OR+d+OR+e+OR+f"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("paused %d subscription(s) whose saved search no longer holds the judged query", n)
	}

	n, err = queries.DeactivateSubscriptions(ctx, db.DeactivateSubscriptionsParams{Ids: ids, Query: "seniority=senior"})
	if err != nil {
		t.Fatal(err)
	}
	var active bool
	if err := pool.QueryRow(ctx, "SELECT active FROM subscriptions WHERE id = $1", subID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if n != 1 || active {
		t.Fatalf("paused=%d active=%v, want the subscription on the judged query paused", n, active)
	}
}
