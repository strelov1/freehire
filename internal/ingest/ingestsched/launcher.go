package ingestsched

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

// Launcher starts one claimed run. It is a port because `systemd-run` exists only on the
// crawl host: the claim, due and reclaim logic — the part worth testing — must not need a
// service manager to be exercised.
type Launcher interface {
	Launch(ctx context.Context, run Run) error
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

	// exec is the seam the tests use. nil means really run systemd-run.
	exec func(ctx context.Context, name string, args ...string) error
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
		// Garbage-collect the unit when it exits, including on failure. Without this a
		// failed run's unit keeps its name and the next launch is refused — the fleet
		// would stop one provider at a time, quietly.
		"--collect",
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

func shardLabel(run Run) string {
	if run.Shards <= 1 {
		return "(whole)"
	}
	return fmt.Sprintf("shard %d/%d", run.Shard, run.Shards)
}
