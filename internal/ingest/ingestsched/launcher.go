package ingestsched

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Outcome is how a launched run ended, as read back from the service manager.
type Outcome struct {
	// Done is false while the run is still executing.
	Done bool
	// ExitCode is the crawl's own status. It is 0 for a run whose unit has vanished,
	// which is what a SUCCESSFUL transient unit does on its own.
	ExitCode int
	// Detail is the service manager's word for how it ended ("exit-code", "timeout",
	// "signal"), kept because "the run failed" and "the run was killed at its budget"
	// need different responses.
	Detail string
}

// Launcher starts one claimed run and later reports whether it has ended. It is a port
// because `systemd-run` exists only on the crawl host: the claim, due and reclaim logic —
// the part worth testing — must not need a service manager to be exercised.
type Launcher interface {
	Launch(ctx context.Context, run Run) error
	// Finished reports whether a launched run has ended, and how.
	//
	// This exists because a transient unit finishes and tells NOBODY: cmd/ingest knows
	// nothing about the scheduler. Without it, claimed_at would be set at claim and
	// cleared by nothing, so every successful run would permanently occupy a slot and the
	// fleet would saturate for good after Cap launches — with every check green.
	Finished(ctx context.Context, run Run) (Outcome, error)
}

// SystemdLauncher starts each run as a TRANSIENT systemd unit.
//
// Transient rather than a static template, for two reasons the fleet actually has. The
// per-run timeout varies (3000s for most providers, 4500s for the per-posting-detail ones)
// and a template can carry only one value. And a transient unit outlives the scheduler
// invocation that created it, so a one-minute `Type=oneshot` scheduler never holds a
// 40-minute crawl open.
//
// What does NOT change from the units this replaces: the crawl runs as the unprivileged
// service account, reads the same environment file, and is accounted in its own cgroup.
// Only the scheduler is privileged, and only so it may create the transient unit.
type SystemdLauncher struct {
	// IngestBinary is the path to cmd/ingest on the host — under the active blue/green
	// release, matching the units this replaces.
	IngestBinary string
	WorkingDir   string
	EnvFile      string
	// RunAs is the unprivileged account the crawl drops to.
	RunAs string

	// exec is the seam the tests use. nil means really run the command.
	exec func(ctx context.Context, name string, args ...string) error
	// show reads a unit's properties. nil means really ask systemctl.
	show func(ctx context.Context, unit string, properties ...string) (map[string]string, error)
}

// NewSystemdLauncher builds the launcher used on the host.
func NewSystemdLauncher(ingestBinary, workingDir, envFile, runAs string) SystemdLauncher {
	return SystemdLauncher{
		IngestBinary: ingestBinary,
		WorkingDir:   workingDir,
		EnvFile:      envFile,
		RunAs:        runAs,
	}
}

// Launch starts run as a transient unit.
//
// The provider key is validated HERE, at the last point where refusing is still possible,
// and not only where it was read. Board rows can originate from crowdsourced submissions,
// and this method turns one into both an argv element and a systemd unit name.
func (l SystemdLauncher) Launch(ctx context.Context, run Run) error {
	if err := ValidateProviderKey(run.Provider); err != nil {
		return fmt.Errorf("refusing to launch: %w", err)
	}

	args := []string{
		"--unit=" + l.unitName(run),
		"--description=freehire ingest " + run.Provider + " " + shardLabel(run),
		// Deliberately NOT --collect. systemd already garbage-collects a SUCCESSFUL
		// transient unit; --collect would collect failed ones too, erasing the exit code
		// before Finished could read it and making "it succeeded" and "it failed" the same
		// answer. A failed unit's name is freed by reset-failed in Finished instead.
		"--property=Type=oneshot",
		"--property=TimeoutStartSec=" + strconv.Itoa(int(run.RunTimeout.Seconds())),
		// The weights the per-provider units carried: a crawl yields to the API.
		"--property=CPUWeight=40",
		"--property=IOWeight=40",
		"--property=WorkingDirectory=" + l.WorkingDir,
		"--property=EnvironmentFile=" + l.EnvFile,
		"--uid=" + l.RunAs,
		l.IngestBinary,
		run.Provider,
	}
	if run.Shards > 1 {
		// An unsharded provider gets NO selector rather than --shard=1/1: cmd/ingest
		// already reads a missing selector as "crawl everything", and two spellings of
		// the same instruction is one more pair that can disagree.
		args = append(args, fmt.Sprintf("--shard=%d/%d", run.Shard, run.Shards))
	}

	return l.execute(ctx, "systemd-run", args...)
}

// Finished asks systemd how the run's unit is doing.
//
// A unit that does not exist finished SUCCESSFULLY — systemd garbage-collects a successful
// transient unit on its own, while a failed one lingers. That asymmetry is the whole reason
// Launch does not pass --collect.
func (l SystemdLauncher) Finished(ctx context.Context, run Run) (Outcome, error) {
	unit := l.unitName(run)
	props, err := l.properties(ctx, unit, "LoadState", "ActiveState", "Result", "ExecMainStatus")
	if err != nil {
		return Outcome{}, fmt.Errorf("read %s: %w", unit, err)
	}

	if state := props["LoadState"]; state == "not-found" || state == "" {
		return Outcome{Done: true}, nil
	}
	switch props["ActiveState"] {
	case "activating", "active", "deactivating", "reloading":
		return Outcome{Done: false}, nil
	}

	code := exitStatus(props["ExecMainStatus"])
	detail := props["Result"]
	// A failed unit holds its name until it is reset, and the next launch of this shard
	// would be refused with "unit already exists" — the fleet would stop one provider at a
	// time. Resetting right after the status has been read keeps the failure legible AND
	// the name free.
	if err := l.execute(ctx, "systemctl", "reset-failed", unit); err != nil {
		// The status was already read, so the run IS finished; failing to tidy up must not
		// hold the claim open. Carry it in Detail rather than losing it.
		detail += " (reset-failed: " + err.Error() + ")"
	}
	return Outcome{Done: true, ExitCode: code, Detail: detail}, nil
}

func (l SystemdLauncher) properties(ctx context.Context, unit string, names ...string) (map[string]string, error) {
	if l.show != nil {
		return l.show(ctx, unit, names...)
	}

	args := []string{"show", unit}
	for _, n := range names {
		args = append(args, "--property="+n)
	}
	// `systemctl show` answers 0 with LoadState=not-found for a unit that never existed,
	// so a non-zero status here is a real failure to ask, not a missing unit.
	out, err := exec.CommandContext(ctx, "systemctl", args...).Output()
	if err != nil {
		return nil, err
	}

	props := make(map[string]string, len(names))
	for _, line := range strings.Split(string(out), "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			props[key] = value
		}
	}
	return props, nil
}

func (l SystemdLauncher) execute(ctx context.Context, name string, args ...string) error {
	if l.exec != nil {
		return l.exec(ctx, name, args...)
	}
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, out)
	}
	return nil
}

// unitName is derived from the same provider string that selects the boards, which is the
// structural form of the habr_career fix: there is no second spelling left to drift.
func (l SystemdLauncher) unitName(run Run) string {
	name := "freehire-ingest-run-" + run.Provider
	if run.Shards > 1 {
		name += "-" + strconv.Itoa(run.Shard)
	}
	return name + ".service"
}

// maxExitStatus is the largest value a process exit status can carry.
const maxExitStatus = 255

// exitStatus parses systemd's ExecMainStatus and bounds it to a real process status.
//
// The value arrives as text, is parsed with strconv.Atoi into a platform-width int, and is
// stored in an int32 column — so an unbounded parse would let a value past int32 WRAP on
// the way to the database and record a status the run never had. Clamping rather than
// erroring because the caller already has the raw text in Result: a status outside 0-255 is
// systemd saying something other than "the process exited with", and the run is finished
// either way. Losing the claim over an odd number would be the worse failure.
// Parsed with ParseInt at the WIDTH the column holds, not with Atoi into a
// platform-width int. Atoi yields a value whose size depends on the architecture, so
// narrowing it later is a conversion no reader — and no analyser — can call safe from the
// line itself; ParseInt saturates at the requested width instead, and its result is in
// range by construction.
//
// The two parse failures mean different things and must not collapse into one. Text that is
// not a number carries no status, so it reads as 0; a number too large to hold is still a
// number, and ParseInt hands back the saturated value alongside ErrRange, so it clamps like
// any other oversized status. Treating both as 0 would record "exited cleanly" for a unit
// that said something enormous.
func exitStatus(raw string) int {
	code, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0
	}
	return clamp(int(code), 0, maxExitStatus)
}

func shardLabel(run Run) string {
	if run.Shards <= 1 {
		return "(whole)"
	}
	return fmt.Sprintf("shard %d/%d", run.Shard, run.Shards)
}
