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

func TestLoadEnrich_langfuseOptionalAndGated(t *testing.T) {
	// Langfuse tracing is optional: the required LLM_* must still be set, but the
	// worker must load fine with no Langfuse vars and report tracing disabled.
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
	if got.LangfuseEnabled() {
		t.Error("LangfuseEnabled() = true with no vars set, want false")
	}

	// Only two of three set is still disabled — all three are required.
	t.Setenv("LANGFUSE_BASE_URL", "https://us.cloud.langfuse.com")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "pk-lf-x")
	got, err = LoadEnrich()
	if err != nil {
		t.Fatalf("LoadEnrich: %v", err)
	}
	if got.LangfuseEnabled() {
		t.Error("LangfuseEnabled() = true with secret key missing, want false")
	}

	// All three set: fields populated and tracing enabled.
	t.Setenv("LANGFUSE_SECRET_KEY", "sk-lf-y")
	got, err = LoadEnrich()
	if err != nil {
		t.Fatalf("LoadEnrich: %v", err)
	}
	if !got.LangfuseEnabled() {
		t.Fatal("LangfuseEnabled() = false with all three set, want true")
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
	if got.LLMBaseURL != "http://gateway:4000/v1" || got.LLMModel != "qwen2.5-72b" {
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
