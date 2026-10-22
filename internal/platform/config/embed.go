package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
	// MaxRun bounds a whole run (EMBED_MAX_RUN_SECONDS; 0 = unbounded): the runner claims
	// no wave that CallTimeout could carry past it, so a scheduled run exits on its own
	// before its unit's TimeoutStartSec instead of being killed mid-wave.
	MaxRun time.Duration
}

// LoadEmbed reads the worker's tuning from the environment, all optional with defaults.
// EMBED_BATCH_SIZE is the wave/batch size (bigger = fewer TEI call sets and Postgres
// round trips for a bulk backfill); EMBED_CONCURRENCY (read by LoadEmbedClient) chunks
// the embed calls inside each batch. There is no required field — unlike cmd/reindex,
// cmd/embed does not require MEILI_MASTER_KEY at all (it never touches Meilisearch). The
// one way it fails is an EMBED_MAX_RUN_SECONDS that is set but unusable: read as
// "unbounded" it would look exactly like the knob working, until the unit timeout kills
// the run again.
func LoadEmbed() (Embed, error) {
	e := Embed{
		BatchSize:    envInt("EMBED_BATCH_SIZE", 500),
		LeaseSeconds: envInt("EMBED_LEASE_SECONDS", 300),
		MaxAttempts:  envInt("EMBED_MAX_ATTEMPTS", 3),
		// 600s, matching SEARCH_DRAIN_CALL_TIMEOUT_SECONDS historically: search-drain still
		// pushes into a Meili index whose cost is a fixed whole-index re-merge, the failure
		// class that timeout was raised to 600s to absorb (see internal/platform/config/
		// search_drain.go). cmd/embed no longer pushes into any search index — its own batch
		// is TEI calls plus one Postgres transaction — but the same generous ceiling is kept
		// here too: a slow TEI backend or a large batch's DB transaction both deserve room
		// before Runner.skipOnContextEnd treats a batch as merely slow rather than failed.
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
	if raw := strings.TrimSpace(os.Getenv("EMBED_MAX_RUN_SECONDS")); raw != "" {
		secs, err := strconv.Atoi(raw)
		if err != nil || secs <= 0 {
			return Embed{}, fmt.Errorf("EMBED_MAX_RUN_SECONDS=%q: want a positive number of seconds", raw)
		}
		e.MaxRun = time.Duration(secs) * time.Second
		// The runner starts a wave only while elapsed+CallTimeout fits in MaxRun, so a
		// budget no larger than one CallTimeout would claim nothing and exit green forever.
		if e.MaxRun <= e.CallTimeout {
			return Embed{}, fmt.Errorf("EMBED_MAX_RUN_SECONDS=%d must exceed EMBED_CALL_TIMEOUT_SECONDS (%s), or no wave can start",
				secs, e.CallTimeout)
		}
	}
	return e, nil
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

// SemanticSearch configures the API server's opt-in semantic search for agents
// (openspec/changes/agent-semantic-search). The query is embedded against EMBED_URL /
// EMBED_API_KEY — the same backend and model the catalogue was embedded with, or the
// query lands in another vector space. Unlike EmbedClient, an empty URL here does NOT
// mean "the default TEI": it means semantic search is unavailable and every request that
// asks for it is answered keyword-only.
type SemanticSearch struct {
	URL    string // EMBED_URL
	APIKey string // EMBED_API_KEY
	// QueryTimeout bounds the one embed call a search makes (EMBED_QUERY_TIMEOUT_MS,
	// default 400). Measured 2026-10-18: 0.09s against an idle TEI, 8-10s while a bulk
	// embed drained on it — the timeout is what turns the second case into a fallback
	// instead of a stalled search.
	QueryTimeout time.Duration
	// ScoreThreshold is the ranking-score floor of a hybrid query
	// (SEMANTIC_SCORE_THRESHOLD, default 0 = none). It is set from sampled agent queries
	// once the vectors are live, not guessed here.
	ScoreThreshold float64
}

// LoadSemanticSearch reads the semantic-search settings. A value that is set but
// unreadable is an error rather than a fallback: a mistyped threshold would otherwise
// look exactly like a tuned one.
func LoadSemanticSearch() (SemanticSearch, error) {
	s := SemanticSearch{
		URL:          os.Getenv("EMBED_URL"),
		APIKey:       os.Getenv("EMBED_API_KEY"),
		QueryTimeout: 400 * time.Millisecond,
	}
	if raw := strings.TrimSpace(os.Getenv("EMBED_QUERY_TIMEOUT_MS")); raw != "" {
		ms, err := strconv.Atoi(raw)
		if err != nil || ms <= 0 {
			return SemanticSearch{}, fmt.Errorf("EMBED_QUERY_TIMEOUT_MS=%q: want a positive number of milliseconds", raw)
		}
		s.QueryTimeout = time.Duration(ms) * time.Millisecond
	}
	if raw := strings.TrimSpace(os.Getenv("SEMANTIC_SCORE_THRESHOLD")); raw != "" {
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil || f < 0 || f > 1 {
			return SemanticSearch{}, fmt.Errorf("SEMANTIC_SCORE_THRESHOLD=%q: want a number between 0 and 1", raw)
		}
		s.ScoreThreshold = f
	}
	return s, nil
}
