package config

import (
	"os"
	"time"
)

// Embed holds the tuning knobs for the incremental semantic-embedding worker
// (cmd/embed). Meilisearch and the embedding backend are configured elsewhere —
// MEILI_URL/MEILI_MASTER_KEY via the shared Settings (Load), and EMBED_URL/
// EMBED_API_KEY/EMBED_CONCURRENCY via LoadEmbedClient — so this holds only the
// queue-drain knobs, mirroring the tuning half of config.Enrich.
type Embed struct {
	BatchSize    int           // claim wave + embed/persist batch size (one TEI call set + one Postgres transaction per wave)
	LeaseSeconds int           // how long a claim is held before it can be reclaimed
	MaxAttempts  int           // failed attempts before an entry is dead-lettered
	CallTimeout  time.Duration // bounds a single batch's embed-and-persist (or per-item fallback) operation
}

// LoadEmbed reads the worker's tuning from the environment, all optional with defaults.
// EMBED_BATCH_SIZE is the wave/batch size (bigger = fewer TEI call sets and Postgres
// round trips for a bulk backfill); EMBED_CONCURRENCY (read by LoadEmbedClient) chunks
// the embed calls inside each batch. There is no required field — unlike cmd/reindex,
// cmd/embed does not require MEILI_MASTER_KEY at all (it never touches Meilisearch), so
// this never fails.
func LoadEmbed() Embed {
	e := Embed{
		BatchSize:    envInt("EMBED_BATCH_SIZE", 500),
		LeaseSeconds: envInt("EMBED_LEASE_SECONDS", 300),
		MaxAttempts:  envInt("EMBED_MAX_ATTEMPTS", 3),
		// 600s, matching SEARCH_DRAIN_CALL_TIMEOUT_SECONDS historically: search-drain still
		// pushes into a Meili index whose cost is a fixed whole-index re-merge, the failure
		// class that timeout was raised to 600s to absorb (see internal/config/
		// search_drain.go). cmd/embed no longer pushes into any search index — its own batch
		// is TEI calls plus one Postgres transaction — but the same generous ceiling is kept
		// here too: a slow TEI backend or a large batch's DB transaction both deserve room
		// before Runner.skipOnTimeout treats a batch as merely slow rather than failed.
		CallTimeout: time.Duration(envInt("EMBED_CALL_TIMEOUT_SECONDS", 600)) * time.Second,
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

// EmbedClient holds the embedding-backend connection settings an embedding search
// client is built with (cmd/embed embeds jobs). They live here rather than inside
// internal/search so the library stays env-free; cmd wires them into search.NewClient's
// WithEmbed* options. All optional: URL defaults to the host2 TEI (search's
// embedderURL), APIKey to none, Concurrency to 1.
type EmbedClient struct {
	URL         string // EMBED_URL — TEI-compatible /embed endpoint; empty = default host2 TEI
	APIKey      string // EMBED_API_KEY — bearer token for an authenticated endpoint
	Concurrency int    // EMBED_CONCURRENCY — embed calls in flight per batch (default 1)
}

// LoadEmbedClient reads the embedding-backend settings from the environment.
func LoadEmbedClient() EmbedClient {
	return EmbedClient{
		URL:         os.Getenv("EMBED_URL"),
		APIKey:      os.Getenv("EMBED_API_KEY"),
		Concurrency: envInt("EMBED_CONCURRENCY", 1),
	}
}
