package config

import (
	"os"
	"time"
)

// IngestScheduler is cmd/ingest-scheduler's configuration.
type IngestScheduler struct {
	// Apply false is shadow mode and the default: a scheduler that launched on install would
	// double-crawl providers still driven by static timers. Turning launches on is an operator act.
	Apply bool

	// Cap bounds concurrent runs across the whole fleet (replaces ingest-slot.sh's flock semaphore).
	Cap int

	// HeavyCap reserves how many of Cap's slots the heavy pool may hold; 0 means
	// ingestsched.DefaultHeavyCap. Read from the environment because it must be tuned together with
	// Cap: a Cap below the default clamped the reservation to the whole cap and starved the light pool.
	HeavyCap int

	// Grace extends a claim's life past its own timeout before it is treated as dead,
	// covering systemd's teardown of a run it killed at TimeoutStartSec.
	Grace time.Duration

	// Where a launched run finds its binary, working directory and environment. IngestBinary
	// points at the active blue/green release.
	IngestBinary string
	WorkingDir   string
	EnvFile      string
	RunAs        string
}

// LoadIngestScheduler reads the scheduler's environment.
func LoadIngestScheduler() IngestScheduler {
	c := IngestScheduler{
		Apply:        os.Getenv("INGEST_SCHEDULER_APPLY") == "1",
		Cap:          envInt("INGEST_SCHEDULER_CAP", 10),
		HeavyCap:     envInt("INGEST_SCHEDULER_HEAVY_CAP", 0),
		Grace:        time.Duration(envInt("INGEST_SCHEDULER_GRACE_SECONDS", 120)) * time.Second,
		IngestBinary: env("INGEST_SCHEDULER_BINARY", "/opt/freehire/src/hire-current/ingest"),
		WorkingDir:   env("INGEST_SCHEDULER_WORKDIR", "/opt/freehire/src/hire-current"),
		EnvFile:      env("INGEST_SCHEDULER_ENV_FILE", "/opt/freehire/.env"),
		RunAs:        env("INGEST_SCHEDULER_RUN_AS", "freehire"),
	}
	// A non-positive cap would read every tick as saturated and silently stop the fleet; floor it.
	if c.Cap < 1 {
		c.Cap = 1
	}
	// A negative reservation is a typo; 0 means "use the default", resolved in Scheduler, so the
	// upper bound is not clamped here.
	if c.HeavyCap < 0 {
		c.HeavyCap = 0
	}
	// Floored at one second: Scheduler.grace() reads 0 as DefaultGrace, which would invert a 0 setting.
	if c.Grace < time.Second {
		c.Grace = time.Second
	}
	return c
}
