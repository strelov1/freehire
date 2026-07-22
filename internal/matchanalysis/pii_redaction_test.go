package matchanalysis

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/pii"
)

// recordingModel records every user prompt it is sent and returns queued canned responses,
// so a test can assert on exactly what text would reach the model provider.
type recordingModel struct {
	resp    []string
	n       int
	prompts []string
}

func (m *recordingModel) GenerateContent(_ context.Context, msgs []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	var b strings.Builder
	for _, msg := range msgs {
		for _, part := range msg.Parts {
			if t, ok := part.(llms.TextContent); ok {
				b.WriteString(t.Text)
			}
		}
	}
	m.prompts = append(m.prompts, b.String())
	r := m.resp[m.n]
	m.n++
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: r}}}, nil
}
func (*recordingModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

// spanDetector reports each configured substring as a NAME span (a stand-in for the model).
type spanDetector struct{ names []string }

func (d spanDetector) Detect(_ context.Context, text string) ([]pii.Span, error) {
	var spans []pii.Span
	for _, n := range d.names {
		if i := strings.Index(text, n); i >= 0 {
			spans = append(spans, pii.Span{Start: i, End: i + len(n), Kind: pii.KindName})
		}
	}
	return spans, nil
}

func piiInput() Input {
	in := sampleInput()
	in.CVText = "Ivan Petrov ivan@petrov.io\nSenior Go Engineer, 5y Go at Acme."
	return in
}

func TestAnalyzeStream_MasksPIIInPromptsAndRestoresOutput(t *testing.T) {
	// Stage 2 recommendation echoes the name placeholder the model would have seen; the
	// restored output must show the real name, and no prompt may carry the raw PII.
	s2 := `{"title_alignment":{"score":80},"experience_relevance":{"score":70},"seniority_fit":{"score":60},"skills_coverage":{"score":50},"company_context":{"score":40},"location_fit":{"score":60},"strengths":[],"gaps":[],"recommendation":"[REDACTED_NAME_1] is a strong Go fit."}`
	// Stage 3 is a partial audit that does not touch the recommendation, so Stage 2's
	// (placeholder-bearing) recommendation survives to the final — the restore path under test.
	s3 := `{"experience_relevance":{"score":50}}`
	m := &recordingModel{resp: []string{stage1JSON, s2, s3}}
	det := spanDetector{names: []string{"Ivan Petrov"}}

	final, err := NewAnalyzer(llm.NewWithModel(m), det).AnalyzeStream(context.Background(), piiInput(), func(Event) {})
	if err != nil {
		t.Fatalf("AnalyzeStream: %v", err)
	}
	if final == nil {
		t.Fatal("expected an analysis, got nil")
	}

	if len(m.prompts) == 0 {
		t.Fatal("no prompts recorded")
	}
	for i, p := range m.prompts {
		if strings.Contains(p, "Ivan Petrov") || strings.Contains(p, "ivan@petrov.io") {
			t.Errorf("stage %d prompt leaked PII:\n%s", i+1, p)
		}
	}
	if !strings.Contains(final.Recommendation, "Ivan Petrov") {
		t.Errorf("recommendation not restored: %q", final.Recommendation)
	}
	if strings.Contains(final.Recommendation, "REDACTED") {
		t.Errorf("placeholder leaked into output: %q", final.Recommendation)
	}
}

func TestAnalyzeStream_FailClosedWhenDetectorErrors(t *testing.T) {
	m := &recordingModel{resp: []string{stage1JSON, stage2JSON, stage3JSON}}
	final, err := NewAnalyzer(llm.NewWithModel(m), failingDetector{}).AnalyzeStream(context.Background(), piiInput(), func(Event) {
		t.Error("no events expected when the detector fails")
	})
	if err != nil {
		t.Fatalf("fail-closed should degrade to no analysis, not error: %v", err)
	}
	if final != nil {
		t.Fatalf("expected nil analysis (fail-closed), got %+v", final)
	}
	if m.n != 0 {
		t.Fatalf("model must not be called when the detector fails, called %d", m.n)
	}
}

type failingDetector struct{}

func (failingDetector) Detect(context.Context, string) ([]pii.Span, error) {
	return nil, errors.New("detector down")
}

// noopDetector reports no spans — the chain runs exactly as before masking (used by the
// pre-existing analyzer tests, whose sample CV carries no PII).
type noopDetector struct{}

func (noopDetector) Detect(context.Context, string) ([]pii.Span, error) { return nil, nil }
