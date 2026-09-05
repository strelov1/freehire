package ingestsched

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// fakeRepo records what the scheduler asked for and hands back what a test staged. It
// stands in for Postgres wherever the assertion is about the scheduler's DECISIONS —
// the SQL itself is proven in repository_integration_test.go against a real database.
type fakeRepo struct {
	eligible     []Settings
	due          []Run
	inFlightRuns []Run

	reconciled     []Settings
	reconcileSkips []Skipped
	claimLimit     int
	claimed        bool
	previewed      bool
	finished       []finishCall

	claimErr error
}

type finishCall struct {
	provider string
	shard    int
	exitCode int
	runErr   string
}

func (f *fakeRepo) Eligible(context.Context) ([]Settings, error) { return f.eligible, nil }

func (f *fakeRepo) Reconcile(_ context.Context, s []Settings) ([]Skipped, error) {
	f.reconciled = s
	return f.reconcileSkips, nil
}

func (f *fakeRepo) InFlightRuns(context.Context) ([]Run, error) { return f.inFlightRuns, nil }

func (f *fakeRepo) Claim(_ context.Context, limit int, _ time.Duration) ([]Run, error) {
	f.claimed = true
	f.claimLimit = limit
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	return f.take(limit), nil
}

func (f *fakeRepo) PreviewDue(_ context.Context, limit int, _ time.Duration) ([]Run, error) {
	f.previewed = true
	f.claimLimit = limit
	return f.take(limit), nil
}

func (f *fakeRepo) take(limit int) []Run {
	if limit >= len(f.due) {
		out := f.due
		f.due = nil
		return out
	}
	out := f.due[:limit]
	f.due = f.due[limit:]
	return out
}

func (f *fakeRepo) RecordFinish(_ context.Context, provider string, shard, exitCode int, runErr string) error {
	f.finished = append(f.finished, finishCall{provider, shard, exitCode, runErr})
	return nil
}

type fakeLauncher struct {
	launched []Run
	err      error
	// outcomes is keyed "provider/shard"; a run absent from it is still running.
	outcomes map[string]Outcome
}

func (f *fakeLauncher) Launch(_ context.Context, run Run) error {
	if f.err != nil {
		return f.err
	}
	f.launched = append(f.launched, run)
	return nil
}

func (f *fakeLauncher) Finished(_ context.Context, run Run) (Outcome, error) {
	return f.outcomes[fmt.Sprintf("%s/%d", run.Provider, run.Shard)], nil
}

func managed(provider string) Settings {
	return Effective(provider, &Override{
		Provider: provider, Shards: 1, Cadence: time.Hour,
		RunTimeout: DefaultRunTimeout, Enabled: true, Managed: true,
	})
}

func newScheduler(repo *fakeRepo, launcher Launcher, apply bool) Scheduler {
	return Scheduler{Repo: repo, Launcher: launcher, Cap: 10, Grace: time.Minute, Apply: apply}
}

// Shadow mode is the DEFAULT, and the first deployment lands underneath a fleet still
// driven by 279 static timers. A shadow tick that claimed anything would advance due times
// the real timers know nothing about.
//
// It DOES reconcile, and that is deliberate rather than an oversight: without run-state
// rows there is nothing for the preview to see, and a shadow run that measured nothing
// would be worse than no shadow run at all. Creating those rows is inert — only the
// scheduler reads them, and only in apply mode does it act. What shadow mode must not do is
// CLAIM: that is the write the static timers would be racing.
func TestShadowTickReconcilesButNeitherClaimsNorLaunches(t *testing.T) {
	repo := &fakeRepo{
		eligible: []Settings{managed("greenhouse")},
		due:      []Run{{Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}},
	}
	launcher := &fakeLauncher{}

	got, err := newScheduler(repo, launcher, false).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(launcher.launched) != 0 {
		t.Errorf("shadow mode launched %v; want nothing", launcher.launched)
	}
	if !repo.previewed {
		t.Error("shadow mode did not preview; it must still report what it would do")
	}
	if len(got.WouldLaunch) != 1 {
		t.Errorf("WouldLaunch = %v, want the one due run", got.WouldLaunch)
	}
	if got.Applied {
		t.Error("Applied = true in shadow mode")
	}
	if repo.claimed {
		t.Error("shadow mode claimed; that is the write the static timers would be racing")
	}
	if len(repo.reconciled) != 1 {
		t.Errorf("reconciled %v; shadow mode must still materialise run state or the preview sees nothing", repo.reconciled)
	}
}

func TestApplyTickClaimsAndLaunches(t *testing.T) {
	repo := &fakeRepo{
		eligible: []Settings{managed("greenhouse")},
		due:      []Run{{Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}},
	}
	launcher := &fakeLauncher{}

	got, err := newScheduler(repo, launcher, true).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(launcher.launched) != 1 || launcher.launched[0].Provider != "greenhouse" {
		t.Fatalf("launched %v, want one greenhouse run", launcher.launched)
	}
	if repo.previewed {
		t.Error("apply mode used the preview read instead of claiming")
	}
	if !got.Applied {
		t.Error("Applied = false in apply mode")
	}
}

// The concurrency cap replaces ingest-slot.sh's flock semaphore. It exists because 279
// independent timers could not see each other; one scheduler can simply count.
func TestTickLaunchesOnlyTheFreeCapacity(t *testing.T) {
	repo := &fakeRepo{
		eligible:     []Settings{managed("paylocity")},
		inFlightRuns: stillRunning("paylocity", 7),
		due: []Run{
			{Provider: "paylocity", Shard: 1, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 2, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 3, Shards: 24, RunTimeout: DefaultRunTimeout},
			{Provider: "paylocity", Shard: 4, Shards: 24, RunTimeout: DefaultRunTimeout},
		},
	}
	launcher := &fakeLauncher{}

	if _, err := newScheduler(repo, launcher, true).Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if repo.claimLimit != 3 {
		t.Errorf("claim limit = %d, want 10 - 7 in flight = 3", repo.claimLimit)
	}
	if len(launcher.launched) != 3 {
		t.Errorf("launched %d runs, want 3", len(launcher.launched))
	}
}

// A saturated tick must be loud and must leave every due row claimable. A fleet that
// quietly stops crawling looks identical to a healthy one — the reason ingest-slot.sh
// logged its skips too.
func TestSaturatedTickLaunchesNothingAndSaysSo(t *testing.T) {
	repo := &fakeRepo{
		eligible:     []Settings{managed("greenhouse")},
		inFlightRuns: stillRunning("paylocity", 10),
		due:          []Run{{Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}},
	}
	launcher := &fakeLauncher{}

	got, err := newScheduler(repo, launcher, true).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if !got.Saturated {
		t.Error("Saturated = false, want true")
	}
	if len(launcher.launched) != 0 {
		t.Errorf("a saturated tick launched %v", launcher.launched)
	}
	if repo.claimLimit != 0 {
		t.Errorf("a saturated tick asked to claim %d; it must not claim at all", repo.claimLimit)
	}
}

// Run state is TRACKED for every enabled provider, including the ones still owned by their
// static timer. Two reasons, and the second is the one that nearly sank the rollout plan:
//
//   - Reconcile deletes the state of every provider absent from its list. Passing only the
//     MANAGED ones would run a full-table delete every minute for the whole cutover, since
//     managed defaults to false — discarding the stagger and the run history continuously.
//   - Shadow mode previews from run state. With no rows there is nothing to preview, so the
//     full day of shadow output §8.3 is supposed to be read would have measured nothing at
//     all.
//
// What `managed` gates is the LAUNCH, and that gate lives in the claim predicate.
func TestTickTracksEveryEnabledProviderIncludingUnmanagedOnes(t *testing.T) {
	unmanaged := managed("lever")
	unmanaged.Managed = false

	disabled := managed("bayt")
	disabled.Enabled = false
	disabled.DisabledReason = "fingerprint client has no proxy support"

	repo := &fakeRepo{eligible: []Settings{managed("greenhouse"), unmanaged, disabled}}

	got, err := newScheduler(repo, &fakeLauncher{}, true).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	names := make([]string, 0, len(repo.reconciled))
	for _, s := range repo.reconciled {
		names = append(names, s.Provider)
	}
	if len(names) != 2 || names[0] != "greenhouse" || names[1] != "lever" {
		t.Fatalf("reconciled %v, want greenhouse and lever — every ENABLED provider", names)
	}
	if got.Tracked != 2 {
		t.Errorf("Tracked = %d, want 2", got.Tracked)
	}
	// bayt is out because it is disabled, which is a decision with a reason attached.
	if len(got.Disabled) != 1 || got.Disabled[0].Provider != "bayt" {
		t.Errorf("Disabled = %v, want only bayt", got.Disabled)
	}
}

// Only a managed provider is driven while the static timers still run, or the two would
// both crawl it. The Managed conjunct goes away with the column in task 8.5.
func TestTickSchedulesOnlyManagedEnabledProviders(t *testing.T) {
	unmanaged := managed("lever")
	unmanaged.Managed = false

	disabled := managed("bayt")
	disabled.Enabled = false
	disabled.DisabledReason = "fingerprint client has no proxy support"

	repo := &fakeRepo{eligible: []Settings{managed("greenhouse"), unmanaged, disabled}}
	launcher := &fakeLauncher{}

	got, err := newScheduler(repo, launcher, true).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if got.Eligible != 3 || got.Tracked != 2 {
		t.Errorf("Eligible/Tracked = %d/%d, want 3/2", got.Eligible, got.Tracked)
	}
	// A curator's decision and a rollout state are reported separately. During cutover
	// Unmanaged holds ~226 providers; mixing them would bury the two genuinely turned off.
	if len(got.Disabled) != 1 || got.Disabled[0].Provider != "bayt" {
		t.Errorf("Disabled = %v, want only bayt so its reason stays visible", got.Disabled)
	}
	if got.Disabled[0].Reason == "" {
		t.Error("bayt's disable reason was dropped on the way to the report")
	}
	if len(got.Unmanaged) != 1 || got.Unmanaged[0] != "lever" {
		t.Errorf("Unmanaged = %v, want [lever]", got.Unmanaged)
	}
}

// A provider key no adapter answers to must be reported and skipped, never scheduled.
// This is habr_career's failure caught at the other end: there, a name that selected
// nothing simply exited 0 and looked healthy.
func TestTickRefusesAProviderKeyTheRegistryDoesNotKnow(t *testing.T) {
	ghost := managed("habrcareer") // the FILE name; the adapter answers to habr_career
	repo := &fakeRepo{eligible: []Settings{managed("greenhouse"), ghost}}
	launcher := &fakeLauncher{}

	got, err := newScheduler(repo, launcher, true).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(repo.reconciled) != 1 || repo.reconciled[0].Provider != "greenhouse" {
		t.Fatalf("reconciled %v, want only greenhouse", repo.reconciled)
	}
	if len(got.Refused) != 1 || got.Refused[0].Provider != "habrcareer" {
		t.Fatalf("Refused = %v, want habrcareer", got.Refused)
	}
	if got.Refused[0].Reason == "" {
		t.Error("a refusal with no reason is the silence this change removes")
	}
}

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
	if repo.claimLimit != 9 {
		t.Errorf("claim limit = %d, want 10 - 1 still running = 9", repo.claimLimit)
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

// One provider that cannot be reconciled must not stop the fleet. The per-provider timers
// this replaces had that isolation for free; concentrating 279 of them into one process is
// the moment to state it, because now one bad row can stop everything.
func TestOneUnreconcilableProviderDoesNotStopTheTick(t *testing.T) {
	repo := &fakeRepo{
		eligible:       []Settings{managed("greenhouse"), managed("lever")},
		reconcileSkips: []Skipped{{Provider: "lever", Reason: "ensure shards: lock timeout"}},
		due:            []Run{{Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}},
	}
	launcher := &fakeLauncher{}

	got, err := newScheduler(repo, launcher, true).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(launcher.launched) != 1 {
		t.Errorf("launched %v; greenhouse must still run", launcher.launched)
	}
	if len(got.Failed) != 1 || got.Failed[0].Provider != "lever" {
		t.Errorf("Failed = %v, want lever reported rather than swallowed", got.Failed)
	}
}

// An empty roster is a failed MEASUREMENT, not an empty catalogue. Reconcile deletes the
// run state of every provider absent from its list, so a tick that accepted zero eligible
// providers would wipe the fleet's entire schedule — including the stagger — on the
// strength of one bad read. gen-ingest-timers.sh refused on exactly this and said so; the
// scheduler must not lose that.
func TestTickRefusesAnEmptyRoster(t *testing.T) {
	repo := &fakeRepo{eligible: nil}
	launcher := &fakeLauncher{}

	_, err := newScheduler(repo, launcher, true).Tick(context.Background())
	if err == nil {
		t.Fatal("an empty roster was accepted; want a refusal")
	}
	if repo.reconciled != nil {
		t.Errorf("reconciled %v on an empty roster; run state must be left alone", repo.reconciled)
	}
}

// A roster that is non-empty but has nothing SCHEDULABLE is different, and must be
// allowed: on the first day of the cutover every provider is still unmanaged, and that is
// the expected state, not a failure.
func TestTickAcceptsARosterWhereNothingIsSchedulableYet(t *testing.T) {
	unmanaged := managed("greenhouse")
	unmanaged.Managed = false
	repo := &fakeRepo{eligible: []Settings{unmanaged}}
	launcher := &fakeLauncher{}

	got, err := newScheduler(repo, launcher, true).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(got.Unmanaged) != 1 || got.Unmanaged[0] != "greenhouse" {
		t.Errorf("Unmanaged = %v, want [greenhouse]", got.Unmanaged)
	}
}

// A launch that fails must release its claim immediately. Leaving it claimed would idle
// that shard for the whole reclaim window — timeout plus grace — over an error the
// scheduler already knows about.
func TestAFailedLaunchReleasesItsClaimAtOnce(t *testing.T) {
	repo := &fakeRepo{
		eligible: []Settings{managed("greenhouse")},
		due:      []Run{{Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}},
	}
	launcher := &fakeLauncher{err: errors.New("systemd-run: unit already exists")}

	got, err := newScheduler(repo, launcher, true).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(repo.finished) != 1 {
		t.Fatalf("finished = %v, want the failed run released", repo.finished)
	}
	if repo.finished[0].exitCode == 0 {
		t.Error("a failed launch was recorded with exit code 0")
	}
	if repo.finished[0].runErr == "" {
		t.Error("the launch error was not recorded")
	}
	if len(got.Launched) != 0 {
		t.Errorf("Launched = %v after every launch failed", got.Launched)
	}
}

// One provider's bad row must not stop the fleet. The old per-provider timers had that
// isolation for free; concentrating the fleet into one process is the moment to state it.
func TestOneFailedLaunchDoesNotStopTheRest(t *testing.T) {
	repo := &fakeRepo{
		eligible: []Settings{managed("greenhouse"), managed("lever")},
		due: []Run{
			{Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout},
			{Provider: "lever", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout},
		},
	}
	launcher := &failFirstLauncher{}

	got, err := newScheduler(repo, launcher, true).Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(got.Launched) != 1 || got.Launched[0].Provider != "lever" {
		t.Errorf("Launched = %v, want lever to have run despite greenhouse failing", got.Launched)
	}
}

type failFirstLauncher struct{ calls int }

func (f *failFirstLauncher) Launch(_ context.Context, _ Run) error {
	f.calls++
	if f.calls == 1 {
		return errors.New("first launch fails")
	}
	return nil
}

func (f *failFirstLauncher) Finished(context.Context, Run) (Outcome, error) {
	return Outcome{}, nil
}

// stillRunning stages n in-flight runs whose units the fake launcher reports as alive,
// since an outcome absent from its map means "not finished".
func stillRunning(provider string, n int) []Run {
	out := make([]Run, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, Run{Provider: provider, Shard: i, Shards: 24, RunTimeout: DefaultRunTimeout})
	}
	return out
}
