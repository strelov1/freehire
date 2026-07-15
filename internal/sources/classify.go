package sources

// Provider-kind taxonomy for the public status view. It reuses the adapter markers
// that already exist for other purposes rather than inventing a parallel labelling:
//   - an aggregator adapter crawls many companies (jobstash, justjoin, …);
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
// is KindOther. Passing All(nil) as reg is safe — the marker assertions never touch
// the transport.
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
