package resumeextract

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/pii"
)

// noopDetector reports no spans (used by the sanitize/parse tests, whose CVs carry no PII).
type noopDetector struct{}

func (noopDetector) Detect(context.Context, string) ([]pii.Span, error) { return nil, nil }

// nameSpanDetector reports each configured substring as a NAME span.
type nameSpanDetector struct{ names []string }

func (d nameSpanDetector) Detect(_ context.Context, text string) ([]pii.Span, error) {
	var spans []pii.Span
	for _, n := range d.names {
		if i := strings.Index(text, n); i >= 0 {
			spans = append(spans, pii.Span{Start: i, End: i + len(n), Kind: pii.KindName})
		}
	}
	return spans, nil
}

type failDetector struct{}

func (failDetector) Detect(context.Context, string) ([]pii.Span, error) {
	return nil, errors.New("detector down")
}

// recordingModel captures the user prompt so a test can assert on what reaches the model.
type recordingModel struct {
	resp   string
	prompt string
	calls  int
}

func (m *recordingModel) GenerateContent(_ context.Context, msgs []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	m.calls++
	for _, msg := range msgs {
		for _, part := range msg.Parts {
			if t, ok := part.(llms.TextContent); ok {
				m.prompt += t.Text
			}
		}
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: m.resp}}}, nil
}
func (*recordingModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

func TestExtract_ContactsFromDetectionAndRedactedPrompt(t *testing.T) {
	cv := "Ivan Petrov ivan@petrov.io github.com/ivanp\nSenior Go Engineer at Acme."
	m := &recordingModel{resp: `{"summary":"Senior Go engineer.","experience":[{"title":"Senior Go Engineer","company":"Acme"}]}`}
	e := NewExtractor(llm.NewWithModel(m), nameSpanDetector{names: []string{"Ivan Petrov"}})

	got, err := e.Extract(context.Background(), cv)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	// The model prompt must carry no raw PII.
	for _, leak := range []string{"Ivan Petrov", "ivan@petrov.io", "github.com/ivanp"} {
		if strings.Contains(m.prompt, leak) {
			t.Errorf("prompt leaked PII %q:\n%s", leak, m.prompt)
		}
	}
	// Contacts come from detection, not the model.
	if got.FullName != "Ivan Petrov" || got.Email != "ivan@petrov.io" {
		t.Errorf("contacts = %q/%q, want detected values", got.FullName, got.Email)
	}
	if len(got.Links) != 1 {
		t.Errorf("Links = %v, want the detected URL", got.Links)
	}
	// Semantic fields still parse from the (redacted) CV.
	if len(got.Experience) != 1 || got.Experience[0].Company != "Acme" {
		t.Errorf("experience = %+v, want the parsed Acme role", got.Experience)
	}
}

func TestExtract_FailClosedWhenDetectorErrors(t *testing.T) {
	m := &recordingModel{resp: `{}`}
	_, err := NewExtractor(llm.NewWithModel(m), failDetector{}).Extract(context.Background(), "Ivan Petrov cv")
	if err == nil {
		t.Fatal("expected fail-closed error when detector fails")
	}
	if m.calls != 0 {
		t.Fatalf("model must not be called when the detector fails, called %d", m.calls)
	}
}
