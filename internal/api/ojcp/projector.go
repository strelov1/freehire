package ojcp

import (
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/job/outboundurl"
)

// NewProjector builds a Projector for this deployment. It resolves, once, which sources
// publish a URL that really is the employer's own page — a per-provider fact the source
// taxonomy already knows and every caller would otherwise have to rediscover.
//
// submittable names the ATS providers this deployment can complete an application on
// without a person; see Projector.Submittable for what belongs in it.
func NewProjector(origin string, submittable map[string]bool) Projector {
	return Projector{
		Origin:                origin,
		Submittable:           submittable,
		CanonicalURLProviders: canonicalURLProviders(),
	}
}

// canonicalURLProviders is every source whose stored URL points at the employer's own
// posting: a multi-tenant ATS (greenhouse, lever, workday) or a company's own careers page
// (apple, sber).
//
// An AGGREGATOR is deliberately excluded. Its stored URL is a tracking redirect on its own
// domain — adzuna's is `adzuna.com/land/ad/<id>` — and OJCP defines `official_job_url` as
// "the canonical public web URL … on the employer's official careers site or ATS",
// telling agents to verify its registrable domain. Publishing a redirect there hands an
// agent an anchor that fails by construction, and breaks cross-provider deduplication for
// everyone. Such a posting keeps its `url`, the field whose own description permits
// third-party boards and aggregators.
//
// KindOther — a manual import, a Telegram feed — is excluded for the weaker reason that
// nothing vouches for where its URL points.
func canonicalURLProviders() map[string]bool {
	// The rule itself lives in internal/ingest/sources, because it is a fact about sources
	// and a second surface now asks the same question. It used to be spelled out here; two
	// copies of "which providers publish the employer's own URL" would be two answers the
	// day the kinds change.
	return sources.EmployerURLProviders()
}

// officialJobURL is the employer's own page for the posting, or "" where we cannot vouch
// that the stored URL is one.
//
// The tracking parameter is stripped rather than left in place: jobview stamps
// utm_source on every URL it serves, and this field is what agents deduplicate and
// domain-verify against, so the tag defeats both purposes. Our own `url` keeps it.
func (p Projector) officialJobURL(j jobview.Job) string {
	if !p.CanonicalURLProviders[j.Source] {
		return ""
	}
	return outboundurl.Untag(j.URL)
}
