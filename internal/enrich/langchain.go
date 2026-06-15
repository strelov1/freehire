package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// defaultEnrichTimeout bounds a single LLM call. Without it a stalled gateway hangs
// the worker indefinitely (a run-once worker holding its cron flock open forever,
// stalling the whole queue). The lease/retry machinery then reclaims the job.
const defaultEnrichTimeout = 90 * time.Second

// maxDescriptionRunes caps the job description sent to the model. Descriptions are
// attacker-influenced (scraped/extracted), so bounding the length keeps a single
// oversized posting from amplifying per-call token cost.
const maxDescriptionRunes = 24000

// truncateRunes returns s clamped to at most limit runes, never splitting a rune.
func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}

// LangChainProvider implements Provider over any OpenAI-compatible endpoint via
// langchaingo. The endpoint, credential, and model are injected at construction;
// the model is asked for a JSON object matching the Enrichment contract.
type LangChainProvider struct {
	llm     llms.Model
	timeout time.Duration
}

// NewLangChainProvider builds a provider against an OpenAI-compatible endpoint.
// baseURL points at the gateway/provider (e.g. a LiteLLM endpoint), apiKey is the
// bearer credential, model is the model id to call. No provider is hard-coded —
// any OpenAI-compatible backend works.
func NewLangChainProvider(baseURL, apiKey, model string) (*LangChainProvider, error) {
	llm, err := openai.New(
		openai.WithBaseURL(baseURL),
		openai.WithToken(apiKey),
		openai.WithModel(model),
	)
	if err != nil {
		return nil, fmt.Errorf("enrich: build llm client: %w", err)
	}
	return &LangChainProvider{llm: llm, timeout: defaultEnrichTimeout}, nil
}

// Enrich asks the model for a structured Enrichment for the job and parses the JSON
// response. It does not validate the result — the caller validates before persisting.
// The LLM call is bounded by the provider timeout so a stalled gateway cannot hang
// the worker.
func (p *LangChainProvider) Enrich(ctx context.Context, job JobInput) (Enrichment, error) {
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
		llms.TextParts(llms.ChatMessageTypeHuman, userPrompt(job)),
	}
	resp, err := p.llm.GenerateContent(ctx, messages, llms.WithJSONMode())
	if err != nil {
		return Enrichment{}, fmt.Errorf("enrich: generate: %w", err)
	}
	if len(resp.Choices) == 0 {
		return Enrichment{}, fmt.Errorf("enrich: model returned no choices")
	}
	return parseEnrichment(resp.Choices[0].Content)
}

// parseEnrichment unmarshals a model's JSON response into an Enrichment, tolerating
// a markdown code fence some models add despite JSON mode.
func parseEnrichment(raw string) (Enrichment, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	var e Enrichment
	if err := json.Unmarshal([]byte(s), &e); err != nil {
		return Enrichment{}, fmt.Errorf("enrich: parse response: %w", err)
	}
	return e, nil
}

// systemPrompt instructs the model to emit only stated fields and to draw enum
// values from the controlled vocabularies — the same lists Validate enforces, so
// the prompt and the validator can never drift.
var systemPrompt = buildSystemPrompt()

func buildSystemPrompt() string {
	var b strings.Builder
	b.WriteString("You extract structured facts from an IT job posting and return ONLY a JSON object.\n")
	b.WriteString("Include a key only when the posting clearly states it; omit anything not stated. Never guess.\n")
	b.WriteString("Enum fields MUST use exactly one of the allowed values below.\n\n")
	b.WriteString("Allowed enum values:\n")

	enum := func(field string, vals []string) {
		fmt.Fprintf(&b, "- %s: %s\n", field, strings.Join(vals, ", "))
	}
	enum("work_mode", WorkModeValues)
	enum("regions (array)", RegionValues)
	enum("employment_type", EmploymentTypeValues)
	enum("relocation", RelocationValues)
	enum("salary_period", SalaryPeriodValues)
	enum("seniority", SeniorityValues)
	enum("english_level", EnglishLevelValues)
	enum("education_level", EducationLevelValues)
	enum("category", CategoryValues)
	enum("domains (array)", DomainValues)
	enum("company_type", CompanyTypeValues)
	enum("company_size", CompanySizeValues)

	b.WriteString("\nOther keys (omit when unstated): ")
	b.WriteString("visa_sponsorship (boolean), countries (array of ISO 3166-1 alpha-2), ")
	b.WriteString("cities (array of strings), timezone_note (string), ")
	b.WriteString("salary_min (int), salary_max (int), salary_currency (ISO 4217), ")
	b.WriteString("experience_years_min (non-negative int), ")
	b.WriteString("skills (array of lowercase tokens, e.g. go, postgresql), ")
	b.WriteString("posting_language (ISO 639-1, e.g. en, uk, ru).\n")

	b.WriteString("\nregions is the job's geographic area, for ANY work mode — a remote role's ")
	b.WriteString("reach or an onsite/hybrid role's location: ")
	b.WriteString("use 'global' ONLY when the posting explicitly says the role is open worldwide / ")
	b.WriteString("anywhere / from any country; otherwise list the region(s) or country code(s) ")
	b.WriteString("the role covers, from the allowed values. Omit when unstated (unknown is not global).\n")
	b.WriteString("\nIf the Location field is empty, the URL path may still encode the location ")
	b.WriteString("(e.g. a city as the first slug segment); read it as a location signal.\n")
	return b.String()
}

func userPrompt(job JobInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", job.Title)
	fmt.Fprintf(&b, "Company: %s\n", job.Company)
	fmt.Fprintf(&b, "Location: %s\n", job.Location)
	// The URL path can encode the location/role on some ATS even when the Location
	// field is empty (e.g. SuccessFactors /job/<City>-<Title>/<id>/).
	fmt.Fprintf(&b, "URL: %s\n", job.URL)
	// Source-provided remote hint (from the ATS API or the location text) — a
	// prior for the model, not a guarantee of scope.
	fmt.Fprintf(&b, "Remote flag: %t\n", job.Remote)
	fmt.Fprintf(&b, "Description:\n%s\n", truncateRunes(job.Description, maxDescriptionRunes))
	return b.String()
}
