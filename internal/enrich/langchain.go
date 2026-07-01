package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/llm"
)

// maxDescriptionRunes caps the job description sent to the model. Descriptions are
// attacker-influenced (scraped/extracted), so bounding the length keeps a single
// oversized posting from amplifying per-call token cost.
const maxDescriptionRunes = 24000

// LangChainProvider implements Provider over any OpenAI-compatible endpoint via
// the shared llm client. The model is asked for a JSON object matching the
// Enrichment contract.
type LangChainProvider struct {
	client *llm.Client
}

// NewLangChainProvider builds a provider against an OpenAI-compatible endpoint.
// No provider is hard-coded — any OpenAI-compatible backend works.
func NewLangChainProvider(baseURL, apiKey, model string) (*LangChainProvider, error) {
	c, err := llm.New(baseURL, apiKey, model)
	if err != nil {
		return nil, fmt.Errorf("enrich: %w", err)
	}
	return &LangChainProvider{client: c}, nil
}

// Enrich asks the model for a structured Enrichment for the job and parses the JSON
// response. It does not validate the result — the caller validates before persisting.
func (p *LangChainProvider) Enrich(ctx context.Context, job JobInput) (Enrichment, error) {
	raw, err := p.client.GenerateJSON(ctx, systemPrompt, userPrompt(job))
	if err != nil {
		return Enrichment{}, fmt.Errorf("enrich: %w", err)
	}
	return parseEnrichment(raw)
}

// errUnparseableResponse marks a model response that wasn't valid JSON. It is
// deterministic for the prompt, so the runner skips its in-process retry on it
// (the outbox attempts counter still re-tries on the next cron run, drawing a
// fresh sample). Transport failures, by contrast, are worth an immediate retry.
var errUnparseableResponse = errors.New("enrich: unparseable model response")

// parseEnrichment unmarshals a model's already-fence-stripped JSON response.
func parseEnrichment(raw string) (Enrichment, error) {
	var e Enrichment
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &e); err != nil {
		return Enrichment{}, fmt.Errorf("%w: %v", errUnparseableResponse, err)
	}
	return e, nil
}

// systemPrompt instructs the model to emit only stated fields and to draw the
// SERVED enum values from the controlled vocabularies — the same lists Validate
// enforces. The dictionary-covered discovery facets (work_mode, regions,
// seniority, category, employment_type, education_level) are deliberately relaxed:
// the prompt invites a novel label when none fits, and Validate does not reject it
// (it is unserved discovery material), so prompt and validator diverge there on purpose.
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

	// Discovery facets: these six (plus the open countries/skills) are served from
	// our own dictionaries, not from your value, so they are a discovery signal.
	// Prefer an allowed value, but emit your own label when none fits — this surfaces
	// vocabulary we are missing. The other enum fields above stay strict.
	b.WriteString("\nException for work_mode, regions, seniority, category, employment_type, ")
	b.WriteString("education_level: prefer an allowed value above, but if none accurately fits, ")
	b.WriteString("you MAY return a concise lowercase label of your own (e.g. seniority ")
	b.WriteString("\"staff_plus\", category \"ml_platform\"). ")
	b.WriteString("Still omit the key when the posting does not state it.\n")

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
	fmt.Fprintf(&b, "Description:\n%s\n", llm.TruncateRunes(job.Description, maxDescriptionRunes))
	return b.String()
}
