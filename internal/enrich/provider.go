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
	// GeoPinned is the one derived signal on this otherwise source-shaped input: the
	// deterministic dictionary already resolved the job's country/region (see
	// GeoPinned). When true, the prompt drops the countries/regions ask — geoFacet
	// would discard the LLM's copy anyway — so the model spends no tokens on geography
	// we already know.
	GeoPinned bool
}

// GeoPinned reports whether the deterministic dictionary resolved a concrete place for
// a job: a country, or a region more specific than the open-anywhere "global" bucket.
// It mirrors jobview.geoPinned (the read-side hybrid that discards the LLM's geo when
// the dictionary pinned one); the two layers are independent, so the small predicate is
// duplicated rather than shared. A pinned job needs no LLM geography in its prompt.
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

// Provider derives a structured Enrichment for a job by calling an LLM.
//
// A Provider is not trusted to be correct: the returned Enrichment is expected to
// honor the controlled vocabularies, but the caller validates it with
// Enrichment.Validate before persisting and rejects anything out of vocabulary.
type Provider interface {
	Enrich(ctx context.Context, job JobInput) (Enrichment, error)
}
