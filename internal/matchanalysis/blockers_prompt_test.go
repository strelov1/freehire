package matchanalysis

import (
	"context"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/hardconstraint"
	"github.com/strelov1/freehire/internal/llm"
)

// recordingModel answers canned responses in order while capturing each call's user
// prompt, so tests can assert what the chain actually sent per stage.
type recordingModel struct {
	resp  []string
	users []string
}

func (m *recordingModel) GenerateContent(_ context.Context, msgs []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	for _, mc := range msgs {
		if mc.Role == llms.ChatMessageTypeHuman {
			for _, p := range mc.Parts {
				if t, ok := p.(llms.TextContent); ok {
					m.users = append(m.users, t.Text)
				}
			}
		}
	}
	r := m.resp[0]
	m.resp = m.resp[1:]
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: r}}}, nil
}
func (*recordingModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

// TestAnalyzeStreamFeedsBlockersToPrompts locks the contract the SSE handler relies on:
// blockers placed on the Input reach the stage prompts (the stream path passes them into
// the input exactly as the POST path does).
func TestAnalyzeStreamFeedsBlockersToPrompts(t *testing.T) {
	in := sampleInput()
	in.Blockers = []hardconstraint.Blocker{
		{Category: hardconstraint.CategoryExperience, Reason: "BLOCKER-REASON-X", Met: false},
	}
	m := &recordingModel{resp: []string{stage1JSON, stage2JSON, stage3JSON}}
	if _, err := NewAnalyzer(llm.NewWithModel(m)).AnalyzeStream(context.Background(), in, func(Event) {}); err != nil {
		t.Fatalf("AnalyzeStream: %v", err)
	}
	if len(m.users) != 3 {
		t.Fatalf("captured %d user prompts, want 3", len(m.users))
	}
	for _, stage := range []int{0, 2} { // stages 1 and 3 write blockers today
		if !strings.Contains(m.users[stage], "BLOCKER-REASON-X") {
			t.Errorf("stage %d prompt should carry the unmet blocker:\n%s", stage+1, m.users[stage])
		}
	}
}

func TestStage1PromptCarriesUnmetBlockers(t *testing.T) {
	in := Input{
		JobTitle:       "Senior Engineer",
		JobDescription: "desc",
		Blockers: []hardconstraint.Blocker{
			{Category: hardconstraint.CategoryExperience, Reason: "Requires 5+ years; résumé shows 3.", Met: false},
			{Category: hardconstraint.CategoryEducation, Reason: "Requires a bachelor degree; résumé meets it.", Met: true},
		},
	}
	p := stage1UserPrompt(in, candidateContext(in.StructuredResume))
	if !strings.Contains(p, "Requires 5+ years; résumé shows 3.") {
		t.Error("stage-1 prompt should carry the unmet experience blocker")
	}
	if strings.Contains(p, "résumé meets it") {
		t.Error("stage-1 prompt should NOT list a met constraint")
	}
}

func TestStage1PromptOmitsBlockerSectionWhenAllMet(t *testing.T) {
	in := Input{JobTitle: "x", JobDescription: "y", Blockers: []hardconstraint.Blocker{
		{Category: hardconstraint.CategoryLanguage, Reason: "English present.", Met: true},
	}}
	if strings.Contains(stage1UserPrompt(in, candidateContext(in.StructuredResume)), "Hard constraints the candidate does NOT meet") {
		t.Error("no blocker section when nothing is unmet")
	}
}
