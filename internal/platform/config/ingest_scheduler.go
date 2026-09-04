package config

import (
	"os"
	"time"
)

// IngestScheduler is cmd/ingest-scheduler's configuration.
type IngestScheduler struct {
	// Apply false is SHADOW MODE and is the DEFAULT. The first deployment lands under a
	// fleet still driven by ~279 static timers, so a scheduler that launched on install
	// would double-crawl every provider at once. Turning launches on is a deliberate
	// operator act, never a side effect of shipping.
	Apply bool

	// Cap bounds concurrent runs across the whole fleet. It replaces ingest-slot.sh's
	// flock semaphore, whose 10 was calibrated against this fleet after 8 measured short.
	Cap int

	// Grace extends a claim's life past its own timeout before it is treated as dead,
	// covering systemd's teardown of a run it killed at TimeoutStartSec.
	Grace time.Duration

	// Where a launched run finds its binary, its working directory and its environment —
	// the same three the per-provider units carried. IngestBinary points at the ACTIVE
	// blue/green release, not at a fixed colour.
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
		Grace:        time.Duration(envInt("INGEST_SCHEDULER_GRACE_SECONDS", 120)) * time.Second,
		IngestBinary: envStr("INGEST_SCHEDULER_BINARY", "/opt/freehire/src/hire-current/ingest"),
		WorkingDir:   envStr("INGEST_SCHEDULER_WORKDIR", "/opt/freehire/src/hire-current"),
		EnvFile:      envStr("INGEST_SCHEDULER_ENV_FILE", "/opt/freehire/.env"),
		RunAs:        envStr("INGEST_SCHEDULER_RUN_AS", "freehire"),
	}
	// A non-positive cap would make every tick read as saturated and the fleet would stop
	// crawling while every check stayed green — the failure mode this whole change exists
	// to make impossible. Floor it rather than trusting a typo.
	if c.Cap < 1 {
		c.Cap = 1
	}
	// Floored at one second, not at zero. Scheduler.grace() reads a zero Grace as "use
	// DefaultGrace", so returning 0 here would silently hand back two minutes and make
	// INGEST_SCHEDULER_GRACE_SECONDS=0 mean the opposite of what it says.
	if c.Grace < time.Second {
		c.Grace = time.Second
	}
	return c
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
