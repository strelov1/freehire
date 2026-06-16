package enrich

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tmc/langchaingo/llms"
)

// blockingLLM hangs until its context is cancelled, modelling a stalled gateway.
type blockingLLM struct{}

func (blockingLLM) GenerateContent(ctx context.Context, _ []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (blockingLLM) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

// A stalled gateway must not hang the worker: the provider's own timeout cancels
// the call so Enrich returns an error instead of blocking forever.
func TestEnrichTimesOutOnStalledModel(t *testing.T) {
	p := &LangChainProvider{llm: blockingLLM{}, timeout: 20 * time.Millisecond}

	done := make(chan error, 1)
	go func() {
		_, err := p.Enrich(context.Background(), JobInput{Description: "x"})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Enrich returned nil error, want a timeout error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Enrich did not return; the per-call timeout did not fire")
	}
}

// The user prompt must include the job URL: some ATS encode the location (and
// sometimes the role) in the URL path (e.g. SuccessFactors
// /job/Limburg-Maschinenfuhrer/<id>/), which the model can read even when the
// structured location field is empty.
func TestUserPromptIncludesURL(t *testing.T) {
	url := "https://jobs.tetrapak.com/job/Limburg-Maschinenfuhrer/883999301/"
	got := userPrompt(JobInput{Title: "Engineer", Company: "Acme", URL: url, Description: "x"})
	if !strings.Contains(got, url) {
		t.Errorf("userPrompt must include the URL (a location signal for slug-based ATS), got:\n%s", got)
	}
}

// The system prompt must pin the region (reach) vocabulary it asks the model to
// use, drawn from the same list Validate enforces, so prompt and validator
// cannot drift.
func TestSystemPromptIncludesRegionVocabulary(t *testing.T) {
	p := buildSystemPrompt()

	if !strings.Contains(p, "regions") {
		t.Errorf("prompt must mention regions, got:\n%s", p)
	}
	for _, v := range RegionValues {
		if !strings.Contains(p, v) {
			t.Errorf("prompt must list region value %q", v)
		}
	}
}

// Relax: the prompt permits a novel own-label for the six discovery facets while
// keeping the strict "exactly one allowed value" instruction for the served fields.
func TestSystemPromptRelaxesDiscoveryFacets(t *testing.T) {
	p := buildSystemPrompt()
	if !strings.Contains(p, "exactly one of the allowed values") {
		t.Errorf("served enum fields must keep the strict instruction")
	}
	if !strings.Contains(p, "concise lowercase label of your own") {
		t.Errorf("discovery facets must permit a novel own label")
	}
	for _, f := range []string{"work_mode", "regions", "seniority", "category"} {
		if !strings.Contains(p, f) {
			t.Errorf("discovery instruction should name the facet %q", f)
		}
	}
}
