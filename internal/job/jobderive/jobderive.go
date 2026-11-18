// Package jobderive computes the deterministic, dictionary-derived facets of a job
// from its raw content fields: the company and public slugs, the geography
// (countries/regions/work-mode), and the skill tags. It is the single home for the
// derivation shared by the ingest pipeline and the moderator write path, so both
// produce identical facets from the same inputs.
package jobderive

import (
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/classify"
	"github.com/strelov1/freehire/internal/dict/engagement"
	"github.com/strelov1/freehire/internal/dict/lang"
	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/dict/perks"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/jobfacts"
	"github.com/strelov1/freehire/internal/platform/stringset"
)

// Derive computes the slugs and dictionary facets for a job. Geography, skills, and the
// title classification (seniority/category) come from the curated dictionaries (which
// emit nothing for what they cannot resolve). A structured signal supplied on the Input
// takes precedence over the dictionary for the facets that carry one: work-mode
// (structured → location hint → description), seniority (structured → title →
// description), category (structured → title), experience (structured → description),
// and skills (structured ∪ dictionary).
func Derive(in Input) Derived {
	// The dictionaries read the title with its invisible format characters cut
	// (normalize.CleanTitle); the public slug below is still minted from in.Title, so a
	// stored row re-derived by backfill-derive keeps its published URL.
	title := normalize.CleanTitle(in.Title)
	// The adapter's own country lets a line that names only a place resolve it — a Buenos
	// Aires barrio, a small or a contested town, the city's metro (location.ParseIn). A
	// structured country is final (it replaces the countries below); a fallback one is
	// read only once it is what the posting ends up placed in, below.
	geo := location.ParseIn(in.Location, in.Countries)
	// Geography precedence: the location dictionary → a title-embedded restriction
	// marker → an eligibility signal in the description → an explicit region-scoping
	// statement in the description → the caller's FallbackCountries. When the location left the geography unpinned (no
	// country, and either no region or only an explicit open-anywhere marker's global
	// bucket), each lower source is tried in turn and only fills what every higher
	// source left blank — a resolved place, from the location or from a higher
	// source in this chain, is never overridden. This is the same shape the
	// work-mode fallback below uses.
	countries, regions := geo.Countries, geo.Regions
	if geoUnpinned(countries, regions) {
		// A title-embedded restriction marker ("[Remote-US]", "(Location - Australia
		// or New Zealand)") is a more literal, ATS-authored signal than prose, so it
		// is tried before the description-based rescues below.
		if c, r := location.RestrictionFromTitle(title); len(r) > 0 {
			countries, regions = c, r
		}
	}
	if geoUnpinned(countries, regions) {
		if c, r := location.EligibilityFromDescription(in.Description); len(r) > 0 {
			countries, regions = c, r
		}
	}
	if geoUnpinned(countries, regions) {
		// An explicit "based/located/hiring in <place list>" restriction statement,
		// additive to (and checked after) the citizenship-phrase rescue above — see
		// location.RegionScopeFromDescription.
		if c, r := location.RegionScopeFromDescription(in.Description); len(r) > 0 {
			countries, regions = c, r
		}
	}
	// The fallback country fills only a posting that stated NO geography — not even an
	// explicit open-anywhere marker, which geoUnpinned counts as unpinned but which a
	// headquarters office must not narrow to one country.
	if len(countries) == 0 && len(regions) == 0 && len(in.FallbackCountries) > 0 {
		countries = in.FallbackCountries
		regions = regionsForCountries(in.FallbackCountries)
		if len(in.Countries) == 0 {
			geo = location.ParseIn(in.Location, in.FallbackCountries)
		}
	}
	// An explicit country signal is authoritative: it fully replaces the derived/US-override
	// countries, the same way a stated work mode replaces the hint — and pulls its region
	// along with it (the same pairing Parse itself does for a country it resolves), so a
	// structured country with no separate region statement still lands under the right
	// region facet instead of falling back to whatever the free-text location parsed.
	// The exception is a location line whose own scope is at least as broad: then the
	// adapter's country is an office address (Ashby's postalAddress US beside "Remote |
	// North America or Europe"), and replacing the line with it narrowed a Europe-open
	// role to one country.
	lineWins := len(in.Countries) > 0 && lineOutranksCountries(geo, in.Countries)
	if len(in.Countries) > 0 && !lineWins {
		countries = in.Countries
		regions = regionsForCountries(in.Countries)
	}
	// An explicit region signal is authoritative: it fully replaces the derived regions
	// (and the country-derived regions above), the same way a stated work mode replaces
	// the hint. It is folded into the region vocabulary first: a source (or a stored
	// row fed back by backfill-derive) stating a country, a subdivision or a free
	// phrase as a "region" must not reach the facet, and one that folds to nothing
	// is no signal at all.
	if r := location.NormalizeRegions(in.Regions); len(r) > 0 {
		if lineWins {
			// rederive feeds a stored row's regions back beside its country; a line that
			// outranks the country must not be narrowed back by them.
			regions = unionSorted(regions, r)
		} else {
			regions = r
		}
	}
	// Parse already drops a global the location line contradicts; this applies the same
	// rule to the final answer, whose countries and regions may come from a structured
	// signal instead — an adapter's own country beside an "Anywhere" line, or a stored
	// row's columns fed back by backfill-derive.
	regions = location.WithoutShadowedGlobal(countries, regions, in.Location)
	// An explicit city signal likewise replaces the dictionary's cities outright.
	cities := geo.Cities
	if len(in.Cities) > 0 {
		cities = in.Cities
	}
	metros := metrosForOverride(geo.Metros, in.Cities, in.Countries, countries)
	// Work-mode precedence: structured (ATS) → location marker → description phrase.
	// Each lower source only fills a value the higher ones left empty.
	workMode := in.WorkMode
	if workMode == "" {
		workMode = geo.WorkMode
	}
	if workMode == "" {
		workMode = location.WorkModeFromDescription(in.Description)
	}
	// Explicit arrangement prose fills a gap or contradicts a remote location bucket or ATS flag.
	// Hybrid is checked before a remote denial: "not remote" alone does not distinguish
	// an office-only role from one split between office and home.
	if workMode == "remote" || workMode == "" {
		switch {
		case location.HybridArrangementStated(in.Description):
			workMode = "hybrid"
		case workMode == "remote" && location.RemoteContradicted(in.Description):
			workMode = "onsite"
		}
	}
	class := classify.Parse(title)
	// Seniority precedence: structured source signal → title dictionary → description
	// phrase. Each lower source only fills a grade the higher ones left empty.
	seniority := in.Seniority
	if seniority == "" {
		seniority = class.Seniority
	}
	if seniority == "" {
		seniority = classify.SeniorityFromDescription(in.Description)
	}
	// Category precedence: structured source signal → title dictionary → a
	// non-tech match in the description (NonTechFromDescription resolves only
	// non-technical categories, feeding the enrichment cost gate; a title-silent
	// tech job stays empty). Each lower source only fills what the higher left empty.
	category := in.Category
	if category == "" {
		category = class.Category
	}
	if category == "" {
		category = classify.NonTechFromDescription(in.Description)
	}
	isTech := deriveIsTech(category, title, in.IsTechHint)
	clearanceLevels := location.ClearanceLevelsFromDescription(in.Description)
	// Employment-type precedence: structured source signal (an ATS timeType /
	// typeOfEmployment enum) → free-text description parse. The source's own value wins.
	employmentType := in.EmploymentType
	if employmentType == "" {
		employmentType = jobfacts.EmploymentType(title, in.Description)
	}
	// Experience precedence: structured source signal → description text parse.
	//
	// The ceiling is applied AFTER the choice, to whichever source won, because both
	// can overshoot: the text parse cannot tell a candidate's years from an employer's ("over
	// 40 years of … experience, chipmakers rely on KLA"), and a structured field can carry an
	// undeclared unit. One bound after the merge catches both. See vocab.MaxExperienceYears.
	experience := in.ExperienceYearsMin
	if experience == nil {
		experience = jobfacts.ExperienceYearsMin(in.Description)
	}
	experience = believableExperience(experience)
	// Education and English precedence: structured source signal → description text
	// parse. Both matchers read English prose only, so on a posting written in another
	// language they are the weaker source by a wide margin, not merely the later one.
	educationLevel := in.EducationLevel
	if educationLevel == "" {
		educationLevel = jobfacts.EducationLevel(in.Description)
	}
	englishLevel := in.EnglishLevel
	if englishLevel == "" {
		englishLevel = jobfacts.EnglishLevel(in.Description)
	}
	return Derived{
		CompanySlug: normalize.CompanySlug(in.Company),
		PublicSlug:  normalize.JobSlug(in.Title, in.Company, in.Source, in.ExternalID),
		Countries:   countries,
		Regions:     regions,
		Cities:      cities,
		Metros:      metros,
		WorkMode:    workMode,
		// Skills is a set: the structured source skills are unioned with the
		// dictionary skills, neither replacing the other. The category-scoped
		// acronym tier (e.g. RAG on an ai_engineering/ml_ai job) takes the
		// resolved local `category` — NOT in.Category — so it also fires when the
		// title dictionary supplied the category, the common case since most
		// sources carry no structured category signal.
		Skills:             unionSkills(in.Skills, skilltag.Parse(in.Description, skilltag.WithAcronymCategory(category))),
		Seniority:          seniority,
		Category:           category,
		IsTech:             isTech,
		RequiresClearance:  trueOrNil(len(clearanceLevels) > 0),
		ViaRecruiter:       trueOrNil(jobfacts.ViaRecruiter(in.Description)),
		Equity:             jobfacts.Equity(in.Title, in.Description),
		ClearanceLevels:    clearanceLevels,
		Perks:              perks.FromDescription(in.Description),
		Engagement:         deriveEngagement(in.Engagement, title, in.Description),
		PostingLanguage:    lang.Detect(in.Description),
		EmploymentType:     employmentType,
		EducationLevel:     educationLevel,
		EnglishLevel:       englishLevel,
		ExperienceYearsMin: experience,
		SalaryMin:          in.SalaryMin,
		SalaryMax:          in.SalaryMax,
		SalaryCurrency:     in.SalaryCurrency,
		SalaryPeriod:       in.SalaryPeriod,
	}
}

func RetainEmploymentType(stored, title, description string) string {
	if stored == "part_time" && jobfacts.PartTimePayPolicyOnly(title, description) {
		return ""
	}
	return stored
}

// deriveEngagement is the engagement column for a posting: the structured signal unioned
// with what the title and description state, in vocab order, anything outside the
// vocabulary dropped; nil when neither names a term.
func deriveEngagement(signal []string, title, description string) []string {
	text := engagement.FromText(normalize.CleanTitle(title), description)
	var out []string
	for _, v := range vocab.EngagementValues {
		if slices.Contains(signal, v) || slices.Contains(text, v) {
			out = append(out, v)
		}
	}
	return out
}

// trueOrNil lifts a dictionary's boolean into a true-or-absent column: a positive is
// asserted, a negative stays unknown (nil), never false. A dictionary reports "not
// stated", and storing that as false would read as "stated to be unnecessary" — a
// different claim, and one the catalogue almost never makes.
func trueOrNil(v bool) *bool {
	if !v {
		return nil
	}
	return &v
}

// deriveIsTech computes the tri-state is_tech signal from the already-resolved
// category, the raw title, and an optional structured source hint. The hint,
// when true, wins outright — ahead of both the tech and non-tech evidence below
// — since it is a fact the source stated about its own crawl scope, not a
// heuristic. Absent a hint, technical evidence wins and is checked first: a
// recognized technical category OR a confident software/IT title (classify.IsTech)
// yields true; otherwise a known non-technical category or a confident non-tech
// title yields false; otherwise the signal is unknown (nil), never coerced, so the
// unclassified mass stays measurable. The tech title detector is the symmetric
// counterpart to IsNonTech — it rescues generic software titles ("Software
// Engineer", "COBOL Programmer") that resolve no sub-category. A non-software
// "…Engineer" matches the non-tech detector only where its discipline is named
// outright ("mechanical engineer"), so "Drainage Engineer" matches neither and stays unknown.
//
// The two kinds of technical evidence are NOT weighed the same. A technical TITLE
// (classify.IsTech) wins outright: it speaks for the whole role, so "Backend Engineer —
// Teller Systems" stays technical despite its accidental "teller". A technical CATEGORY
// does not: it is resolved from a SUBSTRING that can be about something else ("Quality
// Assurance Nurse Manager" resolves to `qa`, "Registered Nurse Intensive Mobile Team" to
// `mobile`), so a non-tech title outvotes a category-only signal.
func deriveIsTech(category, title string, sourceHint bool) *bool {
	if sourceHint || classify.IsTech(title) {
		t := true
		return &t
	}
	nonTech := slices.Contains(vocab.NonTechCategories, category) || classify.IsNonTech(title)
	if slices.Contains(vocab.TechCategories, category) && !nonTech {
		t := true
		return &t
	}
	if nonTech {
		f := false
		return &f
	}
	return nil
}

// regionsForCountries derives the region codes for a set of already-resolved country
// codes via the same country→region grouping Parse itself consults (location.CountryToRegion),
// so a structured country signal with no separate region statement still lands under its
// region facet. A country outside the curated ~133-country set silently contributes no
// region — the same never-guess contract the location dictionary follows.
func regionsForCountries(countries []string) []string {
	countryToRegion := location.CountryToRegion()
	set := make(map[string]struct{}, len(countries))
	for _, c := range countries {
		if r, ok := countryToRegion[c]; ok {
			set[r] = struct{}{}
		}
	}
	return stringset.Sorted(set)
}

// lineOutranksCountries reports whether the location line states a scope at least as broad as
// the adapter's structured countries, so that the adapter's country is an office address
// (Ashby's postalAddress) rather than a narrowing. Two shapes qualify:
//   - the line names a macro-region outright, one none of its own countries explains
//     ("Remote | Europe", "North America or Europe"). The open-anywhere "global" does not
//     count, so a country beside "Anywhere" still pins the posting;
//   - the line is countries alone, no city, and names every structured country ("US or
//     Canada" beside US). A line with a city keeps yielding even when it names the country
//     too: a state code beside a town is where the dictionary misreads a second country,
//     which is what the structured one corrects.
func lineOutranksCountries(geo location.Geo, structured []string) bool {
	own := regionsForCountries(geo.Countries)
	for _, r := range geo.Regions {
		if r != "global" && !slices.Contains(own, r) {
			return true
		}
	}
	if len(geo.Cities) > 0 {
		return false
	}
	for _, c := range structured {
		if !slices.Contains(geo.Countries, c) {
			return false
		}
	}
	return true
}

// unionSorted is the sorted, deduplicated union of two string sets.
func unionSorted(a, b []string) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for _, s := range a {
		set[s] = struct{}{}
	}
	for _, s := range b {
		set[s] = struct{}{}
	}
	return stringset.Sorted(set)
}

// geoUnpinned reports whether the location dictionary left a job's geography unpinned —
// no country resolved, and either no region or only the global bucket an explicit
// open-anywhere marker resolves to. It is the gate for the eligibility override in
// Derive: a resolved country or a specific region (e.g. "eu" from "Europe") is left
// untouched, so the override only rescues a bare or "open anywhere" posting that also
// carries a stated eligibility requirement.
func geoUnpinned(countries, regions []string) bool {
	if len(countries) != 0 {
		return false
	}
	return len(regions) == 0 || (len(regions) == 1 && regions[0] == "global")
}

// unionSkills merges the structured source skills with the dictionary skills into a
// deduped, sorted set. Both inputs are already canonical skill names; the result is
// nil when both are empty (the dictionary's empty-not-nil contract is preserved by
// callers that read Skills, which treat nil and empty alike).
func unionSkills(source, dict []string) []string {
	if len(source) == 0 {
		return dict
	}
	set := make(map[string]struct{}, len(source)+len(dict))
	for _, s := range source {
		set[s] = struct{}{}
	}
	for _, s := range dict {
		set[s] = struct{}{}
	}
	return stringset.Sorted(set)
}

// believableExperience drops a years-of-experience figure outside the range a posting
// is believed to be stating about the candidate, leaving the facet unstated rather than
// clamped: a clamp would assert a requirement the posting never made.
func believableExperience(n *int) *int {
	if n == nil || (*n >= 0 && *n <= vocab.MaxExperienceYears) {
		return n
	}
	return nil
}

// metrosForOverride makes the metros follow the structured geography override the way
// cities and countries do, so a posting never carries a metro of a place it no longer
// states. Adapter cities are re-resolved by bare name under the final country, and only
// when there is exactly one (the same one-stated-country rule location.Parse applies);
// an adapter country alone keeps the line's metros that lie in one of the final
// countries.
func metrosForOverride(parsed, inCities, inCountries, countries []string) []string {
	if len(inCities) > 0 {
		if len(countries) != 1 {
			return nil
		}
		return location.MetrosForCities(inCities, countries[0])
	}
	if len(inCountries) == 0 {
		return parsed
	}
	var out []string
	for _, m := range parsed {
		for _, c := range countries {
			if strings.HasSuffix(m, " ("+strings.ToUpper(c)+")") {
				out = append(out, m)
				break
			}
		}
	}
	return out
}
