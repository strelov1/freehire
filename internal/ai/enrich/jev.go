package enrich

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/wawan93/gojev"
)

// JevProvider implements Provider using the Typesafe AI Jev model.
type JevProvider struct {
	client *gojev.Client
}

// NewJevProvider creates a new JevProvider with the given Typesafe API client.
func NewJevProvider(client *gojev.Client) *JevProvider {
	return &JevProvider{client: client}
}

// Enrich uses Jev's SystemOne to classify the job posting against our controlled vocabularies.
func (p *JevProvider) Enrich(ctx context.Context, job JobInput) (Enrichment, error) {
	state := map[string]string{
		"title":       job.Title,
		"company":     job.Company,
		"location":    job.Location,
		"url":         job.URL,
		"description": job.Description,
	}
	if job.CompanyTypeHint != "" {
		state["company_type_hint"] = job.CompanyTypeHint
	}
	if job.Remote {
		state["remote_hint"] = "true"
	}

	builder := gojev.NewSystemOneBuilder(state)

	// Single-value fields
	builder.Choice("relocation", "What is the relocation policy for this role?", toChoiceMap(vocab.RelocationValues))
	builder.Choice("salary_period", "What is the stated salary period?", toChoiceMap(vocab.SalaryPeriodValues))

	builder.Choice("company_size", "What is the size of the company (number of employees)?", toChoiceMap(vocab.CompanySizeValues))

	companyTypeGlossMap := toChoiceMapWithGloss(vocab.CompanyTypeValues, vocab.CompanyTypeGloss)
	builder.Choice("company_type", "What is the company's business model or type? If 'company_type_hint' is provided, use it as a verified fact unless the posting explicitly contradicts it.", companyTypeGlossMap)

	builder.Noul("visa_sponsorship", "Does the company offer visa sponsorship for this position?", &gojev.NoulCriteria{True: "Yes", False: "No"})

	// We'll also ask for regions if !job.GeoPinned, but it's an array. Jev Choice is single value.
	// Since regions is usually one value, we could model it as a choice, but for now we skip arrays
	// or we can just ask for it as a choice and append it.
	if !job.GeoPinned {
		builder.Choice("region", "What geographic region does this role cover? Use 'global' ONLY if explicitly open worldwide (unknown is not global).", toChoiceMap(vocab.RegionValues))
	}

	req := builder.Build()
	resp, err := p.client.SystemOne(ctx, req)
	if err != nil {
		return Enrichment{}, fmt.Errorf("enrich (jev): %w", err)
	}

	var e Enrichment

	if a := resp.Choice("relocation"); a != nil && a.Choice != "null" && a.Choice != "none" {
		e.Relocation = a.Choice
	}
	if a := resp.Choice("salary_period"); a != nil && a.Choice != "null" && a.Choice != "none" {
		e.SalaryPeriod = a.Choice
	}
	if a := resp.Choice("company_size"); a != nil && a.Choice != "null" && a.Choice != "none" {
		e.CompanySize = a.Choice
	}
	if a := resp.Choice("company_type"); a != nil && a.Choice != "null" && a.Choice != "none" {
		e.CompanyType = a.Choice
	}

	if a := resp.Noul("visa_sponsorship"); a != nil {
		if a.Noul > 0 { // Yes
			b := true
			e.VisaSponsorship = &b
		} else if a.Noul < 0 { // No
			b := false
			e.VisaSponsorship = &b
		}
	}

	if !job.GeoPinned {
		if a := resp.Choice("region"); a != nil && a.Choice != "null" && a.Choice != "none" {
			e.Regions = []string{a.Choice}
		}
	}

	return e, nil
}

func toChoiceMap(values []string) map[string]gojev.JSONContent {
	m := make(map[string]gojev.JSONContent)
	for _, v := range values {
		m[v] = v
	}
	m["null"] = "Not stated in the posting"
	return m
}

func toChoiceMapWithGloss(values []string, gloss map[string]string) map[string]gojev.JSONContent {
	m := make(map[string]gojev.JSONContent)
	for _, v := range values {
		if g, ok := gloss[v]; ok {
			m[v] = g
		} else {
			m[v] = v
		}
	}
	m["null"] = "Not stated in the posting"
	return m
}
