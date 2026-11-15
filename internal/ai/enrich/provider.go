package enrich

import "context"

// Version is the enrichment schema version, stamped on every job written
// (jobs.enrichment_version). Bumping it enqueues re-enrichment of older rows.
// v2: round fractional hourly rates instead of decimal-stripping them (26.08 -> 2608).
const Version = 2

// JobInput is the source-shaped view of a job a Provider derives an Enrichment from.
type JobInput struct {
	Title       string
	Company     string
	Location    string
	Remote      bool
	Description string
	// URL is the public posting URL. Some ATS encode the location in the path
	// (SuccessFactors /job/Limburg-Maschinenfuhrer/), so it is a signal when Location is empty.
	URL string
	// GeoPinned: the dictionary already resolved country/region (see GeoPinned), so the
	// prompt drops the geography ask; geoFacet would discard the LLM's copy anyway.
	GeoPinned bool
	// CompanyTypeHint is a vocab.CompanyTypeValues entry from CompanyTypeHints, keyed by
	// company_slug. A posting rarely states the employer's business model, so this feeds
	// the prompt a verified fact. Empty when the company is not curated.
	CompanyTypeHint string
}

// GeoPinned reports whether the dictionary resolved a country, or a region other than
// "global". Mirrors jobview.geoPinned; duplicated because the two layers are independent.
func GeoPinned(countries, regions []string) bool {
	if len(countries) > 0 {
		return true
	}
	for _, r := range regions {
		if r != "global" {
			return true
		}
	}
	return false
}

// Provider derives a structured Enrichment for a job by calling an LLM. Its output is
// untrusted: the caller runs Enrichment.Validate before persisting.
type Provider interface {
	Enrich(ctx context.Context, job JobInput) (Enrichment, error)
}
