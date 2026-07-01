package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Enrich holds configuration for the enrichment command. The LLM settings are
// provider-agnostic: any OpenAI-compatible endpoint (a LiteLLM gateway, a Chinese
// model provider, etc.) is reached via base URL + key + model. No vendor name or
// default model is baked in — the three LLM settings are required.
type Enrich struct {
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string

	Concurrency  int // LLM calls in flight; also the claim wave size (keeps each wave's lease window short)
	LeaseSeconds int // how long a claim is held before it can be reclaimed
	MaxAttempts  int // failed attempts before an entry is dead-lettered

	// Langfuse LLM tracing is optional observability, shared by both LLM workers.
	// Unlike the LLM settings it is never required: all three empty simply means
	// tracing is off (LangfuseEnabled reports false) and the worker runs unchanged.
	LangfuseBaseURL   string
	LangfusePublicKey string
	LangfuseSecretKey string
}

// LangfuseEnabled reports whether LLM tracing should be wired: true only when all
// three Langfuse settings are present. A partial configuration is treated as off,
// mirroring how a missing MEILI_MASTER_KEY disables search.
func (e Enrich) LangfuseEnabled() bool {
	return e.LangfuseBaseURL != "" && e.LangfusePublicKey != "" && e.LangfuseSecretKey != ""
}

// LoadEnrich reads enrichment configuration from the environment. It fails fast,
// naming every missing required LLM setting, so a misconfigured run enriches nothing.
func LoadEnrich() (Enrich, error) {
	e := Enrich{
		LLMBaseURL:   os.Getenv("LLM_BASE_URL"),
		LLMAPIKey:    os.Getenv("LLM_API_KEY"),
		LLMModel:     os.Getenv("LLM_MODEL"),
		Concurrency:  envInt("ENRICH_CONCURRENCY", 4),
		LeaseSeconds: envInt("ENRICH_LEASE_SECONDS", 300),
		MaxAttempts:  envInt("ENRICH_MAX_ATTEMPTS", 3),

		LangfuseBaseURL:   os.Getenv("LANGFUSE_BASE_URL"),
		LangfusePublicKey: os.Getenv("LANGFUSE_PUBLIC_KEY"),
		LangfuseSecretKey: os.Getenv("LANGFUSE_SECRET_KEY"),
	}

	// A non-positive concurrency would make the claim's LIMIT 0 (silently no-op) or
	// feed a negative LIMIT to Postgres; floor it so the worker always makes progress.
	if e.Concurrency < 1 {
		e.Concurrency = 1
	}

	var missing []string
	if e.LLMBaseURL == "" {
		missing = append(missing, "LLM_BASE_URL")
	}
	if e.LLMAPIKey == "" {
		missing = append(missing, "LLM_API_KEY")
	}
	if e.LLMModel == "" {
		missing = append(missing, "LLM_MODEL")
	}
	if len(missing) > 0 {
		return Enrich{}, fmt.Errorf("config: missing required env: %s", strings.Join(missing, ", "))
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
