package search

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/dict/vocab"
)

// StringFacets maps an equality-facet query param to its index attribute. It is
// the single source of truth for the search index's string-facet vocabulary,
// shared by the HTTP search/facets handlers and the notification matcher.
// Enrichment facets live under the nested "enrichment" object, so they filter on
// a dot path; geography (regions/countries), work_mode and skills are resolved
// facets served top-level, so they filter on a bare attribute. Repeated params
// (?seniority=a&seniority=b) are ORed.
var StringFacets = map[string]string{
	"source": "source",
	// Written on the document at index time from sources.ProviderKind (see
	// JobDocument.SourceType); `source_type_exclude=aggregator` is the "originals only" ask.
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
	// requires_clearance is a true-or-absent boolean (see jobview), so facetEq
	// special-cases the "false" value rather than emitting an equality against a
	// value the index never carries.
	//
	// Being in this map also enrols it in the /jobs/facets distribution (see
	// handler.facetAttributes, which reads this map), which is why the Meilisearch
	// settings patch declaring the attribute filterable MUST reach the live index
	// before a binary carrying this line does — otherwise every facets request 500s.
	"requires_clearance": "requires_clearance",
	// via_recruiter is true-or-absent too, and enrolled in /jobs/facets the same way —
	// so the same settings-before-binary order applies.
	"via_recruiter": "via_recruiter",
	// equity is a closed vocabulary (vocab.EquityValues) whose "not stated" is the
	// attribute's absence, selected by the reserved EquityNotStated value. Enrolled in
	// /jobs/facets like the two above, so the same settings-before-binary order applies.
	EquityParam: EquityParam,
	// clearance_levels is the multi-value level facet beside it: a posting carries the
	// levels it states, and the attribute is absent exactly when requires_clearance
	// is (see ClearanceLevelUnspecified). The settings-before-binary rule above
	// applies to it as well.
	"clearance_levels": "clearance_levels",
	// ai_interview is written on the document at index time from the company's report
	// count (see JobDocument.AIInterview), so it filters on the bare attribute.
	"ai_interview": "ai_interview",
	// auto_apply_available is computed from the job's source at wire-shape build
	// time (see jobview.AutoApplyProviders) and flattens onto the document like
	// every other jobview.Job field, true-or-absent like ai_interview. Unlike
	// ai_interview it is also part of the served job object, not document-only.
	"auto_apply_available": "auto_apply_available",
	"ai_archetype":         "ai_archetype",
	// Derived at index time like the two above, so it filters on the bare attribute.
	// The vocabulary holds one value, which makes `role_type_exclude` the way to ask
	// for postings with no management marker — NOT a positive individual-contributor
	// filter, a distinction the OpenAPI description spells out for integrators.
	"role_type":        "role_type",
	"reality":          "reality.class",
	"collections":      "collections",
	"relocation":       "enrichment.relocation",
	"english_level":    "enrichment.english_level",
	"posting_language": "enrichment.posting_language",
	// perks are the benefits the description states (internal/dict/perks), a jobs
	// column served top-level. Enrolled in /jobs/facets like requires_clearance, with
	// the same settings-before-binary hazard.
	"perks": "perks",
}

// locationFacets are the geography params that describe one user-facing concept
// ("where"), so their included values OR into a single group rather than ANDing
// across facets — selecting the "Global" region and "Brazil" widens the results
// (Global OR Brazil) instead of intersecting them to zero. They share the SPA's
// one Location pane. Excludes stay per-value AND groups like every other facet,
// and a non-location facet (e.g. work_mode) still ANDs with the whole group, so
// "remote AND (Europe OR Brazil)" holds. Geographic AND is nonsensical ("in
// Europe AND in LATAM" is empty), so the `_mode=and` override does not apply here.
var locationFacets = map[string]bool{"regions": true, "countries": true, "cities": true, "metros": true}

// RegionUnspecified is the reserved value of the `regions` facet that selects
// jobs with no resolved geography (an empty regions array) rather than a real
// region code. The SPA's "Not specified" region chip serializes to it. It maps to
// Meilisearch's IS EMPTY (IS NOT EMPTY when excluded), so it ORs with real region
// values in the same facet group and supports exclude like any region — replacing
// the former materialized `remote_unspecified` boolean with a query-time predicate.
const RegionUnspecified = "none"

// EquityParam asks what a posting states about equity compensation.
const EquityParam = "equity"

// EquityNotStated is the reserved value of the equity facet that selects the postings
// stating neither vocab.EquityValues member. Nothing stores it: the dictionary cannot
// tell "no equity" from silence, so the document simply omits the attribute and the
// value maps to NOT EXISTS (EXISTS when excluded) — the regions RegionUnspecified
// pattern, for a scalar attribute instead of an array.
const EquityNotStated = "not_stated"

// ClearanceLevelUnspecified is the reserved value of the `clearance_levels` facet that
// selects postings with no detected clearance requirement. Like RegionUnspecified it
// ORs with real levels in the same group and supports exclude.
//
// It is answered from requires_clearance, not from the levels attribute. The two are
// written from one reading of the description (jobderive), so "no level" and "not
// marked" are the same postings — but requires_clearance already sits on every
// document in the live index, while clearance_levels reaches an existing posting only
// with the next backfill and reindex. Asking the levels attribute (NOT EXISTS) would
// answer "every posting" until then; asking the boolean is right from the first
// request. It also makes `clearance_levels=none` exactly the old
// `requires_clearance=false`, which is how the SPA reads that legacy link.
const ClearanceLevelUnspecified = "none"

// regionImplies names, for a region filter value, the other regions choosing it
// also selects. "eu" is Europe in the geographic sense, and the UK is in Europe, but
// a British posting carries only its own "uk" region — so without this, choosing
// Europe hid every London job (HN feedback, 2026-10-26). The UK stays its own region,
// so it can still be chosen alone. Applied at query time, never stored: the document's
// regions say where the posting IS, and the job card renders them verbatim.
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
	attr, val = retiredCategory(param, attr, val)
	if trueOrAbsentParams[param] {
		return trueOrAbsentFragment(attr, val)
	}
	return Eq(attr, val)
}

// RequiresClearanceParam is the facet whose stored value is true-or-absent: a
// posting that states a government clearance requirement carries it, and every other
// posting — the vast majority — carries no such attribute at all.
const RequiresClearanceParam = "requires_clearance"

// ViaRecruiterParam asks whether the posting states it is published by an
// intermediary on a client's behalf. True-or-absent: "hide recruiter posts" is the
// useful question, and it must keep every posting the dictionary did not mark.
const ViaRecruiterParam = "via_recruiter"

// AIInterviewParam asks whether the posting's company screens with an AI interviewer.
// Same true-or-absent shape: a job carries the attribute only when its company has at
// least one un-retracted report, and the useful question is the negative — "do not show
// me these employers" — which is why it cannot be an equality on false.
const AIInterviewParam = "ai_interview"

// AutoApplyAvailableParam asks whether the posting's ATS provider is one
// cmd/auto-apply can currently attempt to fill and submit for. Same
// true-or-absent shape: a job carries the attribute only when its source is
// one of the four eligible providers (see jobview.AutoApplyProviders) —
// a best-effort eligibility signal, never a submission guarantee.
const AutoApplyAvailableParam = "auto_apply_available"

// trueOrAbsentParams are the facets whose stored value is written only when true, so
// the index holds the attribute for the marked minority and nothing at all for
// everyone else. They share one predicate builder because they share one hazard.
var trueOrAbsentParams = map[string]bool{
	RequiresClearanceParam:  true,
	ViaRecruiterParam:       true,
	AIInterviewParam:        true,
	AutoApplyAvailableParam: true,
}

// trueOrAbsentFragment builds the predicate for those facets. "false" cannot be an
// equality: nothing in the index is ever written false, so `attr = false` matches
// nothing and would silently empty a result set the caller expected to be nearly the
// whole catalogue. Negating the positive is what actually answers "everything not
// marked", including the documents that omit the attribute.
func trueOrAbsentFragment(attr, val string) string {
	// Case-insensitive, like the validator that admitted the value (query_values.go):
	// "False" read as true would return exactly the postings the caller hid.
	if strings.EqualFold(strings.TrimSpace(val), "false") {
		return "NOT " + EqBool(attr, true)
	}
	return EqBool(attr, true)
}

// facetNeq builds a facet's exclude fragment: an inequality, except the regions
// unspecified sentinel, which becomes IS NOT EMPTY, the clearance-level one, which
// becomes "marked as requiring a clearance", and a region that implies others,
// which excludes all of them.
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

// retiredCategory answers `category=management` — a value vocab.CategoryValues no
// longer holds (vocab.RetiredCategoryManagement) — with the facet it was a proxy for:
// role_type=people_manager. Saved searches, alert subscriptions and shared links written
// before the retirement carry it (measured on prod at retirement: 85 saved searches, 56
// of them with an active subscription), usually ORed with other categories
// ("category=sales&category=management"), so the translation happens per VALUE, inside
// the category group: the old link keeps meaning "sales OR people managers" rather than
// being narrowed to "sales AND people managers", and `category_exclude=management`
// excludes the people managers.
//
// Nothing is dropped, so nothing widens and there is nothing for meta.ignored_params to
// report: the filter still narrows on the axis the value named. An unknown category value
// was never reported either — it narrows to nothing rather than widening.
//
// Every other (attr, val) pair is returned unchanged.
func retiredCategory(param, attr, val string) (string, string) {
	if param == "category" && val == vocab.RetiredCategoryManagement {
		return StringFacets["role_type"], vocab.RoleTypeValues[0]
	}
	return attr, val
}

// FilterFromValues turns the facet params of a parsed search query into a
// Meilisearch filter. Within a facet, included values are ORed by default (or
// ANDed when `<param>_mode=and`); excluded values (`<param>_exclude=...`) become
// NOT fragments. Facets are ANDed, except the location facets (regions/countries/
// cities/metros), whose includes OR into one shared group (see locationFacets). Returns
// nil when no facet is set.
//
// It is pure (no *fiber.Ctx), so the HTTP handler and the notification matcher
// build identical filters from the same canonical query string — the handler
// parses the request query, the matcher parses a saved search's stored query.
func FilterFromValues(v url.Values) any {
	return filterFromValues(v, time.Now())
}

// Narrows reports whether a parsed query selects less than the whole catalogue:
// free text, or at least one filter fragment FilterFromValues honours. It reads
// exactly the two things a search is built from (SearchParams.Query and
// SearchParams.Filter), so a query it calls narrowing is one that will actually
// come back narrowed.
//
// It exists because the raw query STRING answers a different question. A saved
// search of `remote=remote_unspecified` (a retired facet) or `country=it` (a typo
// for `countries`) is a non-empty string that no filter reads, so anything gating
// on emptiness lets it through as a real filter — and a subscription to one asks
// to be notified about every posting in the catalogue.
func Narrows(v url.Values) bool {
	return strings.TrimSpace(v.Get("q")) != "" || FilterFromValues(v) != nil
}

// maxWithinDays is the largest day count the two date bounds honour — a century, which
// is further back than any posting and than the catalogue itself.
//
// It exists for the arithmetic, not for the semantics. time.Duration is int64
// nanoseconds, so `time.Duration(n) * 24 * time.Hour` WRAPS above roughly 106,751 days:
// the cutoff landed in the future and the query matched nothing, which inverted the
// parameter — the most permissive bound anyone could type produced the emptiest result.
const maxWithinDays = 36500

// withinDays reports whether n is a day count the date bounds can honour. A value
// outside the range imposes no restriction, the same way a zero, a negative or a
// non-numeric one does: past a century the bound already means "any age", so dropping
// it says exactly what honouring it would have.
func withinDays(n int) bool { return n > 0 && n <= maxWithinDays }

// facetValue maps a caller-supplied facet value to the value the index carries. Two
// facets are resolved through the dictionary that wrote them. `cities`: the index holds
// the canonical name ("Munich"), so a local spelling ("München") is looked up in the
// curated location dictionary. `skills`: the index holds skilltag's canonical slug, so an
// alias ("golang") reaches the postings tagged "go" — until 2026-10-27 `skills=golang`
// answered zero beside 8,522 for `skills=go`. An unknown value stays verbatim, since an
// open facet may legitimately carry a value the dictionary does not know (and an unknown
// skill is named by IgnoredValues rather than dropped here).
func facetValue(param, val string) string {
	switch param {
	case "clearance_levels":
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

// filterFromValues is FilterFromValues with the reference time injected, so the
// relative `posted_within_days` cutoff is deterministic under test. The exported
// wrapper supplies time.Now(); only this inner form is unit-tested for the date
// branch.
func filterFromValues(v url.Values, now time.Time) any {
	var groups [][]string
	// Included geography fragments across regions/countries/cities collect here and
	// are appended as one OR group (see locationFacets).
	var locationGroup []string

	for param, attr := range StringFacets {
		included := splitFacetValues(v[param])
		for i, val := range included {
			included[i] = facetValue(param, val)
		}
		switch {
		case locationFacets[param]:
			for _, val := range included {
				locationGroup = append(locationGroup, facetEq(param, attr, val))
			}
		case len(included) > 0 && v.Get(param+"_mode") == "and":
			// Each value its own AND group: a job must match all of them.
			for _, val := range included {
				groups = append(groups, []string{facetEq(param, attr, val)})
			}
		case len(included) > 0:
			group := make([]string, len(included))
			for i, val := range included {
				group[i] = facetEq(param, attr, val)
			}
			// Most postings carry no level, so "intern" alone drops nearly everything an
			// intern could apply to. The modifier ORs those back into this group only.
			if param == "seniority" && v.Get(SeniorityIncludeUnspecifiedParam) == "true" {
				group = append(group, NotExists(attr))
			}
			groups = append(groups, group)
		}
		// Excluded values: each is its own AND group so all are filtered out.
		for _, val := range splitFacetValues(v[param+"_exclude"]) {
			groups = append(groups, []string{facetNeq(param, attr, facetValue(param, val))})
		}
	}
	if len(locationGroup) > 0 {
		groups = append(groups, locationGroup)
	}

	if raw := v.Get("visa_sponsorship"); raw != "" {
		groups = append(groups, []string{EqBool("enrichment.visa_sponsorship", raw == "true")})
	}

	if n, ok := atoiOK(v.Get("salary_min")); ok {
		groups = append(groups, []string{Gte("enrichment.salary_min", n)})
	}
	if n, ok := atoiOK(v.Get("salary_max")); ok {
		groups = append(groups, []string{Lte("enrichment.salary_max", n)})
	}
	// Both experience bounds read the SAME attribute: `enrichment.experience_years_min`
	// is what the posting asks for, so these are a floor and a ceiling on that one ask,
	// not a min/max pair over two fields. Meili compares only documents carrying the
	// attribute, so either bound drops the postings that state no requirement — the
	// honest reading of "asks for at most N years", and most of the catalogue.
	// `experience_include_unspecified=true` ORs those postings back into each bound's
	// own group; on its own it bounds nothing, so it adds no group of its own.
	//
	// `experience_only_seniority_unspecified=true` ORs every LABELLED posting into the
	// bound's group, which leaves the bound reading only the postings with no level.
	// With the seniority group it composes to `level IN (selected) OR (no level AND
	// within the bound)`: an internship stating no years stays in, an unlabelled posting
	// asking for five does not (freehire#3655).
	includeUnstated := v.Get(ExperienceIncludeUnspecifiedParam) == "true"
	onlyUnlevelled := v.Get(ExperienceOnlySeniorityUnspecifiedParam) == "true"
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
	// n >= 0 because a negative ceiling can only match nothing — the attribute is
	// never below zero — so it is a typo, and honouring it would render an empty
	// page that looks like a legitimately narrow search rather than a bad param.
	if n, ok := atoiOK(v.Get("experience_years_max")); ok && n >= 0 {
		groups = append(groups, experienceGroup(Lte("enrichment.experience_years_min", n)))
	}

	// Application cost: apply_questions_max=N keeps the postings whose captured form
	// requires at most N questions beyond the standard fields. A posting with no captured
	// form carries no apply_questions at all, and Meilisearch compares only documents that
	// carry the attribute — so "we do not know" is excluded rather than read as short,
	// which is the point. A negative bound can only match nothing (the count is never
	// below zero), so it is dropped and reported (ApplyQuestionsMaxParam).
	if n, ok := applyQuestionsMax(v.Get(ApplyQuestionsMaxParam)); ok {
		groups = append(groups, []string{Lte("apply_questions", n)})
	}

	// Freshness: posted_within_days=N restricts to jobs posted in the last N days,
	// i.e. whose effective posting date (posted_ts, unix seconds) is at or after
	// now - N*86400. A non-positive or non-numeric value imposes no restriction.
	if n, ok := atoiOK(v.Get("posted_within_days")); ok && withinDays(n) {
		cutoff := now.Add(-time.Duration(n) * 24 * time.Hour).Unix()
		groups = append(groups, []string{Gte("posted_ts", int(cutoff))})
	}

	// How long the posting has been open: open_within_days=N restricts to jobs whose
	// created_ts (the instant ingest first wrote the row) is at or after now - N*86400.
	// Same shape and same lenient parse as the bound above, and independent of it — a
	// request may carry both, and they AND.
	//
	// The two are NOT interchangeable, which is why both exist. posted_ts follows the
	// date the SOURCE states, and some boards restate it every crawl, so a posting open
	// for months passes a three-day posted bound. created_ts is the system's own
	// observation and cannot be rewritten from outside.
	if n, ok := atoiOK(v.Get("open_within_days")); ok && withinDays(n) {
		cutoff := now.Add(-time.Duration(n) * 24 * time.Hour).Unix()
		groups = append(groups, []string{Gte("created_ts", int(cutoff))})
	}

	return Filter(groups...)
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

// ApplyQuestionsMaxParam bounds how many required questions a posting's application
// form may ask beyond the standard fields. The search box's "short form" toggle sends 3:
// measured 2026-10-31 over a sample of open postings with a captured form, about half ask
// three or fewer, and the share with a required written answer steps from ~20% at one to
// three questions up to ~31% at four.
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
