package search

import (
	"net/url"
	"slices"
	"sort"
	"strings"
)

// The remote-anywhere modifier: how work_mode composes with the location facets. It is a
// second reading of the same query, so it lives apart from the general filter builder.

// remoteAnywhereActive reports whether the remote-anywhere modifier is actually
// in force for this query: set to true, `remote` and at least one physical mode
// both selected, at least one location include, and no `_mode=and` on work_mode.
// Without remote the physical arm would relax the caller's filter; without a
// location the arm would widen past the mode list; `and` is a different filter.
func remoteAnywhereActive(v url.Values) bool {
	modes := workModeValues(v)
	return isTrue(v, WorkModeRemoteAnywhereParam) &&
		!strings.EqualFold(strings.TrimSpace(v.Get("work_mode_mode")), "and") &&
		slices.Contains(modes, "remote") &&
		slices.ContainsFunc(modes, isPhysicalMode) &&
		hasLocationInclude(v)
}

// hasLocationInclude reports whether any location facet carries a value the
// vocabulary knows. An unknown value of a closed facet (`regions=not-a-region`)
// matches nothing on the plain reading; letting it activate the modifier would
// answer every remote posting through the unbounded remote arm instead.
func hasLocationInclude(v url.Values) bool {
	for param := range locationFacets {
		for _, val := range splitFacetValues(v[param]) {
			if !UnknownFacetValue(param, facetValue(param, val)) {
				return true
			}
		}
	}
	return false
}

// workModeValues reads the work_mode includes with the case and whitespace rules
// the value check accepts (`Remote` passes it), so the modifier's precondition
// does not fail on a spelling the rest of the filter honours.
func workModeValues(v url.Values) []string {
	modes := splitFacetValues(v["work_mode"])
	for i, m := range modes {
		modes[i] = strings.ToLower(strings.TrimSpace(m))
	}
	return modes
}

func isPhysicalMode(m string) bool { return m == "hybrid" || m == "onsite" }

// remoteReachGroup builds the remote arm's geography from the remote-reach params,
// each value read exactly as the location facet it names. Empty: remote from anywhere.
func remoteReachGroup(v url.Values) []string {
	var group []string
	for param, facet := range remoteReachFacets {
		for _, val := range splitFacetValues(v[param]) {
			group = append(group, facetEq(facet, StringFacets[facet], facetValue(facet, val)))
		}
	}
	return group
}

// remoteAnywhereGroup builds the disjunction that replaces the work-mode and
// location groups: remote (within the reach, when one is given), OR one of the
// selected physical modes within the location group. Neither arm may carry a bare
// physical mode — that would admit a hybrid posting from any country and undo the
// geography.
func remoteAnywhereGroup(modes, reach, locationGroup []string) []string {
	var physical []string
	for _, m := range modes {
		if isPhysicalMode(m) {
			physical = append(physical, m)
		}
	}
	sort.Strings(physical)
	// Sorted: the group is collected in map iteration order, and the disjunction
	// embeds it as one string — the canonical query string a saved search is keyed
	// on must not depend on that order. locationGroup is not read again.
	sort.Strings(locationGroup)
	physicalArm := "(" + InStrings("work_mode", physical) + " AND (" + strings.Join(locationGroup, " OR ") + "))"
	remoteArm := Eq("work_mode", "remote")
	if len(reach) > 0 {
		sort.Strings(reach)
		remoteArm = "(" + remoteArm + " AND (" + strings.Join(reach, " OR ") + "))"
	}
	return []string{remoteArm, physicalArm}
}
