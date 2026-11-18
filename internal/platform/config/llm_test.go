package config

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// Require names EVERY missing setting, not the first: a worker whose whole job is to spend LLM
// calls should be fixable in one run rather than three.
func TestLLMRequireNamesEveryMissingSetting(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("LLM_TEMPERATURE", "")

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

func TestLLMRequireRejectsInvalidTemperature(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://gw/v1")
	t.Setenv("LLM_API_KEY", "k")
	t.Setenv("LLM_MODEL", "m")
	t.Setenv("LLM_TEMPERATURE", "not-a-number")

	err := LoadLLM().Require()
	if err == nil || !strings.Contains(err.Error(), "LLM_TEMPERATURE") {
		t.Fatalf("Require() error = %v, want an LLM_TEMPERATURE validation error", err)
	}
}

// Tracing is observability, not capability: it is never required.
func TestLLMRequireIgnoresTracing(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://gw/v1")
	t.Setenv("LLM_API_KEY", "k")
	t.Setenv("LLM_MODEL", "m")
	t.Setenv("LLM_TEMPERATURE", "")
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
	t.Setenv("LLM_TEMPERATURE", "")
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

func TestLLMTemperatureFromEnvironmentReachesGateway(t *testing.T) {
	var request map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{}"}}]}`)
	}))
	defer server.Close()

	t.Setenv("LLM_BASE_URL", server.URL)
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_MODEL", "flagship")
	t.Setenv("LLM_TEMPERATURE", "1")
	settings := LoadLLM().Settings("flagship")
	client, flush, err := llm.NewClient(settings, "test")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer flush()

	if _, err := client.GenerateJSON(context.Background(), "system", "user"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	var got float64
	if err := json.Unmarshal(request["temperature"], &got); err != nil {
		t.Fatalf("decode temperature %s: %v", request["temperature"], err)
	}
	if got != 1 {
		t.Errorf("wire temperature = %v, want 1", got)
	}
}

func TestLLMFeatureModelsCoversEveryServerClient(t *testing.T) {
	t.Setenv("ASSISTANT_MODEL", "a")
	t.Setenv("SEARCH_INTENT_MODEL", "s")
	t.Setenv("RESUME_EXTRACT_MODEL", "r")
	t.Setenv("SUPPORT_MODEL", "u")
	if got := strings.Join(LLMFeatureModels(), ","); got != "a,s,r,u" {
		t.Errorf("LLMFeatureModels = %s, want a,s,r,u", got)
	}
}
