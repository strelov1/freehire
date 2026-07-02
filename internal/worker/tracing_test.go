package worker

import (
	"testing"

	"github.com/strelov1/freehire/internal/config"
)

func TestTracing_disabledWhenUnconfigured(t *testing.T) {
	tr, flush := Tracing(config.Enrich{})
	if tr != nil {
		t.Error("tracer should be nil when Langfuse is unconfigured")
	}
	if flush == nil {
		t.Fatal("flush must never be nil")
	}
	flush() // must be a safe no-op
}

func TestTracing_enabledWhenConfigured(t *testing.T) {
	cfg := config.Enrich{
		LangfuseBaseURL:   "https://us.cloud.langfuse.com",
		LangfusePublicKey: "pk-lf-x",
		LangfuseSecretKey: "sk-lf-y",
	}
	tr, flush := Tracing(cfg)
	if tr == nil {
		t.Fatal("tracer should be non-nil when fully configured")
	}
	// flush drains an empty buffer: no generations queued, so no network call.
	flush()
}
