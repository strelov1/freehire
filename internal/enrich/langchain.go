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

// NewLangChainProvider wraps a prebuilt llm.Client (constructed once via
// llm.NewClient, which also wires optional tracing) as an enrichment Provider.
func NewLangChainProvider(c *llm.Client) *LangChainProvider {
	return &LangChainProvider{client: c}
}

// Enrich asks the model for a structured Enrichment for the job and parses the JSON
// response. It does not validate the result — the caller validates before persisting.
func (p *LangChainProvider) Enrich(ctx context.Context, job JobInput) (Enrichment, error) {
	raw, err := p.client.GenerateJSON(ctx, buildSystemPrompt(!job.GeoPinned), userPrompt(job))
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

// buildSystemPrompt instructs the model to emit only stated fields and to draw the
// SERVED enum values from the controlled vocabularies — the same lists Validate
// enforces. The dictionary-covered discovery facets (work_mode, regions,
// seniority, category, employment_type, education_level, english_level) are
// deliberately relaxed: the prompt invites a novel label when none fits, and Validate
// does not reject it (it is unserved discovery material), so prompt and validator
// diverge there on purpose.
//
// askGeo is false when the deterministic dictionary already pinned the job's
// countries/regions (see GeoPinned): geoFacet then discards the LLM's copy, so asking
// for it only burns tokens. The geo enum, its own-label exception, its "Other keys"
// mention, and the geographic-area paragraph are dropped in that case. It stays true
// when the dictionary left geography unpinned and the LLM fills the bucket.
func buildSystemPrompt(askGeo bool) string {
	var b strings.Builder
	b.WriteString("You read an IT job posting and return ONLY a JSON object.\n")
	// summary is the one SYNTHESIZED field and must lead: stating it first, and
	// exempting it from the omit rule up front, stops a budget model from dropping it
	// under the "omit anything not stated" directive that governs every other key.
	b.WriteString("ALWAYS include \"summary\" as the FIRST key: a 1-2 sentence, plain-English synopsis of ")
	b.WriteString("the role — what the person does day to day and the core technologies. You WRITE this ")
	b.WriteString("from the posting (the one field you synthesize, not extract); keep it under 400 ")
	b.WriteString("characters and never invent facts the posting does not support.\n")
	b.WriteString("For every OTHER key: include it only when the posting clearly states it; omit anything not stated. Never guess.\n")
	b.WriteString("Enum fields MUST use exactly one of the allowed values below.\n\n")
	b.WriteString("Allowed enum values:\n")

	enum := func(field string, vals []string) {
		fmt.Fprintf(&b, "- %s: %s\n", field, strings.Join(vals, ", "))
	}
	// work_mode, seniority, category, employment_type, education_level, and
	// english_level are deliberately NOT requested: jobview serves them from the
	// deterministic dictionaries (internal/jobderive), so the LLM's copies were never
	// served — asking for them only burned output tokens (see enrich-prompt-trim).
	if askGeo {
		enum("regions (array)", RegionValues)
	}
	enum("relocation", RelocationValues)
	enum("salary_period", SalaryPeriodValues)
	enum("domains (array)", DomainValues)
	enum("company_type", CompanyTypeValues)
	enum("company_size", CompanySizeValues)

	if askGeo {
		// Discovery facets: countries/regions are served as a dict-then-LLM hybrid (the
		// LLM fills the unpinned geographic bucket via jobview.geoFacet), so they are the
		// sole facets we still let the model coin its own label for. The other enum fields
		// above stay strict.
		b.WriteString("\nException for countries and regions: prefer an allowed value above, but if none ")
		b.WriteString("accurately fits, you MAY return a concise lowercase label of your own. ")
		b.WriteString("Still omit the key when the posting does not state it.\n")
	}

	b.WriteString("\nOther keys (omit when unstated): ")
	b.WriteString("visa_sponsorship (boolean), ")
	if askGeo {
		b.WriteString("countries (array of ISO 3166-1 alpha-2), ")
	}
	b.WriteString("cities (array of strings), timezone_note (string), ")
	b.WriteString("salary_min (int), salary_max (int), salary_currency (ISO 4217).\n")

	// Salary guard: the model, told the field is an int, otherwise decimal-strips a
	// fractional hourly rate ($26.08 -> 2608), inflating it 100x. A concrete
	// counter-example is what makes a budget model round instead of strip.
	b.WriteString("\nsalary_min and salary_max are WHOLE units of the currency. ")
	b.WriteString("Round a fractional rate to the nearest whole unit and NEVER strip the ")
	b.WriteString("decimal point: an hourly \"$26.08\" is 26 (with salary_period=hour), never 2608.\n")

	if askGeo {
		b.WriteString("\nregions is the job's geographic area, for ANY work mode — a remote role's ")
		b.WriteString("reach or an onsite/hybrid role's location: ")
		b.WriteString("use 'global' ONLY when the posting explicitly says the role is open worldwide / ")
		b.WriteString("anywhere / from any country; otherwise list the region(s) or country code(s) ")
		b.WriteString("the role covers, from the allowed values. Omit when unstated (unknown is not global).\n")
	}
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
