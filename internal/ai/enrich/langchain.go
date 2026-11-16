package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// maxDescriptionRunes caps the description sent to the model; scraped text is
// attacker-influenced, so one oversized posting cannot amplify token cost.
const maxDescriptionRunes = 24000

// maxFieldRunes caps the short untrusted header fields (title, company, location, URL);
// it never fires on a real posting.
const maxFieldRunes = 500

// LangChainProvider implements Provider over any OpenAI-compatible endpoint via the
// shared llm client.
type LangChainProvider struct {
	client *llm.Client
}

// NewLangChainProvider wraps a prebuilt llm.Client as an enrichment Provider.
func NewLangChainProvider(c *llm.Client) *LangChainProvider {
	return &LangChainProvider{client: c}
}

// Enrich asks the model for a structured Enrichment and parses the JSON response. It does
// not validate the result.
func (p *LangChainProvider) Enrich(ctx context.Context, job JobInput) (Enrichment, error) {
	askGeo := !job.GeoPinned

	schema, name, err := requestSchema(askGeo)
	if err != nil {
		return Enrichment{}, err
	}
	raw, err := p.client.GenerateJSON(ctx, buildSystemPrompt(askGeo), userPrompt(job),
		llm.WithSchema(name, schema))
	if err != nil {
		return Enrichment{}, fmt.Errorf("enrich: %w", err)
	}
	return parseEnrichment(raw)
}

// errUnparseableResponse marks a model response that wasn't valid JSON. The runner skips
// its in-process retry on it; the next cron run draws a fresh sample.
var errUnparseableResponse = errors.New("enrich: unparseable model response")

// parseEnrichment unmarshals a model's already-fence-stripped JSON response.
func parseEnrichment(raw string) (Enrichment, error) {
	var e Enrichment
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &e); err != nil {
		return Enrichment{}, fmt.Errorf("%w: %v", errUnparseableResponse, err)
	}
	return e, nil
}

// buildSystemPrompt asks the model to null unstated fields and to draw served enum values
// from the vocabularies Validate enforces. Dictionary-covered discovery facets are relaxed
// on purpose: the prompt invites a novel label and Validate does not reject it.
//
// askGeo is false when the dictionary already pinned countries/regions (see GeoPinned);
// the geo enum, its own-label exception, the "Other keys" mention and the geographic-area
// paragraph are then dropped.
func buildSystemPrompt(askGeo bool) string {
	var b strings.Builder
	b.WriteString("You read a job posting and return ONLY a JSON object.\n")
	// summary is the one synthesized field: stating it first and exempting it from the null
	// rule stops a budget model from emptying it.
	b.WriteString("ALWAYS include \"summary\" as the FIRST key: a 1-2 sentence, plain-English synopsis of ")
	b.WriteString("the role — what the person does day to day and the core technologies. You WRITE this ")
	b.WriteString("from the posting (the one field you synthesize, not extract); keep it under 400 ")
	b.WriteString("characters and never invent facts the posting does not support.\n")
	b.WriteString("ALWAYS fill \"category\" from the role the posting describes.\n")
	b.WriteString("For every OTHER key: fill it only when the posting clearly states it; return null for anything not stated. Never guess.\n")
	b.WriteString("Enum fields MUST use exactly one of the allowed values below.\n\n")
	b.WriteString("Allowed enum values:\n")

	enum := func(field string, vals []string) {
		fmt.Fprintf(&b, "- %s: %s\n", field, strings.Join(vals, ", "))
	}
	// work_mode, seniority, employment_type, education_level and english_level are not
	// requested: jobview serves them from the dictionaries (internal/job/jobderive).
	if askGeo {
		enum("regions (array)", vocab.RegionValues)
	}
	enum("relocation", vocab.RelocationValues)
	enum("salary_period", vocab.SalaryPeriodValues)
	// domains is the industry, not the business model; glossed per value.
	b.WriteString("- domains (array) — the company's industry/vertical (what it does), multiple allowed:\n")
	for _, d := range vocab.DomainValues {
		fmt.Fprintf(&b, "    %s: %s\n", d, vocab.DomainGloss[d])
	}
	// company_type is glossed per value so the model can tell outsource from outstaff.
	b.WriteString("- company_type — the company's business model:\n")
	for _, ct := range vocab.CompanyTypeValues {
		fmt.Fprintf(&b, "    %s: %s\n", ct, vocab.CompanyTypeGloss[ct])
	}
	// A curated hint (CompanyTypeHints) overrides the "never guess" default for this field.
	b.WriteString("  If the input states \"Known company type\", that is a VERIFIED fact — ")
	b.WriteString("use it as company_type unless the posting itself explicitly contradicts it.\n")
	enum("company_size", vocab.CompanySizeValues)
	// category files the role itself: every posting implies one, so the null rule above
	// exempts it (#4412).
	enum("category (the role's job family; \"other\" only when none fits)", vocab.CategoryValues)

	if askGeo {
		// countries/regions are the only facets where the model may coin its own label
		// (dict-then-LLM hybrid via jobview.geoFacet).
		b.WriteString("\nException for countries and regions: prefer an allowed value above, but if none ")
		b.WriteString("accurately fits, you MAY return a concise lowercase label of your own. ")
		b.WriteString("Still return null when the posting does not state it.\n")
	}

	b.WriteString("\nOther keys (null when unstated): ")
	b.WriteString("visa_sponsorship (boolean), ")
	if askGeo {
		b.WriteString("countries (array of ISO 3166-1 alpha-2), ")
	}
	b.WriteString("cities (array of strings), timezone_note (string), ")
	b.WriteString("salary_min (int), salary_max (int), salary_currency (ISO 4217), ")
	b.WriteString("skills (array of lowercase tokens, e.g. go, postgresql).\n")

	b.WriteString("\n\"requirements\" (array, max 30): the posting's own explicit requirements ")
	b.WriteString("(skills, tools, experience, must/nice-to-have qualifications), each an object ")
	b.WriteString("with \"text\" (the requirement, short) and \"priority\" (\"required\" or ")
	b.WriteString("\"preferred\"). Base this ONLY on the job posting — you have no CV to compare ")
	b.WriteString("against. Return an empty array if the posting states no explicit requirements.\n")

	// Salary guard: told the field is an int, the model strips the decimal of a fractional
	// rate ($26.08 -> 2608); a concrete counter-example makes it round.
	b.WriteString("\nsalary_min and salary_max are WHOLE units of the currency. ")
	b.WriteString("Round a fractional rate to the nearest whole unit and NEVER strip the ")
	b.WriteString("decimal point: an hourly \"$26.08\" is 26 (with salary_period=hour), never 2608.\n")
	// Polish B2B day rates ("1000-1300 PLN/MD") were read as monthly.
	b.WriteString("A rate per man-day (\"PLN/MD\", \"zł/md\", \"/MD\", \"stawka dzienna\") is ")
	b.WriteString("salary_period=day, never month.\n")

	if askGeo {
		b.WriteString("\nregions is the job's geographic area, for ANY work mode — a remote role's ")
		b.WriteString("reach or an onsite/hybrid role's location: ")
		b.WriteString("use 'global' ONLY when the posting explicitly says the role is open worldwide / ")
		b.WriteString("anywhere / from any country; otherwise list the region(s) or country code(s) ")
		b.WriteString("the role covers, from the allowed values. Return null when unstated (unknown is not global).\n")
	}
	b.WriteString("\nIf the Location field is empty, the URL path may still encode the location ")
	b.WriteString("(e.g. a city as the first slug segment); read it as a location signal.\n")
	return b.String()
}

func userPrompt(job JobInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", llm.TruncateRunes(job.Title, maxFieldRunes))
	fmt.Fprintf(&b, "Company: %s\n", llm.TruncateRunes(job.Company, maxFieldRunes))
	fmt.Fprintf(&b, "Location: %s\n", llm.TruncateRunes(job.Location, maxFieldRunes))
	// Some ATS encode the location/role in the URL path (SuccessFactors /job/<City>-<Title>/<id>/).
	fmt.Fprintf(&b, "URL: %s\n", llm.TruncateRunes(job.URL, maxFieldRunes))
	// Source-provided remote hint: a prior, not a guarantee of scope.
	fmt.Fprintf(&b, "Remote flag: %t\n", job.Remote)
	if job.CompanyTypeHint != "" {
		fmt.Fprintf(&b, "Known company type: %s\n", job.CompanyTypeHint)
	}
	fmt.Fprintf(&b, "Description:\n%s\n", llm.TruncateRunes(job.Description, maxDescriptionRunes))
	return b.String()
}
