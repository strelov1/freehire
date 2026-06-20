package enrich

import (
	"context"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/llm"
)

// fakeLLM is a stub llms.Model that returns a canned response, letting us test the
// provider's prompt-to-Enrichment mapping without a live model.
type fakeLLM struct {
	resp     string
	err      error
	gotMsgs  []llms.MessageContent
	gotCalls int
}

func (f *fakeLLM) GenerateContent(_ context.Context, msgs []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	f.gotCalls++
	f.gotMsgs = msgs
	if f.err != nil {
		return nil, f.err
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: f.resp}}}, nil
}

func (f *fakeLLM) Call(_ context.Context, _ string, _ ...llms.CallOption) (string, error) {
	return "", nil
}

func intVal(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

func TestEnrich_mapsStatedFields(t *testing.T) {
	raw := `{"seniority":"senior","work_mode":"remote","salary_min":70000,` +
		`"salary_max":90000,"salary_currency":"EUR","salary_period":"year",` +
		`"skills":["go","postgresql"]}`
	p := &LangChainProvider{client: llm.NewWithModel(&fakeLLM{resp: raw})}

	got, err := p.Enrich(context.Background(), JobInput{
		Title:       "Senior Go Engineer",
		Description: "Senior Go engineer, fully remote, €70k–90k/year",
	})
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	if got.Seniority != "senior" {
		t.Errorf("seniority = %q, want senior", got.Seniority)
	}
	if got.WorkMode != "remote" {
		t.Errorf("work_mode = %q, want remote", got.WorkMode)
	}
	if intVal(got.SalaryMin) != 70000 || intVal(got.SalaryMax) != 90000 {
		t.Errorf("salary = %v–%v, want 70000–90000", intVal(got.SalaryMin), intVal(got.SalaryMax))
	}
	if got.SalaryCurrency != "EUR" || got.SalaryPeriod != "year" {
		t.Errorf("salary currency/period = %q/%q, want EUR/year", got.SalaryCurrency, got.SalaryPeriod)
	}
	if len(got.Skills) != 2 || got.Skills[0] != "go" || got.Skills[1] != "postgresql" {
		t.Errorf("skills = %v, want [go postgresql]", got.Skills)
	}

	// The result must still validate against the controlled vocabularies.
	if err := got.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestEnrich_omitsUnstatedFields(t *testing.T) {
	p := &LangChainProvider{client: llm.NewWithModel(&fakeLLM{resp: `{"seniority":"junior"}`})}

	got, err := p.Enrich(context.Background(), JobInput{Description: "Junior dev wanted"})
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if got.VisaSponsorship != nil {
		t.Errorf("visa_sponsorship = %v, want nil (unstated)", *got.VisaSponsorship)
	}
	if got.CompanySize != "" {
		t.Errorf("company_size = %q, want empty (unstated)", got.CompanySize)
	}
	if got.SalaryMin != nil {
		t.Errorf("salary_min = %v, want nil (unstated)", *got.SalaryMin)
	}
}

func TestEnrich_stripsCodeFences(t *testing.T) {
	// Some models wrap JSON in a markdown fence despite JSON mode.
	fenced := "```json\n{\"work_mode\":\"hybrid\"}\n```"
	p := &LangChainProvider{client: llm.NewWithModel(&fakeLLM{resp: fenced})}

	got, err := p.Enrich(context.Background(), JobInput{Description: "x"})
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if got.WorkMode != "hybrid" {
		t.Errorf("work_mode = %q, want hybrid", got.WorkMode)
	}
}

func TestEnrich_propagatesModelError(t *testing.T) {
	p := &LangChainProvider{client: llm.NewWithModel(&fakeLLM{err: context.DeadlineExceeded})}
	if _, err := p.Enrich(context.Background(), JobInput{Description: "x"}); err == nil {
		t.Fatal("expected error from model, got nil")
	}
}

func TestEnrich_sendsSystemAndUserMessages(t *testing.T) {
	f := &fakeLLM{resp: `{}`}
	p := &LangChainProvider{client: llm.NewWithModel(f)}
	if _, err := p.Enrich(context.Background(), JobInput{Title: "Go dev"}); err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if len(f.gotMsgs) != 2 {
		t.Fatalf("sent %d messages, want 2 (system + user)", len(f.gotMsgs))
	}
	if f.gotMsgs[0].Role != llms.ChatMessageTypeSystem {
		t.Errorf("first message role = %q, want system", f.gotMsgs[0].Role)
	}
}
