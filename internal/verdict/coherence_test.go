package verdict

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/llm"
)

// fakeModel is a stub llms.Model returning a canned response (or error).
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

func analyzerFor(resp string, err error) *Analyzer {
	return NewAnalyzer(llm.NewWithModel(fakeModel{resp: resp, err: err}))
}

func TestAnalyze_NilClientDegrades(t *testing.T) {
	a := NewAnalyzer(nil)
	got, err := a.Analyze(context.Background(), "résumé", []string{"go"})
	if err != nil || got != nil {
		t.Fatalf("nil client should degrade to (nil, nil), got (%v, %v)", got, err)
	}
}

func TestAnalyze_ValidResponse(t *testing.T) {
	resp := `{"coherence": 82, "advice": {"go": "Show a Go service in Experience."}}`
	a := analyzerFor(resp, nil)
	got, err := a.Analyze(context.Background(), "résumé text", []string{"go"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got.Coherence != 82 {
		t.Errorf("coherence = %d, want 82", got.Coherence)
	}
	if got.Advice["go"] == "" {
		t.Errorf("advice for go should be present")
	}
}

func TestAnalyze_ClampsCoherence(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want int
	}{{150, 100}, {-5, 0}, {100, 100}, {0, 0}} {
		a := analyzerFor(`{"coherence": `+itoa(tc.in)+`, "advice": {}}`, nil)
		got, err := a.Analyze(context.Background(), "r", nil)
		if err != nil {
			t.Fatalf("Analyze: %v", err)
		}
		if got.Coherence != tc.want {
			t.Errorf("coherence(%d) = %d, want %d", tc.in, got.Coherence, tc.want)
		}
	}
}

func TestAnalyze_DropsAdviceOutsideGaps(t *testing.T) {
	resp := `{"coherence": 50, "advice": {"go": "keep", "python": "drop — not a gap"}}`
	a := analyzerFor(resp, nil)
	got, err := a.Analyze(context.Background(), "r", []string{"go"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if _, ok := got.Advice["python"]; ok {
		t.Errorf("advice for a non-gap skill must be dropped: %v", got.Advice)
	}
	if got.Advice["go"] == "" {
		t.Errorf("advice for the requested gap must survive")
	}
}

func TestAnalyze_TruncatesAdvice(t *testing.T) {
	long := strings.Repeat("x", maxAdviceRunes+50)
	resp := `{"coherence": 50, "advice": {"go": "` + long + `"}}`
	a := analyzerFor(resp, nil)
	got, err := a.Analyze(context.Background(), "r", []string{"go"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if n := len([]rune(got.Advice["go"])); n > maxAdviceRunes {
		t.Errorf("advice length = %d runes, want <= %d", n, maxAdviceRunes)
	}
}

func TestAnalyze_MalformedJSONErrors(t *testing.T) {
	a := analyzerFor("not json", nil)
	got, err := a.Analyze(context.Background(), "r", []string{"go"})
	if err == nil || got != nil {
		t.Fatalf("malformed response should error, got (%v, %v)", got, err)
	}
}

func TestAnalyze_ModelErrorPropagates(t *testing.T) {
	a := analyzerFor("", errors.New("boom"))
	if _, err := a.Analyze(context.Background(), "r", []string{"go"}); err == nil {
		t.Fatal("model error should propagate")
	}
}

func TestUserPrompt_IncludesGapsAndTruncatesResume(t *testing.T) {
	sentinel := "SENTINEL_BEYOND_LIMIT"
	resume := strings.Repeat("a", maxResumeRunes) + sentinel
	p := userPrompt(resume, []string{"go", "kubernetes"})
	if !strings.Contains(p, "go") || !strings.Contains(p, "kubernetes") {
		t.Errorf("prompt must list the gap slugs, got:\n%s", p)
	}
	if strings.Contains(p, sentinel) {
		t.Errorf("résumé must be truncated at maxResumeRunes; sentinel leaked through")
	}
}

func TestSystemPrompt_MentionsCoherenceContract(t *testing.T) {
	p := buildCoherencePrompt()
	if !strings.Contains(p, "coherence") || !strings.Contains(p, "advice") {
		t.Errorf("system prompt must state the coherence/advice JSON contract, got:\n%s", p)
	}
}

// itoa avoids importing strconv just for the clamp table.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
