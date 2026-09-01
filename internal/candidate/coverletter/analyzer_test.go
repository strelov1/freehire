package coverletter

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// scriptedModel answers each successive call with the next scripted response, and keeps
// every prompt it was sent so a test can assert on what did NOT reach the model.
type scriptedModel struct {
	responses []string
	err       error
	calls     int
	prompts   []string
}

func (m *scriptedModel) GenerateContent(_ context.Context, msgs []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	for _, msg := range msgs {
		for _, part := range msg.Parts {
			if text, ok := part.(llms.TextContent); ok {
				m.prompts = append(m.prompts, text.Text)
			}
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	i := m.calls
	m.calls++
	if i >= len(m.responses) {
		return nil, errors.New("scriptedModel: unexpected extra call")
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: m.responses[i]}}}, nil
}

func (m *scriptedModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", nil
}

func (m *scriptedModel) sent() string { return strings.Join(m.prompts, "\n") }

func longBody(n int) string { return strings.Repeat("word ", n/5) }

func testInput(atoms []experience.Atom) Input {
	return Input{
		Context: fitanalysis.TailoringContext{
			Job: fitanalysis.TailoringJob{Title: "Backend Engineer", Company: "Acme", Description: "We need Kafka."},
			MissingHave: []matchanalysis.Requirement{
				{Text: "Kafka", Priority: "required", Status: "missing-have"},
			},
			MissingGap: []matchanalysis.Requirement{
				{Text: "Rust", Priority: "preferred", Status: "missing-gap"},
			},
		},
		Candidate:       resumeextract.Professional{Summary: "Go dev"},
		Atoms:           atoms,
		Band:            BandStandard,
		PostingLanguage: "de",
	}
}

func manualAtom(claim string) experience.Atom {
	return experience.Atom{ID: uuid.New(), Claim: claim, Provenance: experience.ProvenanceManual}
}

func TestDraftNilClientIsNoOp(t *testing.T) {
	got, err := NewAnalyzer(nil).Draft(context.Background(), testInput([]experience.Atom{manualAtom("built a pipeline")}))
	if err != nil || got != nil {
		t.Fatalf("nil analyzer = (%v, %v), want (nil, nil)", got, err)
	}
}

// The gate lives INSIDE Draft. A caller that forgets to filter must not be able to leak an
// inferred atom into the prompt — that is the difference between a rule and a convention.
func TestDraftWithholdsInferredAtomsFromTheModel(t *testing.T) {
	inferred := experience.Atom{ID: uuid.New(), Claim: "SECRETINFERREDCLAIM", Provenance: experience.ProvenanceAgentInferred}
	kept := manualAtom("KEPTMANUALCLAIM")
	body := longBody(400)
	model := &scriptedModel{responses: []string{
		`{"selected":["` + kept.ID.String() + `"]}`,
		`{"body":"` + body + `"}`,
		`{"body":"` + body + `"}`,
	}}

	_, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{inferred, kept}))
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if strings.Contains(model.sent(), "SECRETINFERREDCLAIM") {
		t.Error("an agent_inferred claim reached the model; the gate must run before the first call")
	}
	if !strings.Contains(model.sent(), "KEPTMANUALCLAIM") {
		t.Error("the manual claim never reached the model")
	}
}

func TestDraftRefusesWhenNoEvidenceIsPublishable(t *testing.T) {
	inferred := experience.Atom{ID: uuid.New(), Claim: "inferred", Provenance: experience.ProvenanceAgentInferred}
	model := &scriptedModel{}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{inferred}))

	if !errors.Is(err, ErrNoPublishableEvidence) {
		t.Fatalf("err = %v, want ErrNoPublishableEvidence", err)
	}
	if got != nil {
		t.Errorf("letter = %v, want nil", got)
	}
	if model.calls != 0 {
		t.Errorf("model called %d times, want 0 — there is nothing honest to write from", model.calls)
	}
}

func TestDraftRunsThreeStagesAndReturnsTheAuditedBody(t *testing.T) {
	a := manualAtom("cut p95 from 800ms to 120ms")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + a.ID.String() + `"]}`,
		`{"body":"DRAFTED ` + longBody(400) + `"}`,
		`{"body":"AUDITED ` + longBody(400) + `"}`,
	}}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a}))
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if model.calls != 3 {
		t.Errorf("model called %d times, want 3 — select, draft, audit", model.calls)
	}
	if !strings.HasPrefix(got.Body, "AUDITED") {
		t.Errorf("body starts %q, want the audited one", got.Body[:min(20, len(got.Body))])
	}
	if len(got.Cited) != 1 || got.Cited[0] != a.ID {
		t.Errorf("Cited = %v, want [%v]", got.Cited, a.ID)
	}
}

// A third stage may improve the result, never destroy it.
func TestDraftKeepsTheDraftWhenTheAuditWillNotParse(t *testing.T) {
	a := manualAtom("shipped it")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + a.ID.String() + `"]}`,
		`{"body":"DRAFTED ` + longBody(400) + `"}`,
		`not json at all`,
	}}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a}))
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if !strings.HasPrefix(got.Body, "DRAFTED") {
		t.Errorf("body = %q, want the un-audited draft", got.Body[:min(20, len(got.Body))])
	}
}

func TestDraftKeepsTheDraftWhenTheAuditCutsBelowTheFloor(t *testing.T) {
	a := manualAtom("shipped it")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + a.ID.String() + `"]}`,
		`{"body":"DRAFTED ` + longBody(400) + `"}`,
		`{"body":"too short"}`,
	}}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a}))
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if !strings.HasPrefix(got.Body, "DRAFTED") {
		t.Errorf("body = %q, want the un-audited draft — the audit emptied it", got.Body[:min(20, len(got.Body))])
	}
}

func TestDraftFailsWhenTheGatewayFails(t *testing.T) {
	a := manualAtom("shipped it")
	model := &scriptedModel{err: errors.New("gateway down")}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a}))

	if err == nil {
		t.Fatal("err = nil, want a failure — a caller must not overwrite a stored draft with nothing")
	}
	if got != nil {
		t.Errorf("letter = %v, want nil", got)
	}
}

func TestDraftSendsOnlyTheProfessionalProjection(t *testing.T) {
	a := manualAtom("shipped it")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + a.ID.String() + `"]}`,
		`{"body":"` + longBody(400) + `"}`,
		`{"body":"` + longBody(400) + `"}`,
	}}
	in := testInput([]experience.Atom{a})
	in.Candidate = resumeextract.Professional{Summary: "PROJECTEDSUMMARY"}

	if _, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), in); err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if !strings.Contains(model.sent(), "PROJECTEDSUMMARY") {
		t.Error("the projection did not reach the model")
	}
}

// The inversion against matchanalysis: the employer reads this, so the vacancy decides.
func TestLanguageComesFromThePostingAndFallsBackToEnglish(t *testing.T) {
	if got := LanguageOf("de"); got != "de" {
		t.Errorf("LanguageOf(de) = %q, want de", got)
	}
	if got := LanguageOf(""); got != "en" {
		t.Errorf("LanguageOf(empty) = %q, want en — about 5%% of open postings carry no language, and a letter in the candidate's own tongue reads as a mistake", got)
	}
	if got := LanguageOf("  "); got != "en" {
		t.Errorf("LanguageOf(blank) = %q, want en", got)
	}
}

func TestDraftStampsTheLetterWithThePostingLanguage(t *testing.T) {
	a := manualAtom("shipped it")
	model := &scriptedModel{responses: []string{
		`{"selected":["` + a.ID.String() + `"]}`,
		`{"body":"` + longBody(400) + `"}`,
		`{"body":"` + longBody(400) + `"}`,
	}}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a}))
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if got.Language != "de" {
		t.Errorf("Language = %q, want de from the posting", got.Language)
	}
}

func TestDraftDropsACitationTheModelInvented(t *testing.T) {
	a := manualAtom("shipped it")
	invented := uuid.New()
	model := &scriptedModel{responses: []string{
		`{"selected":["` + a.ID.String() + `","` + invented.String() + `"]}`,
		`{"body":"` + longBody(400) + `"}`,
		`{"body":"` + longBody(400) + `"}`,
	}}

	got, err := NewAnalyzer(llm.NewWithModel(model)).Draft(context.Background(), testInput([]experience.Atom{a}))
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	for _, id := range got.Cited {
		if id == invented {
			t.Fatal("an invented citation survived; the model may not widen the offered set")
		}
	}
}
