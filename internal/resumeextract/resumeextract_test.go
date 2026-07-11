package resumeextract

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/llm"
)

func TestSanitize_BoundsStringsAndYears(t *testing.T) {
	s := Structured{
		FullName:   strings.Repeat("a", maxNameRunes+50),
		Summary:    strings.Repeat("b", maxSummaryRunes+500),
		TotalYears: maxYears + 100,
		Experience: []Experience{{
			Title:   strings.Repeat("c", maxShortRunes+50),
			Company: "Acme",
			Summary: strings.Repeat("d", maxEntrySummaryRunes+500),
		}},
	}
	s.Sanitize()

	if got := len([]rune(s.FullName)); got > maxNameRunes {
		t.Errorf("FullName runes = %d, want <= %d", got, maxNameRunes)
	}
	if got := len([]rune(s.Summary)); got > maxSummaryRunes {
		t.Errorf("Summary runes = %d, want <= %d", got, maxSummaryRunes)
	}
	if s.TotalYears > maxYears {
		t.Errorf("TotalYears = %d, want <= %d", s.TotalYears, maxYears)
	}
	if got := len([]rune(s.Experience[0].Title)); got > maxShortRunes {
		t.Errorf("Experience title runes = %d, want <= %d", got, maxShortRunes)
	}
	if got := len([]rune(s.Experience[0].Summary)); got > maxEntrySummaryRunes {
		t.Errorf("Experience summary runes = %d, want <= %d", got, maxEntrySummaryRunes)
	}
}

func TestSanitize_NegativeYearsCoercedToZero(t *testing.T) {
	s := Structured{TotalYears: -3}
	s.Sanitize()
	if s.TotalYears != 0 {
		t.Errorf("TotalYears = %d, want 0", s.TotalYears)
	}
}

func TestSanitize_CapsArrayCardinality(t *testing.T) {
	exp := make([]Experience, maxExperience+10)
	for i := range exp {
		exp[i] = Experience{Title: "Engineer", Company: "Acme"}
	}
	langs := make([]string, maxLanguages+10)
	for i := range langs {
		langs[i] = "English"
	}
	s := Structured{Experience: exp, Languages: langs}
	s.Sanitize()
	if len(s.Experience) > maxExperience {
		t.Errorf("Experience len = %d, want <= %d", len(s.Experience), maxExperience)
	}
	if len(s.Languages) > maxLanguages {
		t.Errorf("Languages len = %d, want <= %d", len(s.Languages), maxLanguages)
	}
}

func TestSanitize_DropsEmptyEntries(t *testing.T) {
	s := Structured{
		Experience: []Experience{
			{Title: "Engineer", Company: "Acme"},
			{},                            // wholly empty → dropped
			{Title: "   ", Company: "\t"}, // whitespace-only → dropped
		},
		Languages: []string{"English", "", "  ", "German"},
		Links:     []string{"https://x.dev", ""},
	}
	s.Sanitize()
	if len(s.Experience) != 1 {
		t.Errorf("Experience len = %d, want 1 (empties dropped)", len(s.Experience))
	}
	if len(s.Languages) != 2 {
		t.Errorf("Languages = %v, want 2 non-empty", s.Languages)
	}
	if len(s.Links) != 1 {
		t.Errorf("Links = %v, want 1 non-empty", s.Links)
	}
}

// queuedModel returns a canned response for the single extraction call.
type queuedModel struct {
	resp string
	err  error
	n    int
}

func (m *queuedModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.n++
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: m.resp}}}, nil
}
func (*queuedModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

func TestExtract_ParsesAndSanitizes(t *testing.T) {
	raw := `{"full_name":"Jane Doe","summary":"Backend engineer.","total_years":-5,` +
		`"experience":[{"title":"Senior Go Engineer","company":"Acme","start":"2020","end":"Present"}],` +
		`"languages":["English",""]}`
	m := &queuedModel{resp: raw}
	got, err := NewExtractor(llm.NewWithModel(m)).Extract(context.Background(), "some cv text")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.n != 1 {
		t.Errorf("LLM calls = %d, want 1", m.n)
	}
	if got.FullName != "Jane Doe" || len(got.Experience) != 1 {
		t.Errorf("parsed = %+v, want name+1 experience", got)
	}
	if got.TotalYears != 0 { // sanitized: negative → 0
		t.Errorf("TotalYears = %d, want 0 (sanitized)", got.TotalYears)
	}
	if len(got.Languages) != 1 { // sanitized: empty dropped
		t.Errorf("Languages = %v, want 1 (empty dropped)", got.Languages)
	}
}

func TestExtract_UnconfiguredReturnsErrDisabled(t *testing.T) {
	_, err := NewExtractor(nil).Extract(context.Background(), "cv")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
}

func TestExtract_BadJSONErrors(t *testing.T) {
	m := &queuedModel{resp: "not json"}
	if _, err := NewExtractor(llm.NewWithModel(m)).Extract(context.Background(), "cv"); err == nil {
		t.Fatal("want error on unparseable model output, got nil")
	}
}

func TestExtractor_EnabledReflectsClient(t *testing.T) {
	if NewExtractor(nil).Enabled() {
		t.Error("nil-client extractor should be disabled")
	}
	if !NewExtractor(llm.NewWithModel(&queuedModel{})).Enabled() {
		t.Error("configured extractor should be enabled")
	}
}
