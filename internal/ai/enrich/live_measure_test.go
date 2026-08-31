//go:build llmlive

// Live check of the enrichment contract against the configured model. It costs real
// tokens, so it is behind the llmlive tag and never runs in CI.
//
//	LLM_BASE_URL=… LLM_API_KEY=… LLM_MODEL=… go test -tags=llmlive ./internal/ai/enrich/ -run Live -v
package enrich

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/llm"
)

const samplePosting = `Senior Backend Engineer (Go) — Acme Robotics

We are a Series B robotics company automating warehouse logistics. You will own our
order-routing services in Go and PostgreSQL, running on Kubernetes.

Fully remote within the EU. We sponsor visas for relocation to Berlin if you prefer
the office. Base salary 85,000–110,000 EUR per year. 200-400 people worldwide.`

func liveClient(t *testing.T) *llm.Client {
	t.Helper()

	s := llm.Settings{
		BaseURL: os.Getenv("LLM_BASE_URL"),
		APIKey:  os.Getenv("LLM_API_KEY"),
		Model:   os.Getenv("LLM_MODEL"),
	}
	// LLM_STRICT_SCHEMA=false measures the gateway the way a deployment that set it
	// would run: same prompt, plain JSON mode, no schema on the wire. Comparing the
	// two is the only way to answer whether a model that ignores a strict schema can
	// still be trusted with this contract.
	if os.Getenv("LLM_STRICT_SCHEMA") == "false" {
		s.SchemaMode = llm.SchemaModeJSONObject
	}
	if !s.Enabled() {
		t.Skip("LLM_BASE_URL/LLM_API_KEY/LLM_MODEL not set")
	}

	c, flush, err := llm.NewClient(s, "measure")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(flush)

	return c
}

// TestLive_EnrichmentFillsUnderTheSchema is the check the migration order was built
// around: enrichment has the longest schema and the heaviest production traffic, so
// the contract must still fill from a real posting before this ships.
func TestLive_EnrichmentFillsUnderTheSchema(t *testing.T) {
	c := liveClient(t)
	p := NewLangChainProvider(c)

	got, err := p.Enrich(context.Background(), JobInput{
		Title:       "Senior Backend Engineer (Go)",
		Company:     "Acme Robotics",
		Description: samplePosting,
	})
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	dump, _ := json.MarshalIndent(got, "", "  ")
	t.Logf("enrichment:\n%s", dump)

	if got.Summary == "" {
		t.Error("summary is empty — it is the one synthesized field and must always be produced")
	}
	if got.SalaryMin == nil || got.SalaryMax == nil || got.SalaryCurrency != "EUR" {
		t.Errorf("salary = %v..%v %q, want 85000..110000 EUR", got.SalaryMin, got.SalaryMax, got.SalaryCurrency)
	}
	if got.SalaryPeriod != "year" {
		t.Errorf("salary_period = %q, want year", got.SalaryPeriod)
	}

	// The fields the schema pins must come back inside their vocabularies.
	for field, pair := range map[string][2]any{
		"relocation":    {got.Relocation, vocab.RelocationValues},
		"salary_period": {got.SalaryPeriod, vocab.SalaryPeriodValues},
		"company_type":  {got.CompanyType, vocab.CompanyTypeValues},
		"company_size":  {got.CompanySize, vocab.CompanySizeValues},
	} {
		value, _ := pair[0].(string)
		allowed, _ := pair[1].([]string)
		if value == "" {
			continue
		}
		if !contains(allowed, value) {
			t.Errorf("%s = %q, outside its vocabulary %v", field, value, allowed)
		}
	}

	// Fields the prompt does not ask for must not come back at all — the list is
	// unaskedFields in schema.go, and skills is deliberately NOT on it: both the
	// schema and the prompt request skills (see buildSystemPrompt's "Other keys"
	// line). Asserting otherwise failed identically on gpt-4o-mini under a strict
	// schema and on glm-4.7-flash under plain JSON mode, which is the shape of a
	// test that outlived its contract rather than of a model that misbehaves.
	if got.Seniority != "" || got.WorkMode != "" {
		t.Errorf("model returned dictionary-covered facets: seniority=%q work_mode=%q",
			got.Seniority, got.WorkMode)
	}
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}

	return false
}
