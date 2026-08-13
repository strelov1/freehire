package config

import (
	"strings"
	"testing"
)

// Require names EVERY missing setting, not the first: a worker whose whole job is to spend LLM
// calls should be fixable in one run rather than three.
func TestLLMRequireNamesEveryMissingSetting(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")

	err := LoadLLM().Require()
	if err == nil {
		t.Fatal("an entirely unset LLM config was accepted")
	}
	for _, key := range []string{"LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not name %s", err, key)
		}
	}
}

// Tracing is observability, not capability: it is never required.
func TestLLMRequireIgnoresTracing(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://gw/v1")
	t.Setenv("LLM_API_KEY", "k")
	t.Setenv("LLM_MODEL", "m")
	t.Setenv("LANGFUSE_BASE_URL", "")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "")
	t.Setenv("LANGFUSE_SECRET_KEY", "")

	if err := LoadLLM().Require(); err != nil {
		t.Errorf("Require rejected a config whose only gap is tracing: %v", err)
	}
}

// The server's policy is the opposite of a worker's and must stay that way: an unconfigured LLM
// degrades the AI layer rather than refusing to boot, so Load must not validate.
func TestLoadLeavesTheLLMUnvalidated(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))

	s := Load()
	if s.BaseURL != "" || s.Model != "" {
		t.Errorf("LLM = %+v, want empty", s.LLM)
	}
}

// Settings takes the model as an argument because two clients share one connection and differ
// only by it — the assistant runs on ASSISTANT_MODEL where the rest run on LLM_MODEL.
func TestLLMSettingsTakesTheModelFromTheCaller(t *testing.T) {
	l := LLM{BaseURL: "http://gw/v1", APIKey: "k", Model: "cheap-json", LangfusePublicKey: "pk"}

	if got := l.Settings("tool-calling"); got.Model != "tool-calling" || got.BaseURL != "http://gw/v1" ||
		got.APIKey != "k" || got.LangfusePublicKey != "pk" {
		t.Errorf("Settings = %+v, want the caller's model over the shared connection", got)
	}
}
