package config

import "time"

// Embed holds the tuning knobs for the incremental semantic-embedding worker
// (cmd/embed). Meilisearch and the embedding backend are configured elsewhere —
// MEILI_URL/MEILI_MASTER_KEY via the shared Settings (Load), and EMBED_URL/
// EMBED_API_KEY/EMBED_CONCURRENCY inside search.NewClient — so this holds only the
// queue-drain knobs, mirroring the tuning half of config.Enrich.
type Embed struct {
	Concurrency  int           // embeds in flight; also the claim wave size (keeps each wave's lease window short)
	LeaseSeconds int           // how long a claim is held before it can be reclaimed
	MaxAttempts  int           // failed attempts before an entry is dead-lettered
	CallTimeout  time.Duration // bounds a single job's embed/index or remove operation
}

// LoadEmbed reads the worker's tuning from the environment, all optional with
// defaults. EMBED_CONCURRENCY doubles as search.NewClient's embed-batch concurrency,
// so one knob controls "how many embeds are in flight". There is no required field —
// the MEILI_MASTER_KEY requirement is enforced at the cmd/embed call site (like
// cmd/reindex), so this never fails.
func LoadEmbed() Embed {
	e := Embed{
		Concurrency:  envInt("EMBED_CONCURRENCY", 8),
		LeaseSeconds: envInt("EMBED_LEASE_SECONDS", 300),
		MaxAttempts:  envInt("EMBED_MAX_ATTEMPTS", 3),
		CallTimeout:  time.Duration(envInt("EMBED_CALL_TIMEOUT_SECONDS", 120)) * time.Second,
	}
	// A non-positive concurrency would make the claim's LIMIT 0 (silently no-op) or
	// feed a negative LIMIT to Postgres; floor it so the worker always makes progress
	// (mirrors LoadEnrich).
	if e.Concurrency < 1 {
		e.Concurrency = 1
	}
	// The lease must outlast a single entry's processing, or an in-flight entry becomes
	// re-claimable mid-embed (double work) and a lease of 0 re-claims a just-failed entry
	// in a tight loop, burning its whole retry budget in one run. Floor it to the per-call
	// timeout — the longest one entry can hold the lease.
	if floor := int(e.CallTimeout.Seconds()); e.LeaseSeconds < floor {
		e.LeaseSeconds = floor
	}
	return e
}
