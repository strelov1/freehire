package config

import "strconv"

// Enrich holds configuration for the enrichment command. The LLM settings are provider-agnostic
// (any OpenAI-compatible endpoint); no vendor or default model is baked in, so all three are required.
type Enrich struct {
	// LLM carries the shared connection/tracing values; Enrich keeps only its own knobs.
	LLM

	Concurrency  int // LLM calls in flight; also the claim wave size (keeps each wave's lease window short)
	LeaseSeconds int // how long a claim is held before it can be reclaimed
	MaxAttempts  int // failed attempts before an entry is dead-lettered

	// UpstreamGraceDays bounds failures the posting did not cause (gateway error, timeout,
	// unreachable database) by how long the entry has been queued, not by MaxAttempts: a claimed
	// entry is re-claimable after its lease, so attempts burn fast while the gateway is down.
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
		// Wide margin for a multi-day outage, while still stopping an entry the gateway never accepts.
		UpstreamGraceDays: envInt("ENRICH_UPSTREAM_GRACE_DAYS", 14),
	}

	// A non-positive concurrency would make the claim's LIMIT 0 or negative; floor it.
	if e.Concurrency < 1 {
		e.Concurrency = 1
	}

	if err := e.Require(); err != nil {
		return Enrich{}, err
	}
	return e, nil
}

func envInt(key string, fallback int) int {
	// An unparseable value falls back.
	if n, err := strconv.Atoi(env(key, "")); err == nil {
		return n
	}
	return fallback
}
