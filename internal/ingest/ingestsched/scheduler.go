package ingestsched

import (
	"context"
	"fmt"
	"time"
)

// DefaultCap is how many ingest runs may execute at once across the whole fleet.
//
// 10 is not a guess: it is the value ingest-slot.sh was calibrated to against this fleet
// after 8 measured short. Carrying it over unchanged keeps this change about the MECHANISM
// — a LIMIT instead of a flock semaphore — rather than quietly re-tuning throughput at the
// same time.
//
// Measured 2026-09-07: one flat cap is not enough. The fleet's problem was never total
// work — a full sweep costs ~11 slot-hours against 10 of capacity — but RESIDENCY: four of
// the ten were permanently held by 25-65 minute crawls, and the ~130 short runs an hour,
// worth 0.4 slot-hours together, could not get in edgewise. 42% of cycles were skipped
// while average utilisation sat near half, and skipping the cheap ones relieved nothing.
// ingest-slot.sh answers this with its HEAVY_SLOTS split (freehire-ops' scripts/host2/ingest-slot.sh); the
// reservation is implemented HERE now too, as HeavyCap/DefaultHeavyCap below, rather than
// only in the script this scheduler is cutting over from.
const DefaultCap = 10

// DefaultHeavyCap is how many of DefaultCap's slots are reserved for the heavy pool —
// every provider Settings.IsHeavy reports true for. It mirrors ingest-slot.sh's
// HEAVY_SLOTS, currently 5 there (raised from 4 on 2026-09-15, measured against a fleet
// that had grown to ~26 slot-hours per sweep with six providers no roster named becoming
// permanently resident in the shared pool — see that script's own comments for the
// arithmetic). The light pool gets what is left, Cap - HeavyCap, carved OUT of the total
// rather than added beside it, for the same reason ingest-slot.sh's own split works that
// way: a reservation added on top would raise real concurrency by however many slots it
// holds, silently, on a host where the crawl fleet — not this process — is the scarce
// resource being protected.
const DefaultHeavyCap = 5

// DefaultGrace is how long past its own timeout a claim may live before it is treated as
// dead. It covers systemd's teardown of a run it killed at TimeoutStartSec, so a run being
// cleaned up is not relaunched underneath itself.
const DefaultGrace = 2 * time.Minute

// Scheduler is one tick: read the roster, reconcile run state, claim what is due within
// the fleet's free capacity, and launch it.
//
// It is a one-shot, not a daemon. All of its state is in Postgres, so a crash costs one
// minute rather than the fleet, and `Type=oneshot` keeps it from stacking on itself.
type Scheduler struct {
	Repo     Repository
	Launcher Launcher

	// Cap bounds concurrent runs across the fleet; Grace extends a claim's life past its
	// own timeout before it is reclaimed.
	Cap   int
	Grace time.Duration

	// HeavyCap bounds how many of Cap's slots the heavy pool (Settings.IsHeavy) may hold
	// at once; 0 means "use DefaultHeavyCap", the same convention Cap itself uses. The
	// light pool gets whatever is left of Cap, never Cap plus this — see DefaultHeavyCap.
	HeavyCap int

	// Apply false is SHADOW MODE, and it is the default. The scheduler resolves, reports
	// and launches nothing, so a first deployment cannot disturb a fleet still driven by
	// the static timers.
	Apply bool
}

// Skipped is a provider the tick did not schedule, and why. Every skip is named: a fleet
// that quietly stops crawling looks exactly like a healthy one, which is how two dead
// providers went unnoticed for weeks under the script this replaces.
type Skipped struct {
	Provider string
	Reason   string
}

// PoolResult is what one concurrency pool — heavy or light — did during a tick. The two
// pools are budgeted independently (see Scheduler.HeavyCap), so a burst of long sharded
// crawls filling the heavy pool can never shrink the light pool's own budget, and this is
// where that split is reported: Cap/InFlight/Launched/WouldLaunch below stay the FLEET-WIDE
// totals across both pools, unchanged in meaning from before the split existed.
type PoolResult struct {
	Cap         int
	InFlight    int
	Launched    int
	WouldLaunch int
}

// TickResult is what one tick decided. It is returned rather than only logged so the
// worker can report it and a test can assert on it.
type TickResult struct {
	Applied   bool
	Saturated bool

	Eligible int
	// Tracked is how many providers have run state — every ENABLED one, whether or not
	// the scheduler is yet allowed to launch it.
	Tracked  int
	InFlight int
	// Reaped counts the claims released this tick because their unit had ended.
	Reaped int

	Launched    []Run
	WouldLaunch []Run

	// Heavy and Light break the totals above down by pool. Heavy.Cap + Light.Cap == Cap;
	// Heavy.InFlight + Light.InFlight == InFlight; and so on for Launched/WouldLaunch.
	Heavy PoolResult
	Light PoolResult

	// Disabled is a curator's decision, each with the reason the schema insists on.
	Disabled []Skipped
	// Unmanaged is a ROLLOUT state, not a decision: the static timer still owns this
	// provider. Kept apart from Disabled because during cutover it holds ~226 providers,
	// and mixing them would bury the two that are genuinely turned off. Removed with the
	// column in task 8.5.
	Unmanaged []string
	// Refused is a provider key the gate rejected — an unregistered or unsafe name.
	Refused []Skipped
	// Failed is a run that was claimed but could not be launched.
	Failed []Skipped
}

// Tick runs the scheduler once.
//
// It returns an error only when it could not decide anything at all — a database it cannot
// read. One provider's bad row, or one launch that fails, is recorded and stepped over: the
// per-provider timers this replaces had that isolation for free, and concentrating the
// fleet into one process is the moment to state it explicitly.
func (s Scheduler) Tick(ctx context.Context) (TickResult, error) {
	eligible, err := s.Repo.Eligible(ctx)
	if err != nil {
		return TickResult{}, fmt.Errorf("read roster: %w", err)
	}

	// An empty roster is a failed measurement, not an empty catalogue. Reconcile deletes
	// the run state of every provider absent from its list, so accepting zero here would
	// wipe the fleet's whole schedule — the stagger included — on the strength of one bad
	// read. gen-ingest-timers.sh refused on exactly this, and said why; losing that in the
	// port would be a regression dressed as a simplification.
	//
	// There is deliberately no "fewer than N providers looks wrong" floor above zero: a
	// legitimately smaller catalogue must still be schedulable, and a floor would block it
	// while catching nothing zero does not.
	if len(eligible) == 0 {
		return TickResult{Applied: s.Apply}, fmt.Errorf("the catalog lists no live board: refusing to reconcile away every provider's schedule")
	}

	result := TickResult{Applied: s.Apply, Eligible: len(eligible)}

	// TRACKED is every ENABLED provider — including the ones a static timer still owns.
	// `managed` gates the LAUNCH, not the tracking, and the difference matters twice:
	// Reconcile deletes the state of every provider absent from its list, so tracking only
	// the managed ones would run a full-table delete every minute of a cutover during which
	// `managed` defaults to false; and shadow mode previews FROM run state, so with no rows
	// the day of shadow output §8.3 exists to read would measure nothing at all.
	tracked := make([]Settings, 0, len(eligible))
	for _, settings := range eligible {
		if !settings.Enabled {
			result.Disabled = append(result.Disabled, Skipped{settings.Provider, settings.DisabledReason})
			continue
		}
		// The gate runs here as well as in the launcher. Here it keeps a bad key from ever
		// gaining run state; there it is the last point where refusing is still possible.
		// Neither alone is enough.
		if err := ValidateProviderKey(settings.Provider); err != nil {
			result.Refused = append(result.Refused, Skipped{settings.Provider, err.Error()})
			continue
		}
		if !settings.Managed {
			// Rollout only. Reported so an operator can see how much of the fleet is still
			// on the old path; removed with the column in task 8.5.
			result.Unmanaged = append(result.Unmanaged, settings.Provider)
		}
		tracked = append(tracked, settings)
	}
	result.Tracked = len(tracked)

	unreconciled, err := s.Repo.Reconcile(ctx, tracked)
	result.Failed = append(result.Failed, unreconciled...)
	if err != nil {
		return result, fmt.Errorf("reconcile run state: %w", err)
	}

	// Reap BEFORE measuring capacity. A launched run's transient unit finishes and tells
	// nobody — cmd/ingest knows nothing about the scheduler — so without this the claim
	// set at claim time would be cleared by nothing, every successful run would occupy a
	// slot forever, and the fleet would saturate for good after Cap launches with every
	// check green. Reaping after the measurement would be almost as bad: a tick that just
	// freed nine slots would still refuse to launch until the next minute.
	//
	// It runs in shadow mode too. That is the mode the fleet sits in for a full day, and a
	// shadow run whose in-flight count only ever grows measures a saturation that is not
	// real.
	//
	// Reaped and counted SEPARATELY by pool: the two pools are budgeted independently
	// below, and a heavy provider running long must never eat into the light tail's own
	// budget, nor the reverse.
	runningHeavy, runningLight, err := s.reap(ctx, &result)
	if err != nil {
		return result, err
	}
	result.InFlight = runningHeavy + runningLight
	result.Heavy.InFlight = runningHeavy
	result.Light.InFlight = runningLight

	// The light pool's cap is what is LEFT of Cap after the heavy reservation — carved OUT
	// of the total, never added beside it, so this split cannot raise real fleet
	// concurrency past Cap. heavyCap itself is clamped to Cap first: maxRuns below is a
	// sanity ceiling (1000), not the fleet's real cap, so a misconfigured HeavyCap > Cap
	// would otherwise let the heavy pool alone claim past Cap while lightCap merely floors
	// at zero — the split existing to PROTECT the fleet cap must not become a way around it.
	heavyCap := clamp(s.heavyCap(), 0, s.cap())
	lightCap := clamp(s.cap()-heavyCap, 0, maxRuns)
	result.Heavy.Cap = heavyCap
	result.Light.Cap = lightCap

	heavyBudget := clamp(heavyCap-runningHeavy, 0, maxRuns)
	lightBudget := clamp(lightCap-runningLight, 0, maxRuns)

	if heavyBudget <= 0 && lightBudget <= 0 {
		// Claim nothing from either pool, so every due row stays claimable for the next
		// tick. Advancing a due time here would silently skip a cycle rather than defer
		// it. A single pool being full is NOT fleet saturation — that pool simply claims
		// zero this tick while the other pool, if it has room, still gets its own runs;
		// that is the whole point of the split.
		result.Saturated = true
		return result, nil
	}

	if !s.Apply {
		heavyPreview, err := s.Repo.PreviewDue(ctx, true, heavyBudget, s.grace())
		if err != nil {
			return result, fmt.Errorf("preview due heavy runs: %w", err)
		}
		lightPreview, err := s.Repo.PreviewDue(ctx, false, lightBudget, s.grace())
		if err != nil {
			return result, fmt.Errorf("preview due light runs: %w", err)
		}
		result.Heavy.WouldLaunch = len(heavyPreview)
		result.Light.WouldLaunch = len(lightPreview)
		result.WouldLaunch = append(result.WouldLaunch, heavyPreview...)
		result.WouldLaunch = append(result.WouldLaunch, lightPreview...)
		return result, nil
	}

	heavyRuns, err := s.Repo.Claim(ctx, true, heavyBudget, s.grace())
	if err != nil {
		return result, fmt.Errorf("claim due heavy runs: %w", err)
	}
	lightRuns, err := s.Repo.Claim(ctx, false, lightBudget, s.grace())
	if err != nil {
		return result, fmt.Errorf("claim due light runs: %w", err)
	}

	result.Heavy.Launched = s.launch(ctx, heavyRuns, &result)
	result.Light.Launched = s.launch(ctx, lightRuns, &result)
	return result, nil
}

// launch starts every one of runs and returns how many launched successfully. A failure is
// recorded and its claim released at once rather than left to idle for the whole reclaim
// window, and does not stop the rest — the per-provider timers this replaces had that
// isolation for free.
func (s Scheduler) launch(ctx context.Context, runs []Run, result *TickResult) int {
	launched := 0
	for _, run := range runs {
		if err := s.Launcher.Launch(ctx, run); err != nil {
			result.Failed = append(result.Failed, Skipped{run.Provider, err.Error()})
			// Release the claim now rather than letting it sit until timeout + grace.
			// The scheduler already knows this run is not happening; idling the shard for
			// an hour over a known error would be a second failure on top of the first.
			if relErr := s.Repo.RecordFinish(ctx, run.Provider, run.Shard, launchFailedExitCode, err.Error()); relErr != nil {
				result.Failed = append(result.Failed, Skipped{run.Provider, "release claim: " + relErr.Error()})
			}
			continue
		}
		result.Launched = append(result.Launched, run)
		launched++
	}
	return launched
}

// reap asks the service manager about every claimed run in BOTH pools, records the ones
// that have ended, and returns how many are genuinely still executing in each — heavy and
// light are budgeted independently, so the counts that feed those two budgets must be
// counted independently too.
//
// One unreadable unit must not cost the whole tick: it is reported and its claim left
// alone, which the reclaim window then handles on its own timescale. Stopping here would
// turn one odd unit into a stopped fleet.
func (s Scheduler) reap(ctx context.Context, result *TickResult) (runningHeavy, runningLight int, err error) {
	heavyClaimed, err := s.Repo.InFlightRuns(ctx, true)
	if err != nil {
		return 0, 0, fmt.Errorf("list in-flight heavy runs: %w", err)
	}
	lightClaimed, err := s.Repo.InFlightRuns(ctx, false)
	if err != nil {
		return 0, 0, fmt.Errorf("list in-flight light runs: %w", err)
	}

	return s.reapPool(ctx, heavyClaimed, result), s.reapPool(ctx, lightClaimed, result), nil
}

// reapPool is one pool's half of reap: ask the service manager about every run in claimed,
// record the ones that ended, and return how many are still running.
func (s Scheduler) reapPool(ctx context.Context, claimed []Run, result *TickResult) int {
	running := 0
	for _, run := range claimed {
		outcome, err := s.Launcher.Finished(ctx, run)
		if err != nil {
			result.Failed = append(result.Failed, Skipped{run.Provider, "read run status: " + err.Error()})
			running++ // Unknown, so assume it is alive: over-counting costs a slot, under-counting double-launches.
			continue
		}
		if !outcome.Done {
			running++
			continue
		}
		if err := s.Repo.RecordFinish(ctx, run.Provider, run.Shard, outcome.ExitCode, outcome.Detail); err != nil {
			result.Failed = append(result.Failed, Skipped{run.Provider, "record finish: " + err.Error()})
			running++
			continue
		}
		result.Reaped++
	}
	return running
}

// launchFailedExitCode marks a run that never started, so last_exit_code distinguishes it
// from a crawl that ran and failed. 126 is the shell's "command found but not executable",
// which is the closest existing convention for "could not be started".
const launchFailedExitCode = 126

func (s Scheduler) cap() int {
	if s.Cap > 0 {
		return s.Cap
	}
	return DefaultCap
}

func (s Scheduler) heavyCap() int {
	if s.HeavyCap > 0 {
		return s.HeavyCap
	}
	return DefaultHeavyCap
}

func (s Scheduler) grace() time.Duration {
	if s.Grace > 0 {
		return s.Grace
	}
	return DefaultGrace
}
