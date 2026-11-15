package search

import (
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/dict/vocab"
)

// StringFacets maps an equality-facet query param to its index attribute: the single
// source of truth for the string-facet vocabulary, shared by the HTTP handlers and the
// notification matcher. Enrichment facets filter on a dot path under "enrichment";
// resolved facets (geography, work_mode, skills) are top-level. Repeated params are ORed.
var StringFacets = map[string]string{
	"source": "source",
	// Written at index time from sources.ProviderKind (see JobDocument.SourceType);
	// `source_type_exclude=aggregator` is the "originals only" ask.
	"source_type":     "source_type",
	"company_slug":    "company_slug",
	"regions":         "regions",
	"work_mode":       "work_mode",
	"employment_type": "enrichment.employment_type",
	"education_level": "enrichment.education_level",
	"seniority":       "enrichment.seniority",
	"category":        "enrichment.category",
	"domains":         "enrichment.domains",
	"countries":       "countries",
	"cities":          "cities",
	"metros":          "metros",
	"company_type":    "enrichment.company_type",
	"company_size":    "enrichment.company_size",
	"salary_currency": "enrichment.salary_currency",
	"salary_period":   "enrichment.salary_period",
	"skills":          "skills",
	"is_tech":         "is_tech",
	// requires_clearance is true-or-absent (see jobview), so facetEq special-cases "false".
	// Being in this map enrols an attribute in /jobs/facets (handler.facetAttributes), so its
	// filterable declaration must reach the live index before a binary carrying the entry, or
	// every facets request 500s. That holds for the entries below too.
	"requires_clearance": "requires_clearance",
	// via_recruiter is true-or-absent too.
	"via_recruiter": "via_recruiter",
	// equity is a closed vocabulary (vocab.EquityValues); "not stated" is the attribute's
	// absence, selected by the reserved EquityNotStated value.
	EquityParam: EquityParam,
	// clearance_levels is multi-value and absent exactly when requires_clearance is (see
	// ClearanceLevelUnspecified).
	"clearance_levels": "clearance_levels",
	// ai_interview is written at index time from the company's report count (see
	// JobDocument.AIInterview).
	"ai_interview": "ai_interview",
	// auto_apply_available is computed from the source (see jobview.AutoApplyProviders),
	// true-or-absent like ai_interview, and also part of the served job object.
	"auto_apply_available": "auto_apply_available",
	"ai_archetype":         "ai_archetype",
	// role_type is derived at index time. Its vocabulary holds one value, so
	// `role_type_exclude` is how to ask for postings with no management marker, NOT a positive
	// individual-contributor filter (the OpenAPI description spells this out).
	"role_type":        "role_type",
	"reality":          "reality.class",
	"collections":      "collections",
	"relocation":       "enrichment.relocation",
	"english_level":    "enrichment.english_level",
	"posting_language": "enrichment.posting_language",
	// perks are the benefits the description states (internal/dict/perks).
	"perks": "perks",
	// engagement is the legal terms the posting states (internal/dict/engagement: IR35,
	// W-2, B2B, CLT...).
	"engagement": "engagement",
}

// locationFacets are the geography params for one concept ("where"): included values OR
// into a single group, so "Global" plus "Brazil" widens rather than intersects. Excludes
// stay per-value AND groups, and a non-location facet still ANDs with the group ("remote
// AND (Europe OR Brazil)"). Geographic AND is empty by nature, so `_mode=and` does not
// apply.
var locationFacets = map[string]bool{"regions": true, "countries": true, "cities": true, "metros": true}

// RegionUnspecified is the reserved `regions` value selecting jobs with no resolved
// geography. It maps to IS EMPTY (IS NOT EMPTY when excluded), so it ORs with real regions
// and supports exclude.
const RegionUnspecified = "none"

// EquityParam asks what a posting states about equity compensation.
const EquityParam = "equity"

// EquityNotStated is the reserved equity value selecting postings that state neither
// vocab.EquityValues member. Nothing stores it: the document omits the attribute, so it
// maps to NOT EXISTS (EXISTS when excluded).
const EquityNotStated = "not_stated"

// ClearanceLevelUnspecified is the reserved `clearance_levels` value selecting postings
// with no detected clearance requirement. It ORs with real levels and supports exclude.
//
// It is answered from requires_clearance, not the levels attribute: requires_clearance is
// on every indexed document, while clearance_levels reaches existing postings only after a
// backfill and reindex, so NOT EXISTS on it would match everything until then. It equals
// the legacy `requires_clearance=false`.
const ClearanceLevelUnspecified = "none"

// regionImplies names the other regions a region filter value also selects. "eu"
// includes "uk", but a British posting carries only "uk", so Europe alone would hide
// London jobs. The UK stays selectable alone. Applied at query time, never stored.
var regionImplies = map[string][]string{"eu": {"uk"}}

// impliedRegions returns every region a regions filter value selects, itself first,
// or nil when it selects only itself.
func impliedRegions(val string) []string {
	extra := regionImplies[val]
	if len(extra) == 0 {
		return nil
	}
	return append([]string{val}, extra...)
}

// facetEq builds a facet's include fragment: an equality, except the regions
// unspecified sentinel, which becomes IS EMPTY, the clearance-level one, which becomes
// "not marked as requiring a clearance", a region that implies others (see
// regionImplies), which becomes an IN, and the retired management category, which
// becomes the role_type equality (see retiredCategory).
func facetEq(param, attr, val string) string {
	if param == "regions" && val == RegionUnspecified {
		return IsEmpty(attr)
	}
	if param == EquityParam && strings.EqualFold(strings.TrimSpace(val), EquityNotStated) {
		return NotExists(attr)
	}
	if param == "clearance_levels" && strings.EqualFold(strings.TrimSpace(val), ClearanceLevelUnspecified) {
		return trueOrAbsentFragment(RequiresClearanceParam, "false")
	}
	if set := impliedRegions(val); param == "regions" && set != nil {
		return InStrings(attr, set)
	}
	if members := vocab.SourceFamilies[strings.ToLower(strings.TrimSpace(val))]; param == "source" && members != nil {
		return InStrings(attr, members)
	}
	attr, val = retiredCategory(param, attr, val)
	if trueOrAbsentParams[param] {
		return trueOrAbsentFragment(attr, val)
	}
	return Eq(attr, val)
}

// RequiresClearanceParam is true-or-absent: only postings stating a clearance
// requirement carry it.
const RequiresClearanceParam = "requires_clearance"

// ViaRecruiterParam asks whether the posting is published by an intermediary.
// True-or-absent: "hide recruiter posts" must keep every posting the dictionary did not mark.
const ViaRecruiterParam = "via_recruiter"

// AIInterviewParam asks whether the posting's company screens with an AI interviewer.
// True-or-absent: set only when the company has an un-retracted report, so it cannot be an
// equality on false.
const AIInterviewParam = "ai_interview"

// AutoApplyAvailableParam asks whether the posting's ATS provider is one cmd/auto-apply
// can attempt (see jobview.AutoApplyProviders). True-or-absent; an eligibility signal, not
// a submission guarantee.
const AutoApplyAvailableParam = "auto_apply_available"

// trueOrAbsentParams are the facets stored only when true; they share one predicate
// builder.
var trueOrAbsentParams = map[string]bool{
	RequiresClearanceParam:  true,
	ViaRecruiterParam:       true,
	AIInterviewParam:        true,
	AutoApplyAvailableParam: true,
}

// trueOrAbsentFragment builds their predicate. "false" cannot be an equality (nothing is
// stored false, so it matches nothing); negating the positive also covers documents that
// omit the attribute.
func trueOrAbsentFragment(attr, val string) string {
	// Case-insensitive like the validator (query_values.go): "False" read as true would
	// return the postings the caller hid.
	if strings.EqualFold(strings.TrimSpace(val), "false") {
		return "NOT " + EqBool(attr, true)
	}
	return EqBool(attr, true)
}

// facetNeq builds a facet's exclude fragment: an inequality, except the regions sentinel
// (IS NOT EMPTY), the clearance-level one ("marked as requiring a clearance"), a region
// that implies others (excludes all of them) and a source family (vocab.SourceFamilies;
// excludes every member).
func facetNeq(param, attr, val string) string {
	if param == "regions" && val == RegionUnspecified {
		return IsNotEmpty(attr)
	}
	if param == EquityParam && strings.EqualFold(strings.TrimSpace(val), EquityNotStated) {
		return Exists(attr)
	}
	if param == "clearance_levels" && strings.EqualFold(strings.TrimSpace(val), ClearanceLevelUnspecified) {
		return trueOrAbsentFragment(RequiresClearanceParam, "true")
	}
	if set := impliedRegions(val); param == "regions" && set != nil {
		return NotInStrings(attr, set)
	}
	if members := vocab.SourceFamilies[strings.ToLower(strings.TrimSpace(val))]; param == "source" && members != nil {
		return NotInStrings(attr, members)
	}
	// A true-or-absent facet is never stored false, so `attr != "false"` matches every
	// document: excluding a value means asking for its opposite.
	if trueOrAbsentParams[param] {
		if strings.EqualFold(strings.TrimSpace(val), "false") {
			return trueOrAbsentFragment(attr, "true")
		}
		return trueOrAbsentFragment(attr, "false")
	}
	attr, val = retiredCategory(param, attr, val)
	return Neq(attr, val)
}

// retiredCategory answers `category=management`, retired from vocab.CategoryValues
// (vocab.RetiredCategoryManagement), with role_type=people_manager. Old saved searches,
// subscriptions and links carry it, often ORed with other categories, so it is translated
// per VALUE inside the category group: the link keeps meaning "sales OR people managers",
// and `category_exclude=management` excludes people managers. Nothing is dropped, so
// meta.ignored_params has nothing to report. Other pairs are returned unchanged.
func retiredCategory(param, attr, val string) (string, string) {
	if param == "category" && val == vocab.RetiredCategoryManagement {
		return StringFacets["role_type"], vocab.RoleTypeValues[0]
	}
	return attr, val
}

// FilterFromValues turns the facet params of a parsed search query into a Meilisearch
// filter. Within a facet, included values are ORed by default (ANDed when
// `<param>_mode=and`); excluded values (`<param>_exclude=...`) become NOT fragments. Facets
// are ANDed, except the location facets, whose includes OR into one group (see
// locationFacets). Returns nil when no facet is set.
//
// Pure, so the handler and the notification matcher build identical filters from the same
// canonical query string.
func FilterFromValues(v url.Values) any {
	return filterFromValues(v, time.Now())
}

// Narrows reports whether a parsed query selects less than the whole catalogue: free
// text or at least one filter fragment FilterFromValues honours. The watchlist is the one
// narrowing it does not express (see WatchlistParam); whoever runs the search resolves it.
//
// The raw query STRING is not enough: `remote=remote_unspecified` (retired) or `country=it`
// (typo for `countries`) is non-empty yet no filter reads it, and a subscription to one
// would notify about every posting.
func Narrows(v url.Values) bool {
	return strings.TrimSpace(v.Get("q")) != "" || FilterFromValues(v) != nil || WantsWatchlist(v)
}

// maxWithinDays is the largest day count the date bounds honour (a century). It guards
// arithmetic: `time.Duration(n) * 24 * time.Hour` wraps above ~106,751 days, putting the
// cutoff in the future and matching nothing.
const maxWithinDays = 36500

// withinDays reports whether n is a day count the bounds can honour. Outside the range
// it imposes no restriction, like zero, a negative or a non-numeric value (past a century
// the bound already means "any age").
func withinDays(n int) bool { return n > 0 && n <= maxWithinDays }

// facetValue maps a caller-supplied value to the one the index carries. `cities`: the
// index holds the canonical name ("Munich"), so local spellings ("München") resolve through
// the location dictionary. `skills`: the index holds skilltag's canonical slug, so an alias
// ("golang") reaches postings tagged "go". Unknown values stay verbatim (an open facet may
// carry them; unknown skills are named by IgnoredValues).
func facetValue(param, val string) string {
	switch param {
	case "clearance_levels", "regions", "category":
		return strings.ToLower(strings.TrimSpace(val))
	case "cities":
		if name, ok := location.CanonicalCity(val); ok {
			return name
		}
	case "skills":
		if canonical, ok := skilltag.Resolve(val); ok {
			return canonical
		}
	}
	return val
}

// filterFromValues is FilterFromValues with the reference time injected, for a
// deterministic `posted_within_days` cutoff under test.
func filterFromValues(v url.Values, now time.Time) any {
	var groups [][]string
	// Included geography fragments collect here and are appended as one OR group (see
	// locationFacets).
	var locationGroup []string
	// Under remote-anywhere, work_mode includes are held back so their group can be built
	// as a disjunction over the location group.
	remoteAnywhere := remoteAnywhereActive(v)
	for param, attr := range StringFacets {
		included := splitFacetValues(v[param])
		for i, val := range included {
			included[i] = facetValue(param, val)
		}
		includeUnspecified := param == "seniority" && isTrue(v, SeniorityIncludeUnspecifiedParam)
		switch {
		case param == "work_mode" && remoteAnywhere:
		case locationFacets[param]:
			for _, val := range included {
				locationGroup = append(locationGroup, facetEq(param, attr, val))
			}
		case len(included) > 0 && strings.EqualFold(strings.TrimSpace(v.Get(param+"_mode")), "and"):
			// Each value its own AND group: a job must match all of them.
			for _, val := range included {
				group := []string{facetEq(param, attr, val)}
				if includeUnspecified {
					group = append(group, NotExists(attr))
				}
				groups = append(groups, group)
			}
		case len(included) > 0:
			group := make([]string, len(included))
			for i, val := range included {
				group[i] = facetEq(param, attr, val)
			}
			// Most postings carry no level, so "intern" alone drops nearly everything; the modifier
			// ORs those back into this group only.
			if includeUnspecified {
				group = append(group, NotExists(attr))
			}
			groups = append(groups, group)
		}
		// Excluded values: each is its own AND group so all are filtered out.
		for _, val := range splitFacetValues(v[param+"_exclude"]) {
			groups = append(groups, []string{facetNeq(param, attr, facetValue(param, val))})
		}
	}
	// Under remote-anywhere the location group is consumed into the disjunction's physical
	// arm; appending it standalone would also AND it over the remote arm.
	if remoteAnywhere {
		groups = append(groups, remoteAnywhereGroup(workModeValues(v), remoteReachGroup(v), locationGroup))
	} else if len(locationGroup) > 0 {
		groups = append(groups, locationGroup)
	}

	if raw := strings.ToLower(strings.TrimSpace(v.Get("visa_sponsorship"))); raw == "true" || raw == "false" {
		groups = append(groups, []string{EqBool("enrichment.visa_sponsorship", raw == "true")})
	}

	if n, ok := atoiOK(v.Get("salary_min")); ok {
		groups = append(groups, []string{Gte("enrichment.salary_min", n)})
	}
	if n, ok := atoiOK(v.Get("salary_max")); ok {
		groups = append(groups, []string{Lte("enrichment.salary_max", n)})
	}
	// Both experience bounds read the SAME attribute, `enrichment.experience_years_min` (what
	// the posting asks for): a floor and a ceiling on one ask. Meili compares only documents
	// carrying it, so either bound drops postings stating no requirement.
	// `experience_include_unspecified=true` ORs those back into each bound's own group; alone
	// it bounds nothing.
	// `experience_only_seniority_unspecified=true` ORs every LABELLED posting into the bound's
	// group, so the bound reads only unlevelled postings. With the seniority group it composes
	// to `level IN (selected) OR (no level AND within the bound)`.
	includeUnstated := isTrue(v, ExperienceIncludeUnspecifiedParam)
	onlyUnlevelled := isTrue(v, ExperienceOnlySeniorityUnspecifiedParam)
	experienceGroup := func(bound string) []string {
		group := []string{bound}
		if includeUnstated {
			group = append(group, NotExists("enrichment.experience_years_min"))
		}
		if onlyUnlevelled {
			group = append(group, Exists(StringFacets["seniority"]))
		}
		return group
	}
	if n, ok := atoiOK(v.Get("experience_years_min")); ok {
		groups = append(groups, experienceGroup(Gte("enrichment.experience_years_min", n)))
	}
	// n >= 0: a negative ceiling matches nothing, so it is a typo, and honouring it would
	// render an empty page that looks like a narrow search.
	if n, ok := atoiOK(v.Get("experience_years_max")); ok && n >= 0 {
		groups = append(groups, experienceGroup(Lte("enrichment.experience_years_min", n)))
	}

	// apply_questions_max=N keeps postings whose captured form requires at most N questions
	// beyond the standard fields. A posting with no captured form has no apply_questions and
	// Meili skips it, so unknown is excluded, not read as short. A negative bound is dropped
	// and reported (ApplyQuestionsMaxParam).
	if n, ok := applyQuestionsMax(v.Get(ApplyQuestionsMaxParam)); ok {
		groups = append(groups, []string{Lte("apply_questions", n)})
	}

	// posted_within_days=N keeps jobs whose posted_ts is within the last N days; an
	// out-of-range or non-numeric value imposes no restriction.
	if n, ok := atoiOK(v.Get("posted_within_days")); ok && withinDays(n) {
		cutoff := now.Add(-time.Duration(n) * 24 * time.Hour).Unix()
		groups = append(groups, []string{Gte("posted_ts", int(cutoff))})
	}

	// open_within_days=N keeps jobs whose created_ts (when ingest first wrote the row) is
	// within the last N days. Same lenient parse as above; both may be present and AND.
	// Not interchangeable: posted_ts follows the date the SOURCE states, and some boards
	// restate it every crawl, so a months-old posting passes a posted bound. created_ts is the
	// system's own observation.
	if n, ok := atoiOK(v.Get("open_within_days")); ok && withinDays(n) {
		cutoff := now.Add(-time.Duration(n) * 24 * time.Hour).Unix()
		groups = append(groups, []string{Gte("created_ts", int(cutoff))})
	}

	return Filter(groups...)
}

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

// splitFacetValues splits each raw query value on comma and flattens the
// result, dropping empty fragments (a stray comma, or a bare `?skills=`) — so
// a repeated key (`skills=go&skills=rust`) and a comma-joined value
// (`skills=go,rust`) resolve to the same set of facet values.
func splitFacetValues(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		for _, part := range strings.Split(s, ",") {
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// ApplyQuestionsMaxParam bounds how many required questions a posting's application form
// may ask beyond the standard fields. The search box's "short form" toggle sends 3.
const ApplyQuestionsMaxParam = "apply_questions_max"

// applyQuestionsMax reads the apply_questions_max bound: a non-negative integer, or no
// bound. IgnoredValues names every value this declines, so a caller whose typo widened
// the answer is told.
func applyQuestionsMax(s string) (int, bool) {
	n, ok := atoiOK(strings.TrimSpace(s))
	return n, ok && n >= 0
}

// atoiOK reports whether a query value is a valid integer, so a missing or
// non-numeric value emits no bogus numeric fragment.
func atoiOK(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}
