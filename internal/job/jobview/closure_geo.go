package jobview

import "slices"

// MergeClosureGeography widens a job's geography facets with the union
// across every OPEN row it represents — its duplicate closure — so a collapsed
// multi-city/multi-country/multi-metro role stays findable by every city and country the rows it hides
// are open in, not only its own. Each facet becomes the sorted, deduped union of the
// document's own values and the closure's; an empty slice leaves that facet unchanged.
//
// Membership is the closure, NOT a shared role_fingerprint. Two of the three dedup passes
// suppress rows whose fingerprints differ from their canon's by construction, so a
// fingerprint key reached none of them and their cities left the index with them — issue
// #2225.
//
// The search document embeds Job, so the index writers and the job page share this one
// union: a page showing fewer places than the search that found it is the mismatch
// freehire#3646 reported.
//
// Every writer of a job document must call this, not only the full reindex. The push is a
// field-level document update and these four facets are always present in the payload, so
// a writer that skips the union does not merely fail to widen the row — it replaces the
// widened values with the row's own. The reindex reads its closure geography from the
// whole-catalogue DuplicateClosureGeoAll; the per-wave writers read theirs from
// DuplicateClosureGeoFor.
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
