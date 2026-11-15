package config

import "time"

// SearchDrain holds the queue-drain tuning knobs for the incremental facet-search worker
// (cmd/search-drain). Meilisearch is configured via the shared Settings.
type SearchDrain struct {
	BatchSize    int           // claim wave + index-push batch size (one Meili task per wave)
	LeaseSeconds int           // how long a claim is held before it can be reclaimed
	MaxAttempts  int           // failed attempts before an entry is dead-lettered
	CallTimeout  time.Duration // bounds a single batch's index operation
	// MaxPerRun independently bounds claimed entries from each queue; 0 is unbounded. The drain
	// alternates one wave per queue and stops a queue after a short claim, letting trickles
	// accumulate. Deferred waves count toward the budget without spending retry attempts. Set it
	// against measured arrival rates so it does not limit throughput.
	MaxPerRun int
}

// LoadSearchDrain reads the worker's tuning from the environment, all optional.
// SEARCH_DRAIN_BATCH_SIZE: bigger collapses more per-board pushes into fewer Meili tasks (see
// internal/search/searchdrain). MEILI_MASTER_KEY is enforced at the call site, so this never fails.
func LoadSearchDrain() SearchDrain {
	c := SearchDrain{
		// A push costs about the same regardless of size (see CallTimeout), so batch size is
		// throughput: 200 rows per push drained slower than the backlog grew, 1000 drained it.
		BatchSize:    envInt("SEARCH_DRAIN_BATCH_SIZE", 2500),
		LeaseSeconds: envInt("SEARCH_DRAIN_LEASE_SECONDS", 180),
		MaxAttempts:  envInt("SEARCH_DRAIN_MAX_ATTEMPTS", 3),
		// A single push costs 90-180s+ on a large index (Meilisearch's whole-index re-merge), almost
		// independent of batch size. Too tight a value misclassifies slow successes as failures and
		// cascades into per-item retries (see internal/search/searchdrain.Runner). This is a backstop
		// against a hung call, so keep it generous.
		CallTimeout: time.Duration(envInt("SEARCH_DRAIN_CALL_TIMEOUT_SECONDS", 600)) * time.Second,
		MaxPerRun:   envInt("SEARCH_DRAIN_MAX_PER_RUN", 0),
	}
	// A non-positive size would make the claim's LIMIT 0 or negative; floor it.
	if c.BatchSize < 1 {
		c.BatchSize = 1
	}
	// The lease must outlast one batch, or an in-flight batch is re-claimed mid-push and a lease
	// of 0 burns the retry budget in a tight loop. Floor it to the per-call timeout.
	if floor := int(c.CallTimeout.Seconds()); c.LeaseSeconds < floor {
		c.LeaseSeconds = floor
	}
	// A negative cap already reads as unbounded in outbox; normalise so logs say what it means.
	if c.MaxPerRun < 0 {
		c.MaxPerRun = 0
	}
	return c
}
