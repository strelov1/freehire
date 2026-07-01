package worker

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/llm"
)

// tracerShutdownTimeout bounds the final flush so a stuck Langfuse endpoint cannot
// hold a finished worker open.
const tracerShutdownTimeout = 15 * time.Second

// Tracing builds an LLM tracer from the enrich config and returns it together with
// a flush function to defer at the end of a run. When Langfuse is not fully
// configured the tracer is nil — a no-op the LLM client tolerates — and flush does
// nothing, so a worker with no Langfuse env runs exactly as before.
func Tracing(cfg config.Enrich) (llm.Tracer, func()) {
	t := llm.NewTracer(cfg.LangfuseBaseURL, cfg.LangfusePublicKey, cfg.LangfuseSecretKey)
	if t == nil {
		return nil, func() {}
	}
	return t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), tracerShutdownTimeout)
		defer cancel()
		if err := t.Shutdown(ctx); err != nil {
			log.Printf("langfuse: shutdown: %v", err)
		}
	}
}
