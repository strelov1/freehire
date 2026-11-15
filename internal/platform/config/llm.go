package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// LLM holds the provider-agnostic connection triple, an optional sampling temperature, and the
// optional Langfuse tracing triple, shared by the server's Settings and the workers. What differs
// between callers is policy: the server degrades when the LLM is unconfigured, a worker that
// spends LLM calls fails fast, expressed by whether it calls Require.
//
// llm.Settings is deliberately not folded in: it is the env-free shape the library takes, so
// internal/platform/llm depends on no config package. Settings() is the one mapping.
type LLM struct {
	BaseURL string
	APIKey  string
	Model   string
	// Temperature is LLM_TEMPERATURE. Nil leaves langchaingo's own behaviour, which sends
	// "temperature":0 for a non-reasoning model id rather than omitting the field; a value
	// applies to every client built from this config in the process.
	Temperature *float64

	// StrictSchema says whether the model behind this gateway honours `response_format: json_schema`.
	// Default true. A deployment fact, not a vendor name: some models (e.g. glm-4.7-flash) answer a
	// strict schema with invented objects but follow `json_object` correctly.
	//
	// Turning it off keeps the contract: every caller spells its shape out in the prompt (see
	// internal/ai/enrich/langchain.go buildSystemPrompt) and validates the reply.
	StrictSchema bool

	// Langfuse tracing is optional. All three empty, or a partial configuration, means off.
	LangfuseBaseURL   string
	LangfusePublicKey string
	LangfuseSecretKey string
}

// LoadLLM reads the LLM settings permissively. Nothing is required here — see Require.
func LoadLLM() LLM {
	return LLM{
		BaseURL:           os.Getenv("LLM_BASE_URL"),
		APIKey:            os.Getenv("LLM_API_KEY"),
		Model:             os.Getenv("LLM_MODEL"),
		Temperature:       envOptionalFloat("LLM_TEMPERATURE"),
		StrictSchema:      envBool("LLM_STRICT_SCHEMA", true),
		LangfuseBaseURL:   os.Getenv("LANGFUSE_BASE_URL"),
		LangfusePublicKey: os.Getenv("LANGFUSE_PUBLIC_KEY"),
		LangfuseSecretKey: os.Getenv("LANGFUSE_SECRET_KEY"),
	}
}

// Require reports every missing connection setting at once, for callers whose job is to spend
// LLM calls. Tracing is never required.
func (l LLM) Require() error {
	var missing []string
	for _, v := range []struct {
		key, value string
	}{
		{"LLM_BASE_URL", l.BaseURL},
		{"LLM_API_KEY", l.APIKey},
		{"LLM_MODEL", l.Model},
	} {
		if v.value == "" {
			missing = append(missing, v.key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required env: %s", strings.Join(missing, ", "))
	}
	if l.Temperature != nil && (math.IsNaN(*l.Temperature) || math.IsInf(*l.Temperature, 0) || *l.Temperature < 0 || *l.Temperature > 2) {
		return fmt.Errorf("config: LLM_TEMPERATURE must be a number between 0 and 2")
	}
	return nil
}

// Settings maps onto the shape internal/platform/llm takes; the only place the two meet.
func (l LLM) Settings(model string) llm.Settings {
	return llm.Settings{
		BaseURL:           l.BaseURL,
		APIKey:            l.APIKey,
		Model:             model,
		Temperature:       l.Temperature,
		SchemaMode:        schemaMode(l.StrictSchema),
		LangfuseBaseURL:   l.LangfuseBaseURL,
		LangfusePublicKey: l.LangfusePublicKey,
		LangfuseSecretKey: l.LangfuseSecretKey,
	}
}

// envOptionalFloat distinguishes an unset sampling parameter from an explicit zero.
// A malformed value is carried as NaN so Require/NewClient can reject it instead of
// silently falling back to the library's default.
func envOptionalFloat(key string) *float64 {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		value = math.NaN()
	}
	return &value
}

// schemaMode turns the env's yes/no into the library's vocabulary. The words differ on purpose:
// the library's has room for answers the deployment's does not anticipate.
func schemaMode(strict bool) llm.SchemaMode {
	if strict {
		return llm.SchemaModeStrict
	}

	return llm.SchemaModeJSONObject
}
