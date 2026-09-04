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

	Eligible  int
	Scheduled int
	InFlight  int

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
	schedulable := make([]Settings, 0, len(eligible))
	for _, settings := range eligible {
		switch {
		case !settings.Enabled:
			result.Disabled = append(result.Disabled, Skipped{settings.Provider, settings.DisabledReason})
		case !settings.Managed:
			// Rollout only: the static timer still owns this provider. Removed with the
			// column in task 8.5.
			result.Unmanaged = append(result.Unmanaged, settings.Provider)
		default:
			// The gate runs here as well as in the launcher. Here it keeps a bad key from
			// ever gaining run state; there it is the last point where refusing is still
			// possible. Neither alone is enough.
			if err := ValidateProviderKey(settings.Provider); err != nil {
				result.Refused = append(result.Refused, Skipped{settings.Provider, err.Error()})
				continue
			}
			schedulable = append(schedulable, settings)
		}
	}
	result.Scheduled = len(schedulable)

	if err := s.Repo.Reconcile(ctx, schedulable); err != nil {
		return result, fmt.Errorf("reconcile run state: %w", err)
	}

	inFlight, err := s.Repo.InFlight(ctx)
	if err != nil {
		return result, fmt.Errorf("count in-flight runs: %w", err)
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
