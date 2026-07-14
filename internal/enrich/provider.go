package enrich

import "context"

// Version is the current enrichment schema version. The enrichment command stamps
// it on every job it writes (jobs.enrichment_version) and uses it as the target
// version when enqueuing work; bumping it enqueues re-enrichment of every job that
// was written under an older version.
// v2: salary prompt guard — round fractional hourly rates to whole currency units
// instead of decimal-stripping them (26.08 -> 2608). Re-enriches corrupted rows.
const Version = 2

// JobInput is the raw, source-shaped view of a job that a Provider reads to derive
// an Enrichment. It carries exactly the fields the model extracts from — no
// enrichment or provenance fields.
type JobInput struct {
	Title       string
	Company     string
	Location    string
	Remote      bool
	Description string
	// URL is the public posting URL. Some ATS encode the location (and sometimes
	// the role) in the URL path — e.g. SuccessFactors /job/Limburg-Maschinenfuhrer/
	// — so it is a location signal even when the structured Location is empty.
	URL string
}

// Provider derives a structured Enrichment for a job by calling an LLM.
//
// A Provider is not trusted to be correct: the returned Enrichment is expected to
// honor the controlled vocabularies, but the caller validates it with
// Enrichment.Validate before persisting and rejects anything out of vocabulary.
type Provider interface {
	Enrich(ctx context.Context, job JobInput) (Enrichment, error)
}
