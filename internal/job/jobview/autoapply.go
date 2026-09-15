package jobview

// AutoApplyProviders is a manually-synced mirror of internal/api/atsapply's
// fillProviders (greenhouse, lever — chromedp) union browserUseProviders
// (ashby, workable — cloud-agent fallback). This package cannot import
// atsapply: job is layer 5 and api is layer 8, strictly above it, per this
// repo's layering rule.
//
// It lives here, not in internal/search (which computed it before), so that
// jobview.FromDomain — the single projection every list/detail/search
// response goes through — can serve AutoApplyAvailable directly on the
// public wire shape. search.FromJob now reuses the value already computed
// here (via jobview.FromRow) rather than re-deriving it, which is one fewer
// place for the two copies to drift apart.
//
// Exported so atsapply's own tests — which CAN import jobview, since api sits
// above job — can assert this actually matches fillProviders/
// browserUseProviders. A same-package edit that drifts from that real source
// of truth fails TestAutoApplyProviders_ExactExpectedSet here; an unmatched
// edit on atsapply's own side fails
// TestAutoApplyFacetProvidersMatchThisPackagesOwnMaps there. Neither test
// alone would catch both directions.
var AutoApplyProviders = map[string]bool{
	"greenhouse": true,
	"lever":      true,
	"ashby":      true,
	"workable":   true,
}
