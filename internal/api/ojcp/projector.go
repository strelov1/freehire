package ojcp

import (
	"github.com/strelov1/freehire/internal/ingest/sources"
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
		CanonicalURLProviders: sources.EmployerURLProviders(),
	}
}
