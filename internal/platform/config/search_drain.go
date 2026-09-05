package config

import "time"

// SearchDrain holds the tuning knobs for the incremental facet-search worker
// (cmd/search-drain). Meilisearch is configured elsewhere — MEILI_URL/
// MEILI_MASTER_KEY via the shared Settings (Load) — so this holds only the
// queue-drain knobs, mirroring config.Embed.
type SearchDrain struct {
	BatchSize    int           // claim wave + index-push batch size (one Meili task per wave)
	LeaseSeconds int           // how long a claim is held before it can be reclaimed
	MaxAttempts  int           // failed attempts before an entry is dead-lettered
	CallTimeout  time.Duration // bounds a single batch's index operation
}

// LoadSearchDrain reads the worker's tuning from the environment, all optional with
// defaults. SEARCH_DRAIN_BATCH_SIZE is the wave/batch size — bigger collapses more
// per-board pushes into fewer, fatter Meili tasks, which is the whole point of this
// worker (see internal/search/searchdrain). There is no required field — the
// MEILI_MASTER_KEY requirement is enforced at the cmd/search-drain call site (like
// cmd/embed), so this never fails.
func LoadSearchDrain() SearchDrain {
	c := SearchDrain{
		// 2500, measured rather than guessed. A push costs what it costs almost
		// regardless of size (see CallTimeout below), so the batch is throughput:
		// on prod 2026-09-04, with a 136k backlog, 200 rows per push drained a NET
		// 38/min and the queue grew; 1000 drained 1234/min and it fell. The backlog
		// had reached 136k with entries two days old, and no new vacancy was
		// reaching search — a batch of 200 was most of why.
		//
		// The 200 was not a considered value either. It was set on the host on
		// 2026-08-05 as "generous timeout + small batch while re-validating" after
		// an incident whose actual cause was the client timeout (below), and it
		// stayed a month after the re-validating ended.
		BatchSize:    envInt("SEARCH_DRAIN_BATCH_SIZE", 2500),
		LeaseSeconds: envInt("SEARCH_DRAIN_LEASE_SECONDS", 180),
		MaxAttempts:  envInt("SEARCH_DRAIN_MAX_ATTEMPTS", 3),
		// Prod measurement (2026-08-05, ~2.7M-doc jobs index): a single push costs
		// 90-180s+, dominated by Meilisearch's whole-index re-merge, almost
		// independent of batch size. 120s was too tight — it misclassified normal,
		// successful-but-slow batches as failed, which (before the timeout/fallback
		// split in internal/search/searchdrain.Runner) cascaded into per-item retries and
		// caused a real outage. This is a genuine backstop against a truly hung call,
		// not a normal-operation tripwire — set it generously.
		CallTimeout: time.Duration(envInt("SEARCH_DRAIN_CALL_TIMEOUT_SECONDS", 600)) * time.Second,
	}
	// A non-positive batch size would make the claim's LIMIT 0 (silently no-op) or
	// feed a negative LIMIT to Postgres; floor it so the worker always makes progress.
	if c.BatchSize < 1 {
		c.BatchSize = 1
	}
	// The lease must outlast a single batch's processing, or an in-flight batch
	// becomes re-claimable mid-push (double work) and a lease of 0 re-claims a
	// just-failed entry in a tight loop, burning its whole retry budget in one run.
	// Floor it to the per-call timeout — the longest one batch can hold the lease.
	if floor := int(c.CallTimeout.Seconds()); c.LeaseSeconds < floor {
		c.LeaseSeconds = floor
	}
	return c
}
