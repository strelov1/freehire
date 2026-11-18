//go:build integration

// Integration tests for the scheduling repository against a real Postgres: eligibility
// read from boards, run-state reconciliation, and the claim — the three things a fake
// cannot prove, because what is being asserted is SQL (a LEFT JOIN's nullability, a
// partial index's predicate, FOR UPDATE SKIP LOCKED's behaviour under concurrency).
package ingestsched

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func newRepo(t *testing.T) (*QueriesRepository, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	return NewQueriesRepository(db.New(pool)), pool
}

// addBoard puts one live board in the catalog, which is the only thing that makes a
// provider eligible. Provider keys here are real registry keys, because the scheduler
// validates against the registry before launching.
func addBoard(t *testing.T, pool *pgxpool.Pool, provider, board string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO boards (provider, board, company, status) VALUES ($1, $2, $3, 'active')`,
		provider, board, "Test Co")
	if err != nil {
		t.Fatalf("add board %s/%s: %v", provider, board, err)
	}
}

// manage hands a provider to the scheduler. The claim and preview queries carry the
// rollout gate (COALESCE(s.managed, false)), so a test that expects a run to be claimed
// must say so explicitly — which is the point: during cutover, a provider nobody has
// handed over is still its static timer's.
func manage(t *testing.T, pool *pgxpool.Pool, provider string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO ingest_schedule (provider, managed) VALUES ($1, true)
		 ON CONFLICT (provider) DO UPDATE SET managed = true`, provider)
	if err != nil {
		t.Fatalf("manage %s: %v", provider, err)
	}
}

// markHeavy flags a provider heavy directly on its ingest_schedule row — the column the
// claim queries actually join against — rather than through a Go-level Settings value, so a
// test using it proves the SQL join sees the flag rather than trusting Effective's mapping
// of it.
func markHeavy(t *testing.T, pool *pgxpool.Pool, provider string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO ingest_schedule (provider, heavy) VALUES ($1, true)
		 ON CONFLICT (provider) DO UPDATE SET heavy = true`, provider)
	if err != nil {
		t.Fatalf("mark %s heavy: %v", provider, err)
	}
}

func settingsFor(t *testing.T, repo *QueriesRepository, provider string) Settings {
	t.Helper()
	all, err := repo.Eligible(context.Background())
	if err != nil {
		t.Fatalf("Eligible: %v", err)
	}
	for _, s := range all {
		if s.Provider == provider {
			return s
		}
	}
	t.Fatalf("provider %q not eligible; got %v", provider, all)
	return Settings{}
}

// The roster is boards, and this is the test that says so. A provider is eligible because
// it has a live board — not because a row was added to ingest_schedule, and not because a
// file with its name exists.
func TestEligibleReadsTheRosterFromBoards(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	addBoard(t, pool, "greenhouse", "acme")
	addBoard(t, pool, "greenhouse", "globex") // two boards, still one provider
	addBoard(t, pool, "lever", "initech")

	// A retired-only provider is not eligible: its timer used to survive forever because
	// the generator only ever created and enabled units.
	_, err := pool.Exec(ctx,
		`INSERT INTO boards (provider, board, company, status) VALUES ('ashby', 'gone', 'Gone', 'retired')`)
	if err != nil {
		t.Fatalf("seed retired board: %v", err)
	}

	got, err := repo.Eligible(ctx)
	if err != nil {
		t.Fatalf("Eligible: %v", err)
	}

	names := make([]string, 0, len(got))
	for _, s := range got {
		names = append(names, s.Provider)
	}
	if len(names) != 2 || names[0] != "greenhouse" || names[1] != "lever" {
		t.Fatalf("eligible providers = %v, want [greenhouse lever] in order", names)
	}
}

// The rule the whole change turns on, asserted end-to-end through SQL: a provider with a
// live board and NO ingest_schedule row comes back scheduled on defaults.
func TestEligibleDefaultsAProviderWithNoOverrideRow(t *testing.T) {
	repo, pool := newRepo(t)
	addBoard(t, pool, "greenhouse", "acme")

	got := settingsFor(t, repo, "greenhouse")
	if got.Overridden {
		t.Error("Overridden = true, want false")
	}
	if got.Shards != DefaultShards || got.Cadence != DefaultCadence || got.RunTimeout != DefaultRunTimeout {
		t.Errorf("got %d shards / %v / %v, want defaults", got.Shards, got.Cadence, got.RunTimeout)
	}
	if !got.Enabled {
		t.Error("Enabled = false; an unconfigured provider must still be scheduled")
	}
}

func TestEligibleAppliesAnOverrideRow(t *testing.T) {
	repo, pool := newRepo(t)
	addBoard(t, pool, "paylocity", "acme")
	_, err := pool.Exec(context.Background(),
		`INSERT INTO ingest_schedule (provider, shards, cadence_sec, timeout_sec, notes, managed)
		 VALUES ('paylocity', 24, 86400, 4500, 'measured ~10.42s/board', true)`)
	if err != nil {
		t.Fatalf("seed override: %v", err)
	}

	got := settingsFor(t, repo, "paylocity")
	if !got.Overridden {
		t.Error("Overridden = false, want true")
	}
	if got.Shards != 24 {
		t.Errorf("Shards = %d, want 24", got.Shards)
	}
	if got.Cadence != 24*time.Hour {
		t.Errorf("Cadence = %v, want 24h", got.Cadence)
	}
	if got.RunTimeout != 4500*time.Second {
		t.Errorf("RunTimeout = %v, want 4500s", got.RunTimeout)
	}
	if !got.Managed {
		t.Error("Managed = false, want true")
	}
	if got.Notes == "" {
		t.Error("Notes were dropped between the row and the report")
	}
}

func TestReconcileMaterialisesOneRowPerShard(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "workday", "acme")

	settings := Effective("workday", &Override{
		Provider: "workday", Shards: 6, Cadence: 6 * time.Hour,
		RunTimeout: DefaultRunTimeout, Enabled: true,
	})
	if _, err := repo.Reconcile(ctx, []Settings{settings}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if got := shardsInState(t, pool, "workday"); len(got) != 6 {
		t.Fatalf("shards = %v, want 1..6", got)
	}
}

// Raising the shard count must not disturb the shards that already exist — their due times
// are the fleet's stagger, and resetting them would bunch a provider's whole cycle onto one
// minute.
func TestReconcileKeepsExistingDueTimesWhenShardsChange(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "join", "acme")

	four := Effective("join", &Override{
		Provider: "join", Shards: 4, Cadence: time.Hour, RunTimeout: DefaultRunTimeout, Enabled: true,
	})
	if _, err := repo.Reconcile(ctx, []Settings{four}); err != nil {
		t.Fatalf("Reconcile 4: %v", err)
	}
	before := dueAt(t, pool, "join", 2)

	five := four
	five.Shards = 5
	if _, err := repo.Reconcile(ctx, []Settings{five}); err != nil {
		t.Fatalf("Reconcile 5: %v", err)
	}

	if got := shardsInState(t, pool, "join"); len(got) != 5 {
		t.Fatalf("shards = %v, want 1..5", got)
	}
	if after := dueAt(t, pool, "join", 2); !after.Equal(before) {
		t.Errorf("shard 2 due time moved from %v to %v; an untouched shard must keep its stagger", before, after)
	}
}

func TestReconcileDropsSurplusShardsAndDepartedProviders(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "paylocity", "acme")
	addBoard(t, pool, "greenhouse", "globex")

	wide := Effective("paylocity", &Override{
		Provider: "paylocity", Shards: 24, Cadence: time.Hour, RunTimeout: DefaultRunTimeout, Enabled: true,
	})
	if _, err := repo.Reconcile(ctx, []Settings{wide, Effective("greenhouse", nil)}); err != nil {
		t.Fatalf("Reconcile wide: %v", err)
	}

	narrow := wide
	narrow.Shards = 12
	// greenhouse is left out of this call: its boards were all retired, so it is no longer
	// eligible and its run state must go with it. Under the script this replaced, its timer
	// would have survived forever.
	if _, err := repo.Reconcile(ctx, []Settings{narrow}); err != nil {
		t.Fatalf("Reconcile narrow: %v", err)
	}

	if got := shardsInState(t, pool, "paylocity"); len(got) != 12 {
		t.Fatalf("paylocity shards = %v, want 1..12", got)
	}
	if got := shardsInState(t, pool, "greenhouse"); len(got) != 0 {
		t.Errorf("greenhouse run state = %v, want none — it is no longer eligible", got)
	}
}

func TestClaimTakesOnlyDueRunsAndAdvancesFromNow(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "greenhouse", "acme")
	manage(t, pool, "greenhouse")

	if _, err := repo.Reconcile(ctx, []Settings{Effective("greenhouse", nil)}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	claimed, err := repo.Claim(ctx, false, 10, time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d runs, want 1", len(claimed))
	}
	if claimed[0].Provider != "greenhouse" || claimed[0].Shard != 1 {
		t.Errorf("claimed %s/%d, want greenhouse/1", claimed[0].Provider, claimed[0].Shard)
	}
	if claimed[0].RunTimeout != DefaultRunTimeout {
		t.Errorf("RunTimeout = %v, want the default", claimed[0].RunTimeout)
	}

	// The second tick must find nothing: the row is claimed and its next due time is an
	// hour out.
	again, err := repo.Claim(ctx, false, 10, time.Minute)
	if err != nil {
		t.Fatalf("second Claim: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("second claim took %d runs, want 0", len(again))
	}
}

// Catch-up is capped at one run. A row six hours overdue must come back due in one
// cadence from NOW, not owe six runs that would stampede the fleet.
func TestClaimAdvancesFromNowNotFromTheMissedDueTime(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "greenhouse", "acme")
	manage(t, pool, "greenhouse")
	if _, err := repo.Reconcile(ctx, []Settings{Effective("greenhouse", nil)}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	_, err := pool.Exec(ctx,
		`UPDATE ingest_run_state SET next_due_at = now() - interval '6 hours' WHERE provider = 'greenhouse'`)
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if _, err := repo.Claim(ctx, false, 10, time.Minute); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	next := dueAt(t, pool, "greenhouse", 1)
	if wait := time.Until(next); wait < 55*time.Minute {
		t.Errorf("next due in %v; want ~1h from now, so a long outage owes one run and not six", wait)
	}
}

func TestClaimReclaimsARunThatOutlivedItsTimeoutPlusGrace(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "greenhouse", "acme")
	manage(t, pool, "greenhouse")
	if _, err := repo.Reconcile(ctx, []Settings{Effective("greenhouse", nil)}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if _, err := repo.Claim(ctx, false, 10, time.Minute); err != nil {
		t.Fatalf("first Claim: %v", err)
	}

	// Still inside the window: a slow run is not a dead one.
	_, err := pool.Exec(ctx,
		`UPDATE ingest_run_state SET claimed_at = now() - interval '10 minutes' WHERE provider = 'greenhouse'`)
	if err != nil {
		t.Fatalf("age the claim: %v", err)
	}
	live, err := repo.Claim(ctx, false, 10, time.Minute)
	if err != nil {
		t.Fatalf("Claim inside window: %v", err)
	}
	if len(live) != 0 {
		t.Fatalf("reclaimed a run still inside its budget (%d)", len(live))
	}

	// Past timeout (3000s) plus grace: dead, and claimable again.
	_, err = pool.Exec(ctx,
		`UPDATE ingest_run_state SET claimed_at = now() - interval '2 hours' WHERE provider = 'greenhouse'`)
	if err != nil {
		t.Fatalf("age the claim further: %v", err)
	}
	dead, err := repo.Claim(ctx, false, 10, time.Minute)
	if err != nil {
		t.Fatalf("Claim past window: %v", err)
	}
	if len(dead) != 1 {
		t.Errorf("reclaimed %d runs, want 1 — a scheduler killed between claim and launch must recover", len(dead))
	}
}

func TestClaimRespectsItsLimit(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "paylocity", "acme")
	manage(t, pool, "paylocity")

	wide := Effective("paylocity", &Override{
		Provider: "paylocity", Shards: 24, Cadence: time.Hour, RunTimeout: DefaultRunTimeout, Enabled: true,
	})
	if _, err := repo.Reconcile(ctx, []Settings{wide}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	claimed, err := repo.Claim(ctx, true, 3, time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(claimed) != 3 {
		t.Fatalf("claimed %d, want exactly the limit of 3", len(claimed))
	}
	// The shard COUNT travels with the claim, because it is what turns shard 3 into
	// `--shard=3/24`. Reading it again at launch could pick up a count a curator changed
	// in between, and build a selector the scheduler never decided on.
	for _, run := range claimed {
		if run.Shards != 24 {
			t.Errorf("claimed %s/%d with Shards = %d, want 24", run.Provider, run.Shard, run.Shards)
		}
	}
}

// Two scheduler ticks can overlap — a slow tick and the next minute's timer, or an
// operator running it by hand. Whatever the interleaving, one due row must produce exactly
// one run: FOR UPDATE SKIP LOCKED makes the loser skip rather than block or double-claim.
func TestConcurrentClaimsNeverTakeTheSameRunTwice(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "greenhouse", "acme")
	manage(t, pool, "greenhouse")
	if _, err := repo.Reconcile(ctx, []Settings{Effective("greenhouse", nil)}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	const racers = 8
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		total  int
		errsCh []error
	)
	wg.Add(racers)
	for range racers {
		go func() {
			defer wg.Done()
			claimed, err := repo.Claim(ctx, false, 10, time.Minute)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errsCh = append(errsCh, err)
				return
			}
			total += len(claimed)
		}()
	}
	wg.Wait()

	if len(errsCh) > 0 {
		t.Fatalf("claims errored: %v", errsCh)
	}
	if total != 1 {
		t.Errorf("%d racers claimed %d runs in total, want exactly 1", racers, total)
	}
}

// A disabled provider must end up with no run state at all, which is what makes it
// unclaimable without the claim query needing to know about `enabled`. Its state is
// deleted rather than left behind, so re-enabling starts from a clean due time instead of
// a months-old one that would fire immediately.
func TestADisabledProviderIsNotScheduledAndItsStateIsDropped(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "greenhouse", "acme")

	on := Effective("greenhouse", &Override{
		Provider: "greenhouse", Shards: 1, Cadence: time.Hour,
		RunTimeout: DefaultRunTimeout, Enabled: true, Managed: true,
	})
	if !on.Schedulable() {
		t.Fatal("an enabled, managed provider must be schedulable")
	}
	if _, err := repo.Reconcile(ctx, []Settings{on}); err != nil {
		t.Fatalf("Reconcile on: %v", err)
	}
	if got := shardsInState(t, pool, "greenhouse"); len(got) != 1 {
		t.Fatalf("shards = %v, want one", got)
	}

	off := on
	off.Enabled = false
	off.DisabledReason = "fingerprint client has no proxy support"
	if off.Schedulable() {
		t.Fatal("a disabled provider must not be schedulable")
	}

	// The scheduler reconciles only what is schedulable, so a disabled provider simply
	// is not in the list.
	if _, err := repo.Reconcile(ctx, nil); err != nil {
		t.Fatalf("Reconcile off: %v", err)
	}
	if got := shardsInState(t, pool, "greenhouse"); len(got) != 0 {
		t.Errorf("run state = %v after disabling, want none", got)
	}

	claimed, err := repo.Claim(ctx, false, 10, time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("claimed %d runs for a disabled provider, want 0", len(claimed))
	}
}

func TestInFlightCountsClaimedRuns(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "paylocity", "acme")
	manage(t, pool, "paylocity")

	wide := Effective("paylocity", &Override{
		Provider: "paylocity", Shards: 4, Cadence: time.Hour, RunTimeout: DefaultRunTimeout, Enabled: true,
	})
	if _, err := repo.Reconcile(ctx, []Settings{wide}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if n, err := repo.InFlightRuns(ctx, true); err != nil || len(n) != 0 {
		t.Fatalf("InFlightRuns before any claim = %d (%v), want 0", len(n), err)
	}
	if _, err := repo.Claim(ctx, true, 3, time.Minute); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if n, err := repo.InFlightRuns(ctx, true); err != nil || len(n) != 3 {
		t.Fatalf("InFlightRuns after claiming 3 = %d (%v), want 3", len(n), err)
	}
	if err := repo.RecordFinish(ctx, "paylocity", 1, 0, ""); err != nil {
		t.Fatalf("RecordFinish: %v", err)
	}
	if n, err := repo.InFlightRuns(ctx, true); err != nil || len(n) != 2 {
		t.Fatalf("InFlightRuns after one finish = %d (%v), want 2", len(n), err)
	}
}

func TestRecordFinishClearsTheClaimAndStoresTheOutcome(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "greenhouse", "acme")
	manage(t, pool, "greenhouse")
	if _, err := repo.Reconcile(ctx, []Settings{Effective("greenhouse", nil)}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if _, err := repo.Claim(ctx, false, 10, time.Minute); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if err := repo.RecordFinish(ctx, "greenhouse", 1, 0, ""); err != nil {
		t.Fatalf("RecordFinish: %v", err)
	}

	var (
		claimedAt  *time.Time
		finishedAt *time.Time
		exitCode   *int32
	)
	err := pool.QueryRow(ctx,
		`SELECT claimed_at, last_finished_at, last_exit_code FROM ingest_run_state
		  WHERE provider = 'greenhouse' AND shard = 1`).
		Scan(&claimedAt, &finishedAt, &exitCode)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if claimedAt != nil {
		t.Error("claimed_at still set after the run finished")
	}
	if finishedAt == nil {
		t.Error("last_finished_at not recorded")
	}
	if exitCode == nil || *exitCode != 0 {
		t.Errorf("last_exit_code = %v, want 0", exitCode)
	}
}

// THE reservation this package's scheduler was missing, proven end to end through
// Scheduler.Tick against real SQL rather than only against the fake repository
// scheduler_test.go uses. Three heavy providers (flagged via ingest_schedule.heavy — the
// unsharded arm of IsHeavy, not the shard-count one every other test in this file already
// exercises) and three light ones are all due at once. With HeavyCap=1 and Cap=3 (so the
// light pool gets Cap-HeavyCap=2), exactly one heavy run and exactly two light runs must be
// claimed in a single tick — never fewer of one pool because the other pool is full, and
// never more of either than its own budget allows.
func TestSchedulerClaimsIndependentBudgetsPerPool(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	heavyProviders := []string{"paylocity", "workday", "oracle"}
	lightProviders := []string{"greenhouse", "lever", "ashby"}

	var settings []Settings
	for _, p := range heavyProviders {
		addBoard(t, pool, p, "acme")
		manage(t, pool, p)
		markHeavy(t, pool, p)
		settings = append(settings, Effective(p, nil))
	}
	for _, p := range lightProviders {
		addBoard(t, pool, p, "acme")
		manage(t, pool, p)
		settings = append(settings, Effective(p, nil))
	}
	if _, err := repo.Reconcile(ctx, settings); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	sched := Scheduler{Repo: repo, Launcher: &fakeLauncher{}, Cap: 3, HeavyCap: 1, Grace: time.Minute, Apply: true}
	got, err := sched.Tick(ctx)
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if got.Heavy.Launched != 1 {
		t.Errorf("Heavy.Launched = %d, want 1 (HeavyCap)", got.Heavy.Launched)
	}
	if got.Light.Launched != 2 {
		t.Errorf("Light.Launched = %d, want 2 (Cap 3 - HeavyCap 1)", got.Light.Launched)
	}
	if len(got.Launched) != 3 {
		t.Fatalf("Launched = %v, want exactly 3 runs total", got.Launched)
	}

	heavySet := map[string]bool{"paylocity": true, "workday": true, "oracle": true}
	heavyLaunched, lightLaunched := 0, 0
	for _, r := range got.Launched {
		if heavySet[r.Provider] {
			heavyLaunched++
		} else {
			lightLaunched++
		}
	}
	if heavyLaunched != 1 {
		t.Errorf("launched %d heavy runs, want exactly 1", heavyLaunched)
	}
	if lightLaunched != 2 {
		t.Errorf("launched %d light runs, want exactly 2 — one light provider must be left for the next tick", lightLaunched)
	}
}

func shardsInState(t *testing.T, pool *pgxpool.Pool, provider string) []int {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT shard FROM ingest_run_state WHERE provider = $1 ORDER BY shard`, provider)
	if err != nil {
		t.Fatalf("read shards: %v", err)
	}
	defer rows.Close()

	var out []int
	for rows.Next() {
		var shard int
		if err := rows.Scan(&shard); err != nil {
			t.Fatalf("scan shard: %v", err)
		}
		out = append(out, shard)
	}
	return out
}

func dueAt(t *testing.T, pool *pgxpool.Pool, provider string, shard int) time.Time {
	t.Helper()
	var due time.Time
	err := pool.QueryRow(context.Background(),
		`SELECT next_due_at FROM ingest_run_state WHERE provider = $1 AND shard = $2`,
		provider, shard).Scan(&due)
	if err != nil {
		t.Fatalf("read due time for %s/%d: %v", provider, shard, err)
	}
	return due
}
