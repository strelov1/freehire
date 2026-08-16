package config

import "strconv"

// Enrich holds configuration for the enrichment command. The LLM settings are
// provider-agnostic: any OpenAI-compatible endpoint (a LiteLLM gateway, a Chinese
// model provider, etc.) is reached via base URL + key + model. No vendor name or
// default model is baked in — the three LLM settings are required.
type Enrich struct {
	// LLM carries the six shared connection/tracing values. Enrich keeps only the knobs that
	// are genuinely its own below — two other workers used to load THIS type purely to reach
	// the LLM half, and inherited the ENRICH_* validation with it.
	LLM

	Concurrency  int // LLM calls in flight; also the claim wave size (keeps each wave's lease window short)
	LeaseSeconds int // how long a claim is held before it can be reclaimed
	MaxAttempts  int // failed attempts before an entry is dead-lettered

	// UpstreamGraceDays bounds failures the posting did NOT cause — a gateway error,
	// a timeout, an unreachable database. Those are bounded by how long the entry has
	// been queued rather than by MaxAttempts, because an attempt counter does not
	// measure outage duration: a claimed entry becomes re-claimable once its lease
	// (LeaseSeconds) expires, so an entry at the head of the queue burns roughly
	// twelve attempts an hour while the gateway is down. Three attempts is fifteen
	// minutes; both July 2026 LiteLLM outages ran for days, and dead-lettered 172,875
	// enrichable postings between them.
	UpstreamGraceDays int
}

// LoadEnrich reads enrichment configuration from the environment. It fails fast,
// naming every missing required LLM setting, so a misconfigured run enriches nothing.
func LoadEnrich() (Enrich, error) {
	e := Enrich{
		LLM:          LoadLLM(),
		Concurrency:  envInt("ENRICH_CONCURRENCY", 4),
		LeaseSeconds: envInt("ENRICH_LEASE_SECONDS", 300),
		MaxAttempts:  envInt("ENRICH_MAX_ATTEMPTS", 3),
		// Two weeks gives a multi-day outage a wide margin while still stopping an
		// entry the gateway will never accept.
		UpstreamGraceDays: envInt("ENRICH_UPSTREAM_GRACE_DAYS", 14),
	}

	// A non-positive concurrency would make the claim's LIMIT 0 (silently no-op) or
	// feed a negative LIMIT to Postgres; floor it so the worker always makes progress.
	if e.Concurrency < 1 {
		e.Concurrency = 1
	}

	if err := e.Require(); err != nil {
		return Enrich{}, err
	}
	return e, nil
}

func envInt(key string, fallback int) int {
	// Reuse env() for the "unset or empty -> fallback" rule; an unparseable value
	// also falls back.
	if n, err := strconv.Atoi(env(key, "")); err == nil {
		return n
	}
	return fallback
}
