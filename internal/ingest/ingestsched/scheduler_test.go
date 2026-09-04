package ingestsched

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepo records what the scheduler asked for and hands back what a test staged. It
// stands in for Postgres wherever the assertion is about the scheduler's DECISIONS —
// the SQL itself is proven in repository_integration_test.go against a real database.
type fakeRepo struct {
	eligible []Settings
	due      []Run
	inFlight int

	reconciled []Settings
	claimLimit int
	previewed  bool
	finished   []finishCall

	claimErr error
}

type finishCall struct {
	provider string
	shard    int
	exitCode int
	runErr   string
}

func (f *fakeRepo) Eligible(context.Context) ([]Settings, error) { return f.eligible, nil }

func (f *fakeRepo) Reconcile(_ context.Context, s []Settings) error {
	f.reconciled = s
	return nil
}

func (f *fakeRepo) InFlight(context.Context) (int, error) { return f.inFlight, nil }

func (f *fakeRepo) Claim(_ context.Context, limit int, _ time.Duration) ([]Run, error) {
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
}

func (f *fakeLauncher) Launch(_ context.Context, run Run) error {
	if f.err != nil {
		return f.err
	}
	f.launched = append(f.launched, run)
	return nil
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
func TestShadowTickLaunchesNothingAndMutatesNoState(t *testing.T) {
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
		eligible: []Settings{managed("paylocity")},
		inFlight: 7,
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
		eligible: []Settings{managed("greenhouse")},
		inFlight: 10,
		due:      []Run{{Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout}},
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

	if len(repo.reconciled) != 1 || repo.reconciled[0].Provider != "greenhouse" {
		t.Fatalf("reconciled %v, want only greenhouse", repo.reconciled)
	}
	if got.Eligible != 3 || got.Scheduled != 1 {
		t.Errorf("Eligible/Scheduled = %d/%d, want 3/1", got.Eligible, got.Scheduled)
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
