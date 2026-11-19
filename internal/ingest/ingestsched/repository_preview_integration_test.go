//go:build integration

package ingestsched

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"
)

// Shadow mode's whole value is that it measures what apply mode WOULD do. PreviewDue and
// ClaimDueRuns carry the same predicate written twice, because sqlc cannot share one — so
// their equivalence is asserted here rather than trusted to survive the next edit.
func TestPreviewSeesExactlyWhatAClaimWouldTake(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "paylocity", "acme")
	addBoard(t, pool, "greenhouse", "globex")
	manage(t, pool, "paylocity")
	manage(t, pool, "greenhouse")

	wide := Effective("paylocity", &Override{
		Provider: "paylocity", Shards: 5, Cadence: time.Hour, RunTimeout: DefaultRunTimeout, Enabled: true,
	})
	if _, err := repo.Reconcile(ctx, []Settings{wide, Effective("greenhouse", nil)}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// One row already claimed and long dead, so the reclaim arm of the predicate is
	// exercised too, not just the plain due arm.
	if _, err := pool.Exec(ctx,
		`UPDATE ingest_run_state SET claimed_at = now() - interval '3 hours'
		  WHERE provider = 'paylocity' AND shard = 1`); err != nil {
		t.Fatalf("age a claim: %v", err)
	}

	// paylocity (5 shards) is heavy and greenhouse is light, so both pools are exercised —
	// this asserts the equivalence holds across the split, not just within one pool.
	previewHeavy, err := repo.PreviewDue(ctx, true, 10, time.Minute)
	if err != nil {
		t.Fatalf("PreviewDue heavy: %v", err)
	}
	previewLight, err := repo.PreviewDue(ctx, false, 10, time.Minute)
	if err != nil {
		t.Fatalf("PreviewDue light: %v", err)
	}
	preview := slices.Concat(previewHeavy, previewLight)
	if len(preview) == 0 {
		t.Fatal("preview saw nothing; the test premise is broken")
	}

	claimedHeavy, err := repo.Claim(ctx, true, 10, time.Minute)
	if err != nil {
		t.Fatalf("Claim heavy: %v", err)
	}
	claimedLight, err := repo.Claim(ctx, false, 10, time.Minute)
	if err != nil {
		t.Fatalf("Claim light: %v", err)
	}
	claimed := slices.Concat(claimedHeavy, claimedLight)

	if len(preview) != len(claimed) {
		t.Fatalf("preview saw %d runs, claim took %d", len(preview), len(claimed))
	}
	seen := make(map[string]Run, len(preview))
	for _, r := range preview {
		seen[fmt.Sprintf("%s/%d", r.Provider, r.Shard)] = r
	}
	for _, r := range claimed {
		key := fmt.Sprintf("%s/%d", r.Provider, r.Shard)
		want, ok := seen[key]
		if !ok {
			t.Errorf("claim took %s, which the preview did not see", key)
			continue
		}
		if want.Shards != r.Shards || want.RunTimeout != r.RunTimeout {
			t.Errorf("%s: preview %d shards/%v, claim %d shards/%v",
				key, want.Shards, want.RunTimeout, r.Shards, r.RunTimeout)
		}
	}
}

// The rollout gate, end to end through SQL. An unmanaged provider is TRACKED — it has run
// state, it appears in the report, it accrues a stagger — and is never CLAIMED, because its
// static timer still owns it. Getting this backwards in either direction is a live
// incident: tracking only managed providers wipes the fleet's state every minute of the
// cutover, and claiming unmanaged ones double-crawls every provider.
func TestAnUnmanagedProviderIsTrackedButNeverClaimed(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "greenhouse", "acme")
	addBoard(t, pool, "lever", "globex")
	manage(t, pool, "lever") // greenhouse is deliberately left to its static timer

	settings := []Settings{Effective("greenhouse", nil), Effective("lever", nil)}
	if _, err := repo.Reconcile(ctx, settings); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if got := shardsInState(t, pool, "greenhouse"); len(got) != 1 {
		t.Errorf("unmanaged provider has %v run state, want one row — it must still be tracked", got)
	}

	claimed, err := repo.Claim(ctx, false, 10, time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Provider != "lever" {
		t.Fatalf("claimed %v, want only the managed lever", claimed)
	}

	// The preview must agree, or the shadow run would report launches the apply run would
	// never make.
	preview, err := repo.PreviewDue(ctx, false, 10, time.Minute)
	if err != nil {
		t.Fatalf("PreviewDue: %v", err)
	}
	for _, r := range preview {
		if r.Provider == "greenhouse" {
			t.Errorf("preview offered the unmanaged greenhouse: %v", preview)
		}
	}
}

// Shadow mode must leave run state exactly as it found it, or the static timers still
// driving the fleet would be racing a due time nobody told them about.
func TestPreviewMutatesNothing(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "greenhouse", "acme")
	manage(t, pool, "greenhouse")
	if _, err := repo.Reconcile(ctx, []Settings{Effective("greenhouse", nil)}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	before := dueAt(t, pool, "greenhouse", 1)
	if _, err := repo.PreviewDue(ctx, false, 10, time.Minute); err != nil {
		t.Fatalf("PreviewDue: %v", err)
	}

	if after := dueAt(t, pool, "greenhouse", 1); !after.Equal(before) {
		t.Errorf("preview moved next_due_at from %v to %v", before, after)
	}
	lightInFlight, err := repo.InFlightRuns(ctx, false)
	if err != nil {
		t.Fatalf("InFlightRuns light: %v", err)
	}
	heavyInFlight, err := repo.InFlightRuns(ctx, true)
	if err != nil {
		t.Fatalf("InFlightRuns heavy: %v", err)
	}
	if len(lightInFlight)+len(heavyInFlight) != 0 {
		t.Errorf("preview claimed %d runs; want none", len(lightInFlight)+len(heavyInFlight))
	}
}
