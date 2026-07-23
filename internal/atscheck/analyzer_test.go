package atscheck

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/llm"
)

type fakeModel struct {
	resp string
	err  error
}

func (f fakeModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: f.resp}}}, nil
}
func (fakeModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

func TestAnalyze_NilClientIsNoOp(t *testing.T) {
	got, err := NewAnalyzer(nil).Analyze(context.Background(), "some cv")
	if err != nil || got != nil {
		t.Fatalf("nil analyzer = (%v,%v), want (nil,nil)", got, err)
	}
}

func TestAnalyze_ParsesAndSanitizes(t *testing.T) {
	model := fakeModel{resp: `{"content_quality":150,"suggestions":["  Use stronger action verbs.  ","",  "Quantify your impact."]}`}
	a := NewAnalyzer(llm.NewWithModel(model))
	got, err := a.Analyze(context.Background(), `{"summary":"Go dev"}`)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got.ContentQuality != 100 {
		t.Errorf("ContentQuality = %d, want clamped to 100", got.ContentQuality)
	}
	if len(got.Suggestions) != 2 {
		t.Errorf("Suggestions = %v, want 2 (empty dropped)", got.Suggestions)
	}
	if got.Suggestions[0] != "Use stronger action verbs." {
		t.Errorf("Suggestions[0] = %q, want trimmed", got.Suggestions[0])
	}
}

func TestAnalyze_ModelErrorPropagates(t *testing.T) {
	a := NewAnalyzer(llm.NewWithModel(fakeModel{err: errors.New("boom")}))
	if _, err := a.Analyze(context.Background(), `{"summary":"x"}`); err == nil {
		t.Fatal("want error when the model fails")
	}
}

// recordingModel captures the user prompt so a test can assert on what reaches the model.
type recordingModel struct {
	resp   string
	prompt string
}

func (m *recordingModel) GenerateContent(_ context.Context, msgs []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
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

func TestAnalyze_StripsContactsAndSendsNoRawCV(t *testing.T) {
	m := &recordingModel{resp: `{"content_quality":80,"suggestions":["Quantify your impact."]}`}
	structured := `{"full_name":"Jane Doe","email":"jane@x.com","phone":"+1 415 555 0000","links":["github.com/jane"],"summary":"Backend engineer"}`
	if _, err := NewAnalyzer(llm.NewWithModel(m)).Analyze(context.Background(), structured); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if !strings.Contains(m.prompt, "Backend engineer") {
		t.Errorf("prompt missing the structured content:\n%s", m.prompt)
	}
	for _, leak := range []string{"Jane Doe", "jane@x.com", "555 0000", "github.com/jane"} {
		if strings.Contains(m.prompt, leak) {
			t.Errorf("prompt leaked contact %q:\n%s", leak, m.prompt)
		}
	}
}

func TestAnalyze_NoStructuredIsNoReview(t *testing.T) {
	m := &recordingModel{resp: `{"content_quality":80}`}
	got, err := NewAnalyzer(llm.NewWithModel(m)).Analyze(context.Background(), "")
	if err != nil || got != nil {
		t.Fatalf("empty structured = (%v,%v), want (nil,nil)", got, err)
	}
	if m.prompt != "" {
		t.Errorf("model must not be called with no structured résumé, prompt=%q", m.prompt)
	}
}
