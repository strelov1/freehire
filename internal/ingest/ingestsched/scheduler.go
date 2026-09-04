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
const DefaultCap = 10

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
	inFlight, err := s.reap(ctx, &result)
	if err != nil {
		return result, err
	}
	result.InFlight = inFlight

	budget := s.cap() - inFlight
	if budget <= 0 {
		// Claim nothing, so every due row stays claimable for the next tick. Advancing a
		// due time here would silently skip a cycle rather than defer it.
		result.Saturated = true
		return result, nil
	}

	if !s.Apply {
		result.WouldLaunch, err = s.Repo.PreviewDue(ctx, budget, s.grace())
		if err != nil {
			return result, fmt.Errorf("preview due runs: %w", err)
		}
		return result, nil
	}

	runs, err := s.Repo.Claim(ctx, budget, s.grace())
	if err != nil {
		return result, fmt.Errorf("claim due runs: %w", err)
	}

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
	}
	return result, nil
}

// reap asks the service manager about every claimed run, records the ones that have ended,
// and returns how many are genuinely still executing.
//
// One unreadable unit must not cost the whole tick: it is reported and its claim left
// alone, which the reclaim window then handles on its own timescale. Stopping here would
// turn one odd unit into a stopped fleet.
func (s Scheduler) reap(ctx context.Context, result *TickResult) (int, error) {
	claimed, err := s.Repo.InFlightRuns(ctx)
	if err != nil {
		return 0, fmt.Errorf("list in-flight runs: %w", err)
	}

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
	return running, nil
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

func (s Scheduler) grace() time.Duration {
	if s.Grace > 0 {
		return s.Grace
	}
	return DefaultGrace
}
