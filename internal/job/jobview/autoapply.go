package jobview

// AutoApplyProviders is a manually-synced mirror of internal/api/atsapply's
// fillProviders (greenhouse, lever) union browserUseProviders (ashby, workable). This
// package cannot import atsapply (job is layer 5, api layer 8).
//
// It lives here so jobview.FromDomain, the one projection every list/detail/search
// response goes through, serves AutoApplyAvailable directly.
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
