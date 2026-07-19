package matchanalysis

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/jobmatch"
	"github.com/strelov1/freehire/internal/llm"
)

// queuedModel returns canned responses in order, one per GenerateContent call — so a
// single fake drives the three sequential stages of the chain.
type queuedModel struct {
	resp []string
	err  error
	n    int
}

func (m *queuedModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	r := m.resp[m.n]
	m.n++
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: r}}}, nil
}
func (*queuedModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

func sampleInput() Input {
	return Input{
		JobTitle:            "Senior Go Engineer",
		JobDescription:      "Build backends in Go. Kafka a plus.",
		CompanyInfo:         `{"tagline":"We ship fridges"}`,
		CVText:              "Backend engineer, 5y Go at Acme.",
		Match:               jobmatch.JobMatch{Matched: []string{"go"}, Missing: []string{"kafka"}, CoveragePercent: 50},
		JobWorkMode:         "onsite",
		JobLocation:         "Berlin, Germany",
		JobRegions:          []string{"eu"},
		JobCountries:        []string{"de"},
		LocationPreferences: `{"work_modes":["remote"],"base":{"country":"br","city":"São Paulo"}}`,
	}
}

const (
	stage1JSON = `{"requirements":[{"text":"Go","priority":"required","status":"covered","evidence":"5y at Acme"},{"text":"Kafka","priority":"preferred","status":"missing-gap"}]}`
	stage2JSON = `{"title_alignment":{"score":80,"comment":"titles align"},"experience_relevance":{"score":70},"seniority_fit":{"score":60},"skills_coverage":{"score":50},"company_context":{"score":40},"location_fit":{"score":60},"strengths":["Strong Go"],"gaps":["No Kafka"],"recommendation":"Apply."}`
	// Stage 3 tightens experience down and prunes the unsupported strength.
	stage3JSON = `{"title_alignment":{"score":80},"experience_relevance":{"score":50},"seniority_fit":{"score":60},"skills_coverage":{"score":50},"company_context":{"score":40},"location_fit":{"score":60},"strengths":[],"gaps":["No Kafka","Thin on scale"],"recommendation":"Apply, address Kafka."}`
)

func TestAnalyze_NilClientIsNoOp(t *testing.T) {
	got, err := NewAnalyzer(nil).Analyze(context.Background(), sampleInput())
	if err != nil || got != nil {
		t.Fatalf("nil analyzer = (%v,%v), want (nil,nil)", got, err)
	}
}

func TestAnalyze_ThreeStageChainUsesAuditedVerdict(t *testing.T) {
	m := &queuedModel{resp: []string{stage1JSON, stage2JSON, stage3JSON}}
	got, err := NewAnalyzer(llm.NewWithModel(m)).Analyze(context.Background(), sampleInput())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if m.n != 3 {
		t.Errorf("stages called = %d, want 3", m.n)
	}
	// Experience relevance came from Stage 3 (50), not Stage 2 (70).
	if got.Dimensions[1].Key != DimExperienceRelevance || got.Dimensions[1].Score != 50 {
		t.Errorf("experience = %+v, want audited score 50", got.Dimensions[1])
	}
	if len(got.Strengths) != 0 {
		t.Errorf("Strengths = %v, want the audit's empty list", got.Strengths)
	}
	if len(got.RequirementMatch) != 2 {
		t.Errorf("RequirementMatch = %d, want the 2 Stage-1 requirements", len(got.RequirementMatch))
	}
	// overall = 80*.20 + 50*.25 + 60*.15 + 50*.15 + 40*.10 + 60*.15 = 16+12.5+9+7.5+4+9 = 58.
	if got.OverallScore != 58 {
		t.Errorf("OverallScore = %d, want 58", got.OverallScore)
	}
}

func TestAnalyze_Stage3FailFallsBackToStage2(t *testing.T) {
	// Stage 3 fails on both attempts (the retry also gets junk), then falls back.
	m := &queuedModel{resp: []string{stage1JSON, stage2JSON, "not json", "still not json"}}
	got, err := NewAnalyzer(llm.NewWithModel(m)).Analyze(context.Background(), sampleInput())
	if err != nil {
		t.Fatalf("Analyze should degrade, not error: %v", err)
	}
	// Falls back to Stage 2's experience score (70).
	if got.Dimensions[1].Score != 70 {
		t.Errorf("experience = %d, want Stage-2 fallback 70", got.Dimensions[1].Score)
	}
}

func TestAnalyze_Stage3PartialMergesOntoStage2(t *testing.T) {
	// A budget model returns only the fields it changed. The omitted dimensions must
	// keep their Stage 2 scores, not collapse to 0 (which would corrupt a strong verdict).
	partial := `{"experience_relevance":{"score":40},"gaps":["Thin on scale"]}`
	m := &queuedModel{resp: []string{stage1JSON, stage2JSON, partial}}
	got, err := NewAnalyzer(llm.NewWithModel(m)).Analyze(context.Background(), sampleInput())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	// title_alignment omitted by the audit → keeps Stage 2's 80, not 0.
	if got.Dimensions[0].Key != DimTitleAlignment || got.Dimensions[0].Score != 80 {
		t.Errorf("title = %+v, want Stage-2 80 preserved", got.Dimensions[0])
	}
	// experience_relevance present in the audit → overridden to 40.
	if got.Dimensions[1].Score != 40 {
		t.Errorf("experience = %d, want audited 40", got.Dimensions[1].Score)
	}
	if len(got.Gaps) != 1 || got.Gaps[0] != "Thin on scale" {
		t.Errorf("gaps = %v, want the audit's override", got.Gaps)
	}
}

func TestAnalyzeStream_RetriesATransientStageFailure(t *testing.T) {
	// Stage 1 first returns an HTML error page (a transient gateway 502), then valid JSON
	// on the retry; the chain must recover and produce the final analysis.
	m := &queuedModel{resp: []string{`<html>502 Bad Gateway</html>`, stage1JSON, stage2JSON, stage3JSON}}
	got, err := NewAnalyzer(llm.NewWithModel(m)).Analyze(context.Background(), sampleInput())
	if err != nil {
		t.Fatalf("Analyze should recover on retry: %v", err)
	}
	if got == nil || got.OverallScore != 58 {
		t.Errorf("recovered final = %+v, want overall 58", got)
	}
	if m.n != 4 {
		t.Errorf("model calls = %d, want 4 (1 failed stage-1 + retry + stages 2,3)", m.n)
	}
}

func TestAnalyze_Stage1ErrorPropagates(t *testing.T) {
	m := &queuedModel{err: errors.New("boom")}
	if _, err := NewAnalyzer(llm.NewWithModel(m)).Analyze(context.Background(), sampleInput()); err == nil {
		t.Fatal("want error when Stage 1 fails")
	}
}

func TestAnalyzeStream_EmitsOrderedEventsAndMatchesSyncFinal(t *testing.T) {
	events := func() []Event {
		m := &queuedModel{resp: []string{stage1JSON, stage2JSON, stage3JSON}}
		var got []Event
		final, err := NewAnalyzer(llm.NewWithModel(m)).AnalyzeStream(context.Background(), sampleInput(), func(e Event) {
			got = append(got, e)
		})
		if err != nil {
			t.Fatalf("AnalyzeStream: %v", err)
		}
		if final == nil || final.OverallScore != 58 {
			t.Fatalf("stream final = %+v, want overall 58", final)
		}
		return got
	}()

	// Kinds must arrive in chain order.
	wantKinds := []EventKind{
		EventStageStart, EventRequirements, EventStageDone, // stage 1
		EventStageStart, EventDimensions, EventStageDone, // stage 2
		EventStageStart, EventStageDone, // stage 3
		EventFinal,
	}
	if len(events) != len(wantKinds) {
		t.Fatalf("emitted %d events, want %d: %+v", len(events), len(wantKinds), events)
	}
	for i, k := range wantKinds {
		if events[i].Kind != k {
			t.Errorf("event[%d].Kind = %q, want %q", i, events[i].Kind, k)
		}
	}
	// The requirements event carries the Stage-1 match; the final event carries the analysis.
	if reqEv := events[1]; len(reqEv.Requirements) != 2 {
		t.Errorf("requirements event = %+v, want 2 requirements", reqEv)
	}
	if fin := events[len(events)-1]; fin.Analysis == nil || len(fin.Analysis.Dimensions) != 6 {
		t.Errorf("final event analysis = %+v, want 6 dimensions", fin.Analysis)
	}

	// Analyze must return the identical final verdict (it is a thin collector).
	m := &queuedModel{resp: []string{stage1JSON, stage2JSON, stage3JSON}}
	sync, _ := NewAnalyzer(llm.NewWithModel(m)).Analyze(context.Background(), sampleInput())
	if sync == nil || sync.OverallScore != 58 {
		t.Errorf("Analyze final = %+v, want overall 58 (same as stream)", sync)
	}
}

func TestAnalyzeStream_NilClientIsNoOp(t *testing.T) {
	got, err := NewAnalyzer(nil).AnalyzeStream(context.Background(), sampleInput(), func(Event) {
		t.Error("no events expected from a nil client")
	})
	if err != nil || got != nil {
		t.Fatalf("nil stream = (%v,%v), want (nil,nil)", got, err)
	}
}

func TestStagePrompts_CarryTheirInputs(t *testing.T) {
	in := sampleInput()
	reqs := []Requirement{{Text: "Go", Priority: "required", Status: "covered"}}
	if s := stage1UserPrompt(in); !strings.Contains(s, "Senior Go Engineer") || !strings.Contains(s, "5y Go at Acme") || !strings.Contains(s, "go") {
		t.Error("stage1 prompt must carry job title, CV text, and the anchor")
	}
	if s := stage2UserPrompt(in, reqs); !strings.Contains(s, "We ship fridges") || !strings.Contains(s, "covered") {
		t.Error("stage2 prompt must carry company_info and the Stage-1 match")
	}
	// Stage 2 must carry the job geography and the candidate's location preferences so
	// the model can score location & work-mode fit.
	if s := stage2UserPrompt(in, reqs); !strings.Contains(s, "Berlin") || !strings.Contains(s, "onsite") || !strings.Contains(s, "São Paulo") {
		t.Error("stage2 prompt must carry job geography + candidate location preferences")
	}
	v := recruiterVerdict{TitleAlignment: dimScore{Score: 80}, Recommendation: "Apply."}
	if s := stage3UserPrompt(in, reqs, v); !strings.Contains(s, "Apply.") {
		t.Error("stage3 prompt must carry the Stage-2 verdict to audit")
	}
}
