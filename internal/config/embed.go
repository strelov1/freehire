package config

import "time"

// Embed holds the tuning knobs for the incremental semantic-embedding worker
// (cmd/embed). Meilisearch and the embedding backend are configured elsewhere —
// MEILI_URL/MEILI_MASTER_KEY via the shared Settings (Load), and EMBED_URL/
// EMBED_API_KEY/EMBED_CONCURRENCY inside search.NewClient — so this holds only the
// queue-drain knobs, mirroring the tuning half of config.Enrich.
type Embed struct {
	BatchSize    int           // claim wave + embed/upsert batch size (one Meili task per wave)
	LeaseSeconds int           // how long a claim is held before it can be reclaimed
	MaxAttempts  int           // failed attempts before an entry is dead-lettered
	CallTimeout  time.Duration // bounds a single batch's embed/index or remove operation
}

// LoadEmbed reads the worker's tuning from the environment, all optional with defaults.
// EMBED_BATCH_SIZE is the wave/batch size (bigger = fewer Meili tasks for a bulk
// backfill); EMBED_CONCURRENCY (read separately by search.NewClient) chunks the embed
// calls inside each batch. There is no required field — the MEILI_MASTER_KEY requirement
// is enforced at the cmd/embed call site (like cmd/reindex), so this never fails.
func LoadEmbed() Embed {
	e := Embed{
		BatchSize:    envInt("EMBED_BATCH_SIZE", 500),
		LeaseSeconds: envInt("EMBED_LEASE_SECONDS", 300),
		MaxAttempts:  envInt("EMBED_MAX_ATTEMPTS", 3),
		CallTimeout:  time.Duration(envInt("EMBED_CALL_TIMEOUT_SECONDS", 300)) * time.Second,
	}
	// A non-positive batch size would make the claim's LIMIT 0 (silently no-op) or feed a
	// negative LIMIT to Postgres; floor it so the worker always makes progress.
	if e.BatchSize < 1 {
		e.BatchSize = 1
	}
	// The lease must outlast a single batch's processing, or an in-flight batch becomes
	// re-claimable mid-embed (double work) and a lease of 0 re-claims a just-failed entry
	// in a tight loop, burning its whole retry budget in one run. Floor it to the per-call
	// timeout — the longest one batch can hold the lease.
	if floor := int(e.CallTimeout.Seconds()); e.LeaseSeconds < floor {
		e.LeaseSeconds = floor
	}
	return e
}
