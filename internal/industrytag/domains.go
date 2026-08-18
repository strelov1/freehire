package industrytag

import "slices"

// domainIndustry translates the job-derived domain vocabulary (vocab.DomainValues,
// ~20 coarse verticals the enrichment LLM emits per job) into this package's curated
// industry vocabulary. It exists so the companies catalogue can offer ONE industry
// filter that reaches a company through either source — the curated
// companies.industries an importer wrote, or the companies.domains its own postings
// imply. Nothing is stored: the filter translates and matches both columns.
//
// Written domain→industry because that is the direction with an answer — a domain
// names at most one industry, while an industry may be reachable through several
// domains (none are today, but the inversion below does not assume otherwise).
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

// industryDomains is domainIndustry inverted — the direction the filter asks in,
// since a request names industries and needs the domain values that also mean them.
// Built once at init rather than per query: the table is fixed at compile time.
var industryDomains = func() map[string][]string {
	out := make(map[string][]string, len(domainIndustry))
	for domain, industry := range domainIndustry {
		out[industry] = append(out[industry], domain)
	}
	for _, domains := range out {
		slices.Sort(domains)
	}
	return out
}()

// DomainsForIndustries returns the domain values through which the given canonical
// industries can also be recognised, sorted and de-duplicated so the result can be
// compared and written into a text[] parameter directly.
//
// Like Canonicalize it is dict-only and never returns nil: an industry this mapping
// does not cover — including one that is not an industry at all — contributes
// nothing, and an empty result is an empty slice.
func DomainsForIndustries(industries []string) []string {
	seen := make(map[string]struct{}, len(industries))
	for _, industry := range industries {
		for _, domain := range industryDomains[industry] {
			seen[domain] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for domain := range seen {
		out = append(out, domain)
	}
	slices.Sort(out)
	return out
}
