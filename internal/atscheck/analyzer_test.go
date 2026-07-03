package atscheck

import (
	"context"
	"errors"
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
	model := fakeModel{resp: `{"content_quality":150,"findings":["  Use stronger action verbs.  ","",  "Quantify your impact."]}`}
	a := NewAnalyzer(llm.NewWithModel(model))
	got, err := a.Analyze(context.Background(), "cv text")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got.ContentQuality != 100 {
		t.Errorf("ContentQuality = %d, want clamped to 100", got.ContentQuality)
	}
	if len(got.Findings) != 2 {
		t.Errorf("Findings = %v, want 2 (empty dropped)", got.Findings)
	}
	if got.Findings[0] != "Use stronger action verbs." {
		t.Errorf("Findings[0] = %q, want trimmed", got.Findings[0])
	}
}

func TestAnalyze_ModelErrorPropagates(t *testing.T) {
	a := NewAnalyzer(llm.NewWithModel(fakeModel{err: errors.New("boom")}))
	if _, err := a.Analyze(context.Background(), "cv"); err == nil {
		t.Fatal("want error when the model fails")
	}
}
