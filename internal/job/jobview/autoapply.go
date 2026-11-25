package jobview

// AutoApplyProviders is a manually-synced mirror of internal/api/atsapply's
// fillProviders (greenhouse, lever) union browserUseProviders (ashby, workable). This
// package cannot import atsapply (job is layer 5, api layer 8).
//
// It lives here so jobview.FromDomain, the one projection every list/detail/search
// response goes through, serves AutoApplyAvailable directly.
//
// It is also the base of what auto-apply may QUEUE (autoapply.Queueable adds recruitee): a
// provider advertised here must be startable, so the enqueue set derives from this one
// instead of listing it again.
//
// Exported so atsapply's tests can assert it matches the real sets. Drift fails
// TestAutoApplyProviders_ExactExpectedSet here or
// TestAutoApplyFacetProvidersMatchThisPackagesOwnMaps there; neither alone catches
// both directions.
var AutoApplyProviders = map[string]bool{
	"greenhouse": true,
	"lever":      true,
	"ashby":      true,
	"workable":   true,
}
