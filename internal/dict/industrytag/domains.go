package industrytag

import "slices"

// domainIndustry translates the job-derived domain vocabulary (vocab.DomainValues)
// into this package's industry vocabulary, so one companies filter reaches a company
// through either companies.industries or companies.industries_derived
// (the facet recount in company_facets.sql materializes this translation; see DomainIndustryPairs).
//
// Written domain→industry because a domain names at most one industry, while an
// industry may be reachable through several domains. "other" maps to nothing (the
// classifier declining to answer); a retired domain such as "saas" falls out by the
// same lookup.
//
// Contested placements follow NAICS and Crunchbase:
//
//   - "mobility" → transportation: ride-hailing is NAICS 485310 (ground passenger
//     transportation), distinct from motor vehicle manufacturing, so "automotive"
//     would misfile taxi platforms.
//   - "media" → entertainment: the alias table already routes digital-media,
//     media-and-entertainment and content-creation there.
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

// DomainIndustryPairs returns the domain→industry mapping as two parallel slices
// sorted by domain, for passing as two text[] query parameters. It is the one place
// this table crosses into SQL, so the facet recount in company_facets.sql cannot drift from it.
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
