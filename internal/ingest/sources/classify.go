package sources

import "github.com/strelov1/freehire/internal/job/job"

// Provider-kind taxonomy for the public status view and the search `source_type` facet. It reuses the adapter markers
// that already exist for other purposes rather than inventing a parallel labelling:
//   - an aggregator adapter crawls many companies (jobstash, nofluffjobs, …);
//   - a boardless non-aggregator serves a single company — its own careers page
//     (apple, google, sber, …);
//   - anything with a per-tenant board is a multi-tenant ATS platform
//     (greenhouse, lever, workday, …);
//   - a social adapter, or the Telegram extraction pipeline, reads community channels.
//
// The four kinds a document can carry are vocab.SourceTypeValues; KindOther is never
// written onto one.
const (
	KindATS        = "ats"
	KindAggregator = "aggregator"
	KindCompany    = "company"
	KindSocial     = "social"
	KindOther      = "other"
)

// nonAdapterKinds places the sources that are not crawl adapters but are known to write
// jobs: the Telegram extraction pipeline (a post with no outbound link is stored under
// this name; one that links to an ATS is stored under that ATS) and the vacancies a
// verified employer publishes here. The remaining non-adapter sources — a moderator's
// manual import, a pasted link — say nothing about where the posting came from.
var nonAdapterKinds = map[string]string{
	"telegram":         KindSocial,
	job.EmployerSource: KindCompany,
}

// ProviderKind classifies a provider by its adapter's markers. A provider absent
// from reg (e.g. a non-adapter source such as a manual import or a Telegram feed)
// is KindOther, so reg must be Taxonomy(): in a crawl registry a keyed adapter is
// absent wherever its credential is unset, and would be misreported as KindOther.
func ProviderKind(reg map[string]Source, provider string) string {
	src, ok := reg[provider]
	if !ok {
		if kind, known := nonAdapterKinds[provider]; known {
			return kind
		}
		return KindOther
	}
	if _, ok := src.(social); ok {
		return KindSocial
	}
	if _, ok := src.(aggregator); ok {
		return KindAggregator
	}
	if _, ok := src.(boardless); ok {
		return KindCompany
	}
	return KindATS
}

// SourceTypes returns the `source_type` a job from each source carries in the search index:
// its ProviderKind, or "" for a source no kind describes. reg must be Taxonomy(), for the
// reason ProviderKind gives. The answers are resolved once, so a caller building documents
// in bulk pays for the taxonomy once rather than per row.
func SourceTypes(reg map[string]Source) func(source string) string {
	types := make(map[string]string, len(reg)+len(nonAdapterKinds))
	for provider := range reg {
		types[provider] = ProviderKind(reg, provider)
	}
	for provider := range nonAdapterKinds {
		types[provider] = ProviderKind(reg, provider)
	}
	// Every other source resolves to "" through the map miss: ProviderKind would call it
	// KindOther, which no document carries.
	return func(source string) string { return types[source] }
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
