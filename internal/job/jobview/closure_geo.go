package jobview

import "slices"

// MergeClosureGeography widens a job's geography facets with the union across every
// OPEN row in its duplicate closure, so a collapsed multi-city role stays findable by
// every place its hidden rows are open in. Each facet becomes the sorted, deduped union
// of the document's own values and the closure's; an empty slice leaves it unchanged.
//
// Membership is the closure, NOT a shared role_fingerprint: two dedup passes suppress
// rows whose fingerprints differ from their canon's by construction.
//
// The search document embeds Job, so index writers and the job page share this union.
//
// Every writer of a job document must call this, not only the full reindex: the push is
// a field-level update and these four facets are always in the payload, so a writer
// that skips the union overwrites the widened values. The reindex reads
// DuplicateClosureGeoAll; the per-wave writers read DuplicateClosureGeoFor.
func (j *Job) MergeClosureGeography(g ClosureGeo) {
	j.Countries = unionSorted(j.Countries, g.Countries)
	j.Regions = unionSorted(j.Regions, g.Regions)
	j.Cities = unionSorted(j.Cities, g.Cities)
	j.Metros = unionSorted(j.Metros, g.Metros)
}

// ClosureGeo is the geography union across a document's duplicate closure.
type ClosureGeo struct{ Countries, Regions, Cities, Metros []string }

// unionSorted returns the sorted, deduped union of two facet slices. A nil result
// stays nil (no closure addition and no own values) so an untouched facet is omitted.
func unionSorted(own, extra []string) []string {
	if len(extra) == 0 {
		return own
	}
	merged := append(append([]string(nil), own...), extra...)
	slices.Sort(merged)
	return slices.Compact(merged)
}
