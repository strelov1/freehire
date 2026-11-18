package ingestsched

import (
	"context"
	"fmt"
	"testing"
)

// THE bug this reaper exists for. A launched run's transient unit finishes on its own and
// tells nobody: cmd/ingest knows nothing about the scheduler. Without a reap, claimed_at is
// set at claim and cleared by nothing, so every successful run permanently occupies a slot
// and the fleet saturates for good after DefaultCap launches — with every check green,
// which is exactly the silence this whole change removes.
func TestTickReapsRunsWhoseUnitHasFinished(t *testing.T) {
	done := Run{Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}
	stillGoing := Run{Provider: "lever", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}

	repo := &fakeRepo{
		eligible:     []Settings{managed("greenhouse"), managed("lever")},
		inFlightRuns: []Run{done, stillGoing},
	}
	launcher := &fakeLauncher{
		outcomes: map[string]Outcome{
			"greenhouse/1": {Done: true},
			"lever/1":      {Done: false},
		},
	}

	got, err := newScheduler(repo, launcher, true).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(repo.finished) != 1 {
		t.Fatalf("finished = %v, want the one whose unit is gone", repo.finished)
	}
	if repo.finished[0].provider != "greenhouse" {
		t.Errorf("reaped %s, want greenhouse", repo.finished[0].provider)
	}
	if repo.finished[0].exitCode != 0 {
		t.Errorf("exit code = %d, want 0 — a vanished transient unit succeeded", repo.finished[0].exitCode)
	}
	if got.Reaped != 1 {
		t.Errorf("Reaped = %d, want 1", got.Reaped)
	}
}

// A crawl that ran and failed must be recorded with ITS exit code, not with the
// launch-failure code and not as a success. last_exit_code is what an operator reads to
// tell "never started" from "started and died".
func TestTickReapsAFailedRunWithItsOwnExitCode(t *testing.T) {
	failed := Run{Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}
	repo := &fakeRepo{
		eligible:     []Settings{managed("greenhouse")},
		inFlightRuns: []Run{failed},
	}
	launcher := &fakeLauncher{
		outcomes: map[string]Outcome{
			"greenhouse/1": {Done: true, ExitCode: 1, Detail: "exit-code"},
		},
	}

	if _, err := newScheduler(repo, launcher, true).Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(repo.finished) != 1 {
		t.Fatalf("finished = %v, want one", repo.finished)
	}
	if repo.finished[0].exitCode != 1 {
		t.Errorf("exit code = %d, want the run's own 1", repo.finished[0].exitCode)
	}
	if repo.finished[0].runErr == "" {
		t.Error("the failure detail was dropped")
	}
}

// The reap must run BEFORE the budget is computed, or a tick that just freed nine slots
// would still refuse to launch anything until the next minute.
func TestTickReapsBeforeItMeasuresFreeCapacity(t *testing.T) {
	var inFlight []Run
	for i := 1; i <= 10; i++ {
		inFlight = append(inFlight, Run{Provider: "paylocity", Shard: i, Shards: 24, RunTimeout: DefaultRunTimeout})
	}
	outcomes := map[string]Outcome{}
	for i := 1; i <= 9; i++ {
		outcomes[fmt.Sprintf("paylocity/%d", i)] = Outcome{Done: true}
	}
	outcomes["paylocity/10"] = Outcome{Done: false}

	repo := &fakeRepo{
		eligible:     []Settings{managed("paylocity")},
		inFlightRuns: inFlight,
		due:          []Run{{Provider: "paylocity", Shard: 11, Shards: 24, RunTimeout: DefaultRunTimeout}},
	}
	launcher := &fakeLauncher{outcomes: outcomes}

	got, err := newScheduler(repo, launcher, true).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if got.Saturated {
		t.Error("Saturated = true after reaping nine of ten runs; the reap must precede the budget")
	}
	// All ten runs are paylocity (Shards 24, heavy), so this exercises the heavy pool's
	// own budget: DefaultHeavyCap 5 - the one still running = 4.
	if repo.heavyLimit != 4 {
		t.Errorf("heavy claim limit = %d, want DefaultHeavyCap 5 - 1 still running = 4", repo.heavyLimit)
	}
}

// Shadow mode must reap too. It is the mode the fleet sits in for a full day, and a shadow
// run whose in-flight count only ever grows measures a saturation that is not real.
func TestShadowModeStillReaps(t *testing.T) {
	repo := &fakeRepo{
		eligible:     []Settings{managed("greenhouse")},
		inFlightRuns: []Run{{Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}},
	}
	launcher := &fakeLauncher{outcomes: map[string]Outcome{"greenhouse/1": {Done: true}}}

	if _, err := newScheduler(repo, launcher, false).Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(repo.finished) != 1 {
		t.Errorf("shadow mode reaped %v; a stale claim it never made must still be cleared", repo.finished)
	}
}
