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

// PublishesEmployerURL reports whether a provider's stored job URL really is the employer's
// own page for the posting.
//
// It is true for an ATS and for a company's own careers site, and false for an aggregator —
// whose URL points at the aggregator, not at the employer. The distinction is what lets a
// surface publish source attribution honestly: calling an aggregator's link the employer's
// own is a claim we cannot vouch for, and a consumer that domain-verifies or deduplicates on
// it would be misled by every one of them.
//
// It lives here rather than beside a consumer because it is a fact about SOURCES, and a
// second surface asking the same question must not answer it from its own copy of the rule.
func PublishesEmployerURL(provider string) bool {
	switch ProviderKind(Taxonomy(), provider) {
	case KindATS, KindCompany:
		return true
	}
	return false
}
