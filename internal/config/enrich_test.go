package config

import (
	"strings"
	"testing"
)

func TestLoadEnrich_missingRequiredFailsFast(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")

	_, err := LoadEnrich()
	if err == nil {
		t.Fatal("expected error when LLM_* are unset, got nil")
	}
	for _, want := range []string{"LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should name missing %s", err.Error(), want)
		}
	}
}

func TestLoadEnrich_namesOnlyTheMissingOne(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://gateway:4000/v1")
	t.Setenv("LLM_API_KEY", "sk-test")
	t.Setenv("LLM_MODEL", "")

	_, err := LoadEnrich()
	if err == nil {
		t.Fatal("expected error when LLM_MODEL is unset")
	}
	if !strings.Contains(err.Error(), "LLM_MODEL") {
		t.Errorf("error %q should name LLM_MODEL", err.Error())
	}
	if strings.Contains(err.Error(), "LLM_BASE_URL") || strings.Contains(err.Error(), "LLM_API_KEY") {
		t.Errorf("error %q should not name the set vars", err.Error())
	}
}

// Langfuse tracing is optional: the required LLM_* must still be set, but a worker must load
// fine with no Langfuse vars at all.
//
// What config is responsible for is CARRYING the three values through, whole or empty. Whether
// tracing switches on is internal/llm's decision (NewTracer returns nil unless all three are
// present) — Enrich used to carry a LangfuseEnabled() of its own, which nothing outside its own
// test called: a second answer to a question the library already owned.
func TestLoadEnrich_langfuseIsOptionalAndCarriedThrough(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://gateway:4000/v1")
	t.Setenv("LLM_API_KEY", "sk-test")
	t.Setenv("LLM_MODEL", "qwen2.5-72b")

	t.Setenv("LANGFUSE_BASE_URL", "")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "")
	t.Setenv("LANGFUSE_SECRET_KEY", "")
	got, err := LoadEnrich()
	if err != nil {
		t.Fatalf("LoadEnrich with no Langfuse vars: %v", err)
	}
	if got.LangfuseBaseURL != "" || got.LangfusePublicKey != "" || got.LangfuseSecretKey != "" {
		t.Errorf("Langfuse fields = %+v, want empty", got.LLM)
	}

	// A partial configuration still loads — refusing here would make optional observability
	// able to stop a worker.
	t.Setenv("LANGFUSE_BASE_URL", "https://us.cloud.langfuse.com")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "pk-lf-x")
	if _, err := LoadEnrich(); err != nil {
		t.Fatalf("LoadEnrich with partial Langfuse config: %v", err)
	}

	t.Setenv("LANGFUSE_SECRET_KEY", "sk-lf-y")
	got, err = LoadEnrich()
	if err != nil {
		t.Fatalf("LoadEnrich: %v", err)
	}
	if got.LangfuseBaseURL != "https://us.cloud.langfuse.com" ||
		got.LangfusePublicKey != "pk-lf-x" || got.LangfuseSecretKey != "sk-lf-y" {
		t.Errorf("Langfuse fields not populated: %+v", got)
	}
}

func TestLoadEnrich_defaultsAndOverrides(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://gateway:4000/v1")
	t.Setenv("LLM_API_KEY", "sk-test")
	t.Setenv("LLM_MODEL", "qwen2.5-72b")

	got, err := LoadEnrich()
	if err != nil {
		t.Fatalf("LoadEnrich: %v", err)
	}
	if got.BaseURL != "http://gateway:4000/v1" || got.Model != "qwen2.5-72b" {
		t.Errorf("unexpected config: %+v", got)
	}
	// Tunables fall back to conservative defaults.
	if got.Concurrency != 4 || got.LeaseSeconds != 300 || got.MaxAttempts != 3 {
		t.Errorf("defaults wrong: concurrency=%d lease=%d max=%d", got.Concurrency, got.LeaseSeconds, got.MaxAttempts)
	}

	t.Setenv("ENRICH_CONCURRENCY", "8")
	got, err = LoadEnrich()
	if err != nil {
		t.Fatalf("LoadEnrich: %v", err)
	}
	if got.Concurrency != 8 {
		t.Errorf("concurrency override = %d, want 8", got.Concurrency)
	}

	// A non-positive concurrency would make the claim's LIMIT 0 — the worker would
	// silently enrich nothing while the cron looks healthy. Clamp it to a working floor.
	for _, bad := range []string{"0", "-3"} {
		t.Setenv("ENRICH_CONCURRENCY", bad)
		got, err = LoadEnrich()
		if err != nil {
			t.Fatalf("LoadEnrich: %v", err)
		}
		if got.Concurrency != 1 {
			t.Errorf("ENRICH_CONCURRENCY=%s clamped to %d, want 1", bad, got.Concurrency)
		}
	}
}
