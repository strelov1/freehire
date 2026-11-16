package assistant

import (
	"testing"

	"github.com/tmc/langchaingo/llms"
)

// The usage frame carries the cached share of the prompt beside the totals: it is what a
// replayed conversation actually costs, and the only place a client can see it.
func TestEmitUsageCarriesCachedInputTokens(t *testing.T) {
	var got []Event
	emitUsage(func(e Event) { got = append(got, e) }, &llms.ContentChoice{GenerationInfo: map[string]any{
		"PromptTokens":       1200,
		"CompletionTokens":   80,
		"TotalTokens":        1280,
		"PromptCachedTokens": 1024,
	}})
	if len(got) != 1 {
		t.Fatalf("emitted %d events, want one usage frame", len(got))
	}
	if e := got[0]; e.Kind != EventUsage || e.InputTokens != 1200 || e.CachedInputTokens != 1024 || e.OutputTokens != 80 {
		t.Errorf("usage frame = %+v, want input 1200, cached 1024, output 80", e)
	}
}
