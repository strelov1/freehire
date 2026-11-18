package ingestsched

import (
	"context"
	"testing"
	"time"
)

// THE bug this whole change fixes: before the split, a burst of long sharded crawls could
// fill the fleet's entire cap, and the short tail — most of the fleet's providers, none of
// them individually expensive — would starve behind it. Here the heavy pool is completely
// full (10 of 10 heavy slots) while the light pool has headroom, and a light provider must
// still be claimed and launched despite the heavy pool having nothing free at all.
func TestHeavyAndLightPoolsAreBudgetedIndependently(t *testing.T) {
	repo := &fakeRepo{
		eligible:     []Settings{managed("paylocity"), managed("greenhouse")},
		inFlightRuns: stillRunning("paylocity", 10), // fills the heavy pool past its own cap
		due:          []Run{{Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}},
	}
	launcher := &fakeLauncher{}
	sched := Scheduler{Repo: repo, Launcher: launcher, Cap: 15, HeavyCap: 10, Grace: time.Minute, Apply: true}

	got, err := sched.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if got.Saturated {
		t.Error("Saturated = true; the light pool still had room, so the tick must not report the fleet as full")
	}
	if repo.heavyLimit != 0 {
		t.Errorf("heavy claim limit = %d, want 0 — the heavy pool is already at its cap", repo.heavyLimit)
	}
	if len(launcher.launched) != 1 || launcher.launched[0].Provider != "greenhouse" {
		t.Fatalf("launched %v, want the one light run despite the heavy pool being full", launcher.launched)
	}
	if got.Heavy.InFlight != 10 || got.Heavy.Cap != 10 {
		t.Errorf("Heavy = %+v, want InFlight 10 / Cap 10", got.Heavy)
	}
	if got.Light.Launched != 1 || got.Light.Cap != 5 {
		t.Errorf("Light = %+v, want Launched 1 / Cap 5 (Cap 15 - HeavyCap 10)", got.Light)
	}
}

// A misconfigured HeavyCap greater than Cap must not let the heavy pool alone claim past
// the fleet's real ceiling — the split exists to PROTECT Cap, not to open a second door
// around it. maxRuns (the sqlc claim LIMIT's own sanity ceiling, 1000) is not a stand-in for
// Cap, so heavyCap itself must be clamped, not just lightCap.
func TestHeavyCapNeverClaimsPastTheFleetCap(t *testing.T) {
	paylocity := managed("paylocity")
	paylocity.Shards = 24 // sharded, so every one of its runs is heavy
	repo := &fakeRepo{
		eligible: []Settings{paylocity},
		due: []Run{
			{Provider: "paylocity", Shard: 1, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 2, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 3, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 4, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 5, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 6, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 7, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 8, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 9, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 10, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 11, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 12, Shards: 24, RunTimeout: DefaultRunTimeout},
		},
	}
	launcher := &fakeLauncher{}
	// HeavyCap (20) exceeds Cap (10) — an operator typo, or a HeavyCap left over from a
	// larger fleet after Cap was lowered.
	sched := Scheduler{Repo: repo, Launcher: launcher, Cap: 10, HeavyCap: 20, Grace: time.Minute, Apply: true}

	got, err := sched.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if repo.heavyLimit > 10 {
		t.Errorf("heavy claim limit = %d, want <= Cap (10) — a misconfigured HeavyCap must not raise real fleet concurrency past Cap", repo.heavyLimit)
	}
	if got.Heavy.Cap > 10 {
		t.Errorf("Heavy.Cap = %d, want <= 10", got.Heavy.Cap)
	}
	// One slot, not zero. A reservation clamped to the whole cap is what starved the light
	// pool on the crawl host for three days; the clamp now stops one short of Cap, so a
	// HeavyCap left over from a larger fleet costs the light tail throughput rather than
	// switching it off. See TestLightPoolKeepsASlotWhenHeavyCapExceedsCap for the
	// regression this protects.
	if got.Light.Cap != 1 {
		t.Errorf("Light.Cap = %d, want 1 (Cap 10 - HeavyCap clamped to Cap-1)", got.Light.Cap)
	}
	if len(launcher.launched) > 10 {
		t.Errorf("launched %d runs, want at most Cap (10)", len(launcher.launched))
	}
}

// The production incident of 2026-09-18, in the numbers it actually had.
//
// The crawl host lowered INGEST_SCHEDULER_CAP to 4. Nothing set HeavyCap — there was no
// environment variable for it — so it resolved to DefaultHeavyCap (5), was clamped to the
// whole cap, and the light pool was budgeted zero slots. The tick then reported
// saturated=false (one empty pool is not fleet saturation) and launched=0 (nothing could
// be claimed), which is indistinguishable from a quiet fleet with nothing due. 78
// providers — every unsharded one handed to the scheduler — stopped crawling for three
// days behind a green unit.
//
// Asserting on Light.Cap alone would pass against a clamp that merely floors the number
// somewhere; the run must actually be LAUNCHED, which is the thing the fleet lost.
func TestLightPoolKeepsASlotWhenHeavyCapExceedsCap(t *testing.T) {
	repo := &fakeRepo{
		eligible: []Settings{managed("geekjob")},
		due:      []Run{{Provider: "geekjob", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}},
	}
	launcher := &fakeLauncher{}
	// HeavyCap 0 means "use DefaultHeavyCap", which is larger than this Cap — exactly the
	// host's configuration on the day.
	sched := Scheduler{Repo: repo, Launcher: launcher, Cap: 4, Grace: time.Minute, Apply: true}

	got, err := sched.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if got.Light.Cap < 1 {
		t.Errorf("Light.Cap = %d, want at least 1: a heavy reservation larger than Cap must not switch the light pool off", got.Light.Cap)
	}
	if got.Heavy.Cap+got.Light.Cap > 4 {
		t.Errorf("Heavy.Cap (%d) + Light.Cap (%d) exceeds Cap (4); the reservation is carved out of the cap, never added beside it",
			got.Heavy.Cap, got.Light.Cap)
	}
	if len(launcher.launched) != 1 {
		t.Errorf("launched %d runs, want the one due light run", len(launcher.launched))
	}
}
