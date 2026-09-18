package sources

// Provider-kind taxonomy for the public status view. It reuses the adapter markers
// that already exist for other purposes rather than inventing a parallel labelling:
//   - an aggregator adapter crawls many companies (jobstash, nofluffjobs, …);
//   - a boardless non-aggregator serves a single company — its own careers page
//     (apple, google, sber, …);
//   - anything with a per-tenant board is a multi-tenant ATS platform
//     (greenhouse, lever, workday, …).
const (
	KindATS        = "ats"
	KindAggregator = "aggregator"
	KindCompany    = "company"
	KindOther      = "other"
)

// ProviderKind classifies a provider by its adapter's markers. A provider absent
// from reg (e.g. a non-adapter source such as a manual import or a Telegram feed)
// is KindOther, so reg must be Taxonomy(): in a crawl registry a keyed adapter is
// absent wherever its credential is unset, and would be misreported as KindOther.
func ProviderKind(reg map[string]Source, provider string) string {
	src, ok := reg[provider]
	if !ok {
		return KindOther
	}
	if _, ok := src.(aggregator); ok {
		return KindAggregator
	}
	if _, ok := src.(boardless); ok {
		return KindCompany
	}
	return KindATS
}

// EmployerURLProviders names the providers whose stored job URL really is the employer's own
// page for the posting.
//
// True for an ATS and for a company's own careers site, false for an aggregator — whose URL
// points at the aggregator, not at the employer. The distinction is what lets a surface
// publish source attribution honestly: calling an aggregator's link the employer's own is a
// claim we cannot vouch for, and a consumer that domain-verifies or deduplicates on it would
// be misled by every one of them.
//
// It lives here rather than beside a consumer because it is a fact about SOURCES, and a
// second surface asking the same question must not answer it from its own copy of the rule.
//
// It answers for ALL providers at once, and that shape is the point. A per-provider
// predicate reads better at the call site and costs a full Taxonomy() build every time —
// measured on this tree, 35µs and 394 allocations to construct 223 adapters, which a
// ten-result search would pay ten times over. Callers resolve this once and keep the map.
func EmployerURLProviders() map[string]bool {
	taxonomy := Taxonomy()
	publishes := make(map[string]bool, len(taxonomy))
	for provider := range taxonomy {
		switch ProviderKind(taxonomy, provider) {
		case KindATS, KindCompany:
			publishes[provider] = true
		}
	}
	return publishes
}
