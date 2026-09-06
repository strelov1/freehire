package industrytag

import "slices"

// domainIndustry translates the job-derived domain vocabulary (vocab.DomainValues,
// ~20 coarse verticals the enrichment LLM emits per job) into this package's curated
// industry vocabulary. It exists so the companies catalogue can offer ONE industry
// filter that reaches a company through either source — the curated
// companies.industries an importer wrote, or companies.industries_derived,
// RefreshCompanyFacets' materialized translation of the company's own domains (see
// DomainIndustryPairs, and the limit-derived-industry-domain-count change for why
// that translation is materialized rather than applied per query).
//
// Written domain→industry because that is the direction with an answer — a domain
// names at most one industry, while an industry may be reachable through several
// domains.
//
// One of the twenty domains maps to nothing: "other" is the classifier declining to
// answer, not a vertical. A domain absent from vocab.DomainValues entirely — "saas",
// retired for naming a business model rather than a vertical, but still present on
// rows enriched before it went — falls out by the same lookup, with no special case.
//
// Where a placement is contested, it is settled against NAICS and Crunchbase rather
// than by argument. Two were:
//
//   - "mobility" → transportation. Ride-hailing is NAICS 485310, under 485 Transit
//     and Ground Passenger Transportation, explicitly distinct from 3361 Motor
//     Vehicle Manufacturing; Crunchbase has no "mobility" category at all, making
//     Transportation the umbrella with Ride Sharing and Automotive beneath it. So
//     "automotive" would have filed taxi platforms under vehicle manufacturing.
//   - "media" → entertainment. The alias table beside this one already routes
//     digital-media, media-and-entertainment, media-and-communications and
//     content-creation there, and Crunchbase groups the two as one. The domain was
//     being held to a stricter standard than its own synonyms.
var domainIndustry = map[string]string{
	"adtech":        "adtech",
	"ai":            "ai",
	"climatetech":   "climate-tech",
	"crypto":        "crypto",
	"cybersecurity": "cybersecurity",
	"devtools":      "developer-tools",
	"ecommerce":     "ecommerce",
	"edtech":        "edtech",
	"fintech":       "fintech",
	"gambling":      "gambling",
	"gamedev":       "gaming",
	"govtech":       "government",
	"healthcare":    "healthcare",
	"hrtech":        "hr-tech",
	"logistics":     "logistics",
	"media":         "entertainment",
	"mobility":      "transportation",
	"proptech":      "proptech",
	"travel":        "travel",
}

// DomainIndustryPairs returns the domain→industry mapping as two parallel, sorted-by
// -domain slices, `pairs[i]` being `(domains[i], industries[i])` — the shape a caller
// passes as two `text[]` query parameters rather than duplicating the table in SQL.
// It is the one place this table crosses into a query, so a materialization that
// needs the mapping (RefreshCompanyFacets) reads it from here instead of growing a
// second, driftable copy.
func DomainIndustryPairs() (domains, industries []string) {
	domains = make([]string, 0, len(domainIndustry))
	for domain := range domainIndustry {
		domains = append(domains, domain)
	}
	slices.Sort(domains)

	industries = make([]string, len(domains))
	for i, domain := range domains {
		industries[i] = domainIndustry[domain]
	}
	return domains, industries
}
