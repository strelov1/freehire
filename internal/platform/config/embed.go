package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Embed holds the queue-drain tuning knobs for the incremental embedding worker (cmd/embed).
// The backend is configured via LoadEmbedClient.
type Embed struct {
	BatchSize    int           // claim wave + embed/persist batch size (one TEI call set + one Postgres transaction per wave)
	LeaseSeconds int           // how long a claim is held before it can be reclaimed
	MaxAttempts  int           // failed attempts before an entry is dead-lettered
	CallTimeout  time.Duration // bounds a single batch's embed-and-persist (or per-item fallback) operation
	// MaxRun bounds a whole run (EMBED_MAX_RUN_SECONDS; 0 = unbounded): no wave is claimed that
	// CallTimeout could carry past it, so the run exits before its unit's TimeoutStartSec.
	MaxRun time.Duration
}

// LoadEmbed reads the worker's tuning from the environment, all optional. EMBED_BATCH_SIZE is
// the wave size; EMBED_CONCURRENCY (LoadEmbedClient) chunks the embed calls inside each batch.
// The one failure is an EMBED_MAX_RUN_SECONDS that is set but unusable: read as unbounded it
// would look like the knob working.
func LoadEmbed() (Embed, error) {
	e := Embed{
		BatchSize:    envInt("EMBED_BATCH_SIZE", 500),
		LeaseSeconds: envInt("EMBED_LEASE_SECONDS", 300),
		MaxAttempts:  envInt("EMBED_MAX_ATTEMPTS", 3),
		// Generous ceiling (same as search-drain): a slow TEI backend or a large batch's DB
		// transaction needs room before Runner.skipOnContextEnd treats a batch as failed.
		CallTimeout: time.Duration(envInt("EMBED_CALL_TIMEOUT_SECONDS", 600)) * time.Second,
	}
	// A non-positive size would make the claim's LIMIT 0 or negative; floor it.
	if e.BatchSize < 1 {
		e.BatchSize = 1
	}
	// The lease must outlast one batch, or an in-flight batch is re-claimed mid-embed and a
	// lease of 0 burns the retry budget in a tight loop. Floor it to the per-call timeout.
	if floor := int(e.CallTimeout.Seconds()); e.LeaseSeconds < floor {
		e.LeaseSeconds = floor
	}
	if raw := strings.TrimSpace(os.Getenv("EMBED_MAX_RUN_SECONDS")); raw != "" {
		secs, err := strconv.Atoi(raw)
		if err != nil || secs <= 0 {
			return Embed{}, fmt.Errorf("EMBED_MAX_RUN_SECONDS=%q: want a positive number of seconds", raw)
		}
		e.MaxRun = time.Duration(secs) * time.Second
		// A wave starts only while elapsed+CallTimeout fits in MaxRun, so a smaller budget would
		// claim nothing and exit green.
		if e.MaxRun <= e.CallTimeout {
			return Embed{}, fmt.Errorf("EMBED_MAX_RUN_SECONDS=%d must exceed EMBED_CALL_TIMEOUT_SECONDS (%s), or no wave can start",
				secs, e.CallTimeout)
		}
	}
	return e, nil
}

// EmbedClient holds the embedding-backend connection settings, kept here so internal/search
// stays env-free. All optional: URL defaults to the host2 TEI, APIKey to none, Concurrency to 1.
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
