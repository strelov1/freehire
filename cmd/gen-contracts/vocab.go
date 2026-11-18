package main

import (
	"sort"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/mailclassify"
	"github.com/strelov1/freehire/internal/application/userjob"
	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/dict/classify"
	"github.com/strelov1/freehire/internal/dict/engagement"
	"github.com/strelov1/freehire/internal/dict/industrytag"
	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/perks"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/collections"
	"github.com/strelov1/freehire/internal/job/ghost"
)

// emitVocab renders one closed vocabulary as a frozen value array plus a string-union
// type, e.g. emitVocab("Seniority", "SENIORITY_VALUES", […]) →
//
//	export const SENIORITY_VALUES = ['intern', …] as const;
//	export type Seniority = (typeof SENIORITY_VALUES)[number];
func emitVocab(typeName, constName string, values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + v + "'"
	}
	var b strings.Builder
	b.WriteString("export const " + constName + " = [" + strings.Join(quoted, ", ") + "] as const;\n")
	b.WriteString("export type " + typeName + " = (typeof " + constName + ")[number];\n")
	return b.String()
}

// emitMap renders a string→string map as a frozen object literal plus a type alias,
// e.g. emitMap("CityCountry", "CITY_COUNTRY_MAP", {…}) →
//
//	export const CITY_COUNTRY_MAP = {
//	  'Amsterdam': 'nl',
//	  …
//	} as const;
//	export type CityCountry = typeof CITY_COUNTRY_MAP;
//
// Keys are emitted in sorted order so the committed output is deterministic despite
// Go's randomized map iteration.
func emitMap(typeName, constName string, m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	if len(keys) == 0 {
		b.WriteString("export const " + constName + " = {} as const;\n")
	} else {
		b.WriteString("export const " + constName + " = {\n")
		for _, k := range keys {
			b.WriteString("  " + quoteTS(k) + ": " + quoteTS(m[k]) + ",\n")
		}
		b.WriteString("} as const;\n")
	}
	b.WriteString("export type " + typeName + " = typeof " + constName + ";\n")
	return b.String()
}

// emitMapOfSlices renders a string→[]string map as a frozen object of string
// arrays plus a type alias. Keys and each value slice are emitted in sorted order
// so the committed output is deterministic despite Go's randomized map iteration.
func emitMapOfSlices(typeName, constName string, m map[string][]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	if len(keys) == 0 {
		b.WriteString("export const " + constName + " = {} as const;\n")
	} else {
		b.WriteString("export const " + constName + " = {\n")
		for _, k := range keys {
			vals := append([]string(nil), m[k]...)
			sort.Strings(vals)
			quoted := make([]string, len(vals))
			for i, v := range vals {
				quoted[i] = quoteTS(v)
			}
			b.WriteString("  " + quoteTS(k) + ": [" + strings.Join(quoted, ", ") + "],\n")
		}
		b.WriteString("} as const;\n")
	}
	b.WriteString("export type " + typeName + " = typeof " + constName + ";\n")
	return b.String()
}

// quoteTS renders a string as a single-quoted TS literal, escaping backslashes and
// single quotes so a value like "N'Djamena" can't break the generated file.
func quoteTS(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// emitStageGroups renders the stage→group membership as an ordered array literal:
//
//	export const STAGE_GROUPS = [
//	  { id: 'applied', label: 'Applied', stages: ['applied', 'screening', 'responded'] },
//	  …
//	] as const;
//	export type StageGroup = (typeof STAGE_GROUPS)[number];
//
// An array in the table's own order, not sorted: that order is the board's column order and
// the funnel's band order.
func emitStageGroups(groups []userjob.Group) string {
	var b strings.Builder
	b.WriteString("export const STAGE_GROUPS = [\n")
	for _, g := range groups {
		stages := make([]string, len(g.Stages))
		for i, s := range g.Stages {
			stages[i] = quoteTS(s)
		}
		b.WriteString("  { id: " + quoteTS(g.ID) +
			", label: " + quoteTS(g.Label) +
			", stages: [" + strings.Join(stages, ", ") + "] },\n")
	}
	b.WriteString("] as const;\n")
	b.WriteString("export type StageGroup = (typeof STAGE_GROUPS)[number];\n")
	return b.String()
}

// emitSignalStages renders what each mail signal implies for the application stage:
//
//	export const SIGNAL_STAGE = {
//	  'acknowledgement': { stage: 'applied', advances: true },
//	  'rejection': { stage: 'rejected', advances: false },
//	  …
//	} as const;
//
// Every signal is present, including those implying no stage (empty `stage`), so the reader
// needs no hand-kept exception list. Keys sorted, like emitMap.
func emitSignalStages(signals []string) string {
	sorted := append([]string(nil), signals...)
	sort.Strings(sorted)

	var b strings.Builder
	b.WriteString("export const SIGNAL_STAGE = {\n")
	for _, s := range sorted {
		stage, advances := mailclassify.StageFor(mailclassify.StatusSignal(s))
		b.WriteString("  " + quoteTS(s) + ": { stage: " + quoteTS(stage) +
			", advances: " + strconv.FormatBool(advances) + " },\n")
	}
	b.WriteString("} as const;\n")
	b.WriteString("export type SignalStage = typeof SIGNAL_STAGE;\n")
	return b.String()
}

// emitCollections renders the company-tag registry as a frozen array of objects
// plus the two types the frontend reads off it:
//
//	export const COLLECTIONS = [
//	  { slug: 'yc', title: 'Y Combinator', description: '…', kind: 'editorial' },
//	  …
//	] as const;
//	export type Collection = (typeof COLLECTIONS)[number];
//	export type CollectionKind = Collection['kind'];
//
// Emitted in registry order, which is the display order on the /collections hub and in the
// job-search facet. `kind` decides which filter group a tag renders in, so a slug missing from
// a hand-kept frontend copy would filter without a pill; hence generated.
func emitCollections(all []collections.Collection) string {
	var b strings.Builder
	b.WriteString("export const COLLECTIONS = [\n")
	for _, c := range all {
		b.WriteString("  { slug: " + quoteTS(c.Slug) +
			", title: " + quoteTS(c.Title) +
			", description: " + quoteTS(c.Description) +
			", kind: " + quoteTS(string(c.Kind)) + " },\n")
	}
	b.WriteString("] as const;\n")
	b.WriteString("export type Collection = (typeof COLLECTIONS)[number];\n")
	b.WriteString("export type CollectionKind = Collection['kind'];\n")
	return b.String()
}

// genVocab emits every closed vocabulary. Source = non-boardless providers plus the
// non-adapter sources set by other writers: "telegram" (the tg-extract worker) and the
// moderator/bulk-import origins ("workatastartup", "remoteok", "arc"). Stages from
// userjob; the rest from the enrich controlled vocabularies.
// A manual/bulk-import origin can later gain a registered adapter (e.g. remoteok), so it
// appears in both the hardcoded prefix and FilterableProviders; dedup keeps SOURCE_VALUES a
// set, since a duplicate throws each_key_duplicate in the SPA.
func genVocab() string {
	source := dedup(append([]string{"telegram", "workatastartup", "remoteok", "arc"}, sources.FilterableProviders()...))

	var b strings.Builder
	b.WriteString(emitVocab("Source", "SOURCE_VALUES", source))
	b.WriteString(emitVocab("Stage", "STAGE_VALUES", userjob.Stages))
	// The ledger's event kinds, generated so a kind added in Go cannot render as a blank row in
	// the SPA's label map.
	b.WriteString(emitVocab("ApplicationEventKind", "APPLICATION_EVENT_KINDS", appevent.Kinds))
	// The ledger's event sources: the activity grid counts a `stage_set` only when the candidate
	// made it, a rule about the source, and a hand-kept list would silently stop counting a new
	// source.
	b.WriteString(emitVocab("ApplicationEventSource", "APPLICATION_EVENT_SOURCES", appevent.Sources))
	// The stage's human label and group, built from Stages so a stage cannot reach the SPA
	// labelled but ungrouped.
	stageLabels := make(map[string]string, len(userjob.Stages))
	for _, s := range userjob.Stages {
		stageLabels[s] = userjob.Label(s)
	}
	b.WriteString(emitMap("StageLabels", "STAGE_LABELS", stageLabels))
	b.WriteString(emitStageGroups(userjob.Groups))
	b.WriteString(emitVocab("WorkMode", "WORK_MODE_VALUES", vocab.WorkModeValues))
	b.WriteString(emitVocab("Seniority", "SENIORITY_VALUES", vocab.SeniorityValues))
	b.WriteString(emitVocab("Category", "CATEGORY_VALUES", vocab.CategoryValues))
	// The technical half of the categories, generated rather than re-listed so a picker
	// that leads with technical roles cannot drift from the split is_tech reads.
	b.WriteString(emitVocab("TechCategory", "TECH_CATEGORY_VALUES", vocab.TechCategories))
	// The survey's challenge answers, generated so the tour's options are checked against
	// the list the server validates — a drifted value is an answer that is always a 400.
	b.WriteString(emitVocab("JobChallenge", "JOB_CHALLENGE_VALUES", vocab.JobChallengeValues))
	b.WriteString(emitVocab("EmploymentType", "EMPLOYMENT_TYPE_VALUES", vocab.EmploymentTypeValues))
	b.WriteString(emitVocab("Relocation", "RELOCATION_VALUES", vocab.RelocationValues))
	b.WriteString(emitVocab("EnglishLevel", "ENGLISH_LEVEL_VALUES", vocab.EnglishLevelValues))
	b.WriteString(emitVocab("CompanyType", "COMPANY_TYPE_VALUES", vocab.CompanyTypeValues))
	// The Talent Network catalogue's filter params: a param the panes offer but the API does not
	// read is reported as ignored and widens the results beyond the visitor's chips.
	b.WriteString(emitVocab("TalentFacetParam", "TALENT_FACET_PARAMS", talentnetwork.FacetParams()))
	// The company feedback category vocabulary (internal/engage/companyfeedback), generated
	// so the review form's category picker can't drift from the DB CHECK constraint.
	b.WriteString(emitVocab("CompanyFeedbackType", "COMPANY_FEEDBACK_TYPE_VALUES", vocab.CompanyFeedbackTypeValues))
	// The report-reason vocabulary for flagging a specific feedback entry, generated
	// so the report picker can't drift from the DB CHECK constraint.
	b.WriteString(emitVocab("CompanyFeedbackReportReason", "COMPANY_FEEDBACK_REPORT_REASON_VALUES", vocab.CompanyFeedbackReportReasonValues))
	b.WriteString(emitVocab("Domain", "DOMAIN_VALUES", vocab.DomainValues))
	// The curated company-industry vocabulary (internal/dict/industrytag) — the level
	// beneath domains. Generated in both halves, values and labels, so the filter's
	// options cannot drift from the dictionary the column is written through.
	b.WriteString(emitVocab("Industry", "INDUSTRY_VALUES", industrytag.Canonicals()))
	b.WriteString(emitMap("IndustryLabels", "INDUSTRY_LABELS", industrytag.Labels()))
	b.WriteString(emitMapOfSlices("IndustryAliases", "INDUSTRY_ALIASES", industrytag.SearchAliases()))
	// The six AI skill-signature archetype slugs (internal/ai/aiarchetype), generated
	// so the AI Specialization filter's valid values can't drift from the rule table.
	b.WriteString(emitVocab("AIArchetype", "AI_ARCHETYPE_VALUES", vocab.AIArchetypeValues))
	// The role-type vocabulary (internal/dict/roletype), generated for the same reason:
	// the filter's one pill must not drift from the dictionary. It holds a single
	// value, and the absence of one means "no management marker", never
	// "individual contributor" — nothing in the SPA may label it as the latter.
	b.WriteString(emitVocab("RoleType", "ROLE_TYPE_VALUES", vocab.RoleTypeValues))
	// The clearance levels (internal/dict/location), so the Security clearance chips
	// cannot offer a level the dictionary never writes.
	b.WriteString(emitVocab("ClearanceLevel", "CLEARANCE_LEVEL_VALUES", vocab.ClearanceLevelValues))
	// The equity vocabulary (internal/job/jobfacts.Equity), so the Equity chips cannot
	// offer a value the derivation never writes. The reserved "not stated" value is the
	// attribute's absence (search.EquityNotStated), not a member, so the SPA adds it.
	b.WriteString(emitVocab("Equity", "EQUITY_VALUES", vocab.EquityValues))
	// The source-type vocabulary (sources.ProviderKind's searchable kinds), so the Source
	// pane's pills cannot offer a kind no document carries.
	b.WriteString(emitVocab("SourceType", "SOURCE_TYPE_VALUES", vocab.SourceTypeValues))
	// The perk vocabulary (internal/dict/perks), so the Benefits & Perks chips cannot
	// offer a perk the dictionary never writes.
	b.WriteString(emitVocab("Perk", "PERK_VALUES", vocab.PerkValues))
	b.WriteString(emitMap("PerkLabels", "PERK_LABELS", perks.Labels()))
	// The engagement vocabulary (internal/dict/engagement), so the Engagement chips
	// cannot offer a value the dictionary never writes.
	b.WriteString(emitVocab("Engagement", "ENGAGEMENT_VALUES", vocab.EngagementValues))
	b.WriteString(emitMap("EngagementLabels", "ENGAGEMENT_LABELS", engagement.Labels()))
	// The market each term belongs to, so the chips group by it.
	b.WriteString(emitMap("EngagementMarkets", "ENGAGEMENT_MARKETS", engagement.Markets()))
	// The ghost criterion vocabulary: the job page draws one gauge segment per fired criterion,
	// and a criterion missing from the SPA's checklist would show a segment nothing can explain.
	// Generated, the omission is a type error.
	b.WriteString(emitVocab("GhostCriterion", "GHOST_CRITERION_VALUES", ghost.CriterionCodes[:]))
	// The classified-mail vocabulary: the inbox renders a label and badge colour per signal, so a
	// signal missing from the SPA's maps would be a blank chip. Generated, the omission is a type
	// error.
	b.WriteString(emitVocab("EmailStatusSignal", "EMAIL_STATUS_SIGNAL_VALUES", mailclassify.SignalValues))
	b.WriteString(emitSignalStages(mailclassify.SignalValues))
	// Country→region grouping for the hierarchical location filter, derived from the
	// location dictionary.
	b.WriteString(emitMap("CountryRegionMap", "COUNTRY_REGION_MAP", location.CountryToRegion()))
	// Skill slug→display-name catalog, generated from the skilltag dictionary so every surface
	// shows one spelling and a new skill cannot arrive unlabelled.
	b.WriteString(emitMap("SkillLabels", "SKILL_LABELS", skilltag.Labels()))
	// Facet slug→shorthand-aliases for the picker's search: the same curated terms
	// used to tag titles, so typing "swe" or "data engineer" finds the specialization
	// that tags them. Keyed by category and seniority slug — the two dictionaries this
	// is built from, and the two facets that read it.
	b.WriteString(emitMapOfSlices("FacetAliases", "FACET_ALIASES", facetAliases()))
	// The company-tag registry (slug, display copy, kind) — the source of truth for
	// the /collections hub and the job-search facet, generated so the frontend copy
	// cannot drift from the Go registry that decides membership.
	b.WriteString(emitCollections(collections.All))
	return b.String()
}

// roleAliases unions the curated shorthand→role aliases from the three
// dictionaries feeding the role facet — category and seniority aliases (classify)
// plus named-role aliases (roletag) — keyed by role slug and deduped, for the web
// picker's alias search. Every value is a lowercase alias already used to tag
// titles, so search and tagging stay in lockstep.
func facetAliases() map[string][]string {
	sets := map[string]map[string]struct{}{}
	addAll := func(m map[string][]string) {
		for slug, aliases := range m {
			if sets[slug] == nil {
				sets[slug] = map[string]struct{}{}
			}
			for _, a := range aliases {
				sets[slug][a] = struct{}{}
			}
		}
	}
	addAll(classify.CategoryAliases())
	addAll(classify.SeniorityAliases())

	out := make(map[string][]string, len(sets))
	for slug, set := range sets {
		for a := range set {
			out[slug] = append(out[slug], a)
		}
	}
	return out
}

// dedup returns the slice with duplicates removed, preserving first-occurrence order.
func dedup(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := values[:0]
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
