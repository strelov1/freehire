// Package jobderive computes the deterministic, dictionary-derived facets of a job
// from its raw content fields: the company and public slugs, the geography
// (countries/regions/work-mode), and the skill tags. It is the single home for the
// derivation shared by the ingest pipeline and the moderator write path, so both
// produce identical facets from the same inputs.
package jobderive

import (
	"slices"

	"github.com/strelov1/freehire/internal/classify"
	"github.com/strelov1/freehire/internal/jobfacts"
	"github.com/strelov1/freehire/internal/lang"
	"github.com/strelov1/freehire/internal/location"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/skilltag"
	"github.com/strelov1/freehire/internal/stringset"
	"github.com/strelov1/freehire/internal/vocab"
)

// Input is the raw job content the derivation reads. Source and ExternalID are the
// already-resolved storage identity (the caller decides how to namespace them); they
// feed the public slug so it is stable with the dedup key.
//
// WorkMode, Seniority, Category, Skills, and ExperienceYearsMin are the caller's
// STRUCTURED signals — values the source states explicitly (an enum, a grade, a tag
// list, a numeric field), already mapped into freehire's vocabularies. Each takes
// precedence over the dictionary derivation: the scalar signals win when present and
// the dictionary fills only when they are empty/nil; the multi-valued Skills are
// unioned with the dictionary skills. They carry structured signal only, never a
// heuristic an adapter inferred from free text; an adapter with no signal leaves them
// empty/nil so the dictionary decides.
//
// Countries, Regions and Cities are the caller's structured GEOGRAPHY signal (a country
// code the platform states directly, macro-region codes, and city names the
// submitter/moderator stated by hand). Unlike the scalars they fully replace the
// location-dictionary derivation for that facet when present — an explicit place is
// authoritative, not a hint the dictionary refines — and when empty the dictionary
// derives as before.
type Input struct {
	Title              string
	Company            string
	Source             string
	ExternalID         string
	Location           string
	Description        string
	WorkMode           string
	Countries          []string
	Regions            []string
	Cities             []string
	Seniority          string
	Category           string
	EmploymentType     string
	Skills             []string
	ExperienceYearsMin *int
	// SalaryMin/SalaryMax/SalaryCurrency/SalaryPeriod are the caller's structured
	// salary signal — an adapter sets them only when the ATS states a salary in its
	// own structured field (never a description-text guess: unlike the scalars
	// above, there is no dictionary fallback for salary, so this is pure passthrough).
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string
	SalaryPeriod   string
}

// Derived is the set of facets computed from an Input.
type Derived struct {
	CompanySlug string
	PublicSlug  string
	Countries   []string
	Regions     []string
	Cities      []string
	WorkMode    string
	Skills      []string
	Seniority   string
	Category    string
	// IsTech is the tri-state technical/non-technical signal: non-nil true when the
	// derived category is a recognized technical category, non-nil false when it is a
	// known non-technical category or the title states a confident non-tech role, and
	// nil when neither resolves (unknown — never coerced, so the coverage gap stays
	// measurable).
	IsTech *bool
	// Synthetic enrichment facets (category B): deterministic stand-ins for fields
	// the LLM also emits, served dict-only like the facets above. ExperienceYearsMin
	// is nil when the description states no figure.
	PostingLanguage    string
	EmploymentType     string
	EducationLevel     string
	EnglishLevel       string
	ExperienceYearsMin *int
	// SalaryMin/SalaryMax/SalaryCurrency/SalaryPeriod pass Input's structured salary
	// signal through unchanged — see Input's doc for why there is no dictionary
	// fallback here.
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string
	SalaryPeriod   string
}

// Derive computes the slugs and dictionary facets for a job. Geography, skills, and the
// title classification (seniority/category) come from the curated dictionaries (which
// emit nothing for what they cannot resolve). A structured signal supplied on the Input
// takes precedence over the dictionary for the facets that carry one: work-mode
// (structured → location hint → description), seniority (structured → title →
// description), category (structured → title), experience (structured → description),
// and skills (structured ∪ dictionary).
func Derive(in Input) Derived {
	geo := location.Parse(in.Location)
	// Geography precedence: the location dictionary → a US-only description signal.
	// When the location left the geography unpinned (no country, and either no region
	// or only the bare-"Remote" global bucket) but the description states a hard
	// US-only eligibility requirement, the job is US-restricted, not open-anywhere —
	// pin it to the US so it leaves Global/Worldwide. Like the work-mode fallback
	// below, prose is the lowest-priority source and only fills what the location
	// dictionary left blank; a resolved place is never overridden.
	countries, regions := geo.Countries, geo.Regions
	if usOnly(countries, regions, in.Description) {
		countries, regions = []string{"us"}, []string{"north_america"}
	}
	// An explicit country signal is authoritative: it fully replaces the derived/US-override
	// countries, the same way a stated work mode replaces the hint — and pulls its region
	// along with it (the same pairing Parse itself does for a country it resolves), so a
	// structured country with no separate region statement still lands under the right
	// region facet instead of falling back to whatever the free-text location parsed.
	if len(in.Countries) > 0 {
		countries = in.Countries
		regions = regionsForCountries(in.Countries)
	}
	// An explicit region signal is authoritative: it fully replaces the derived regions
	// (and the country-derived regions above), the same way a stated work mode replaces
	// the hint.
	if len(in.Regions) > 0 {
		regions = in.Regions
	}
	// An explicit city signal likewise replaces the dictionary's cities outright.
	cities := geo.Cities
	if len(in.Cities) > 0 {
		cities = in.Cities
	}
	// Work-mode precedence: structured (ATS) → location marker → description phrase.
	// Each lower source only fills a value the higher ones left empty.
	workMode := in.WorkMode
	if workMode == "" {
		workMode = geo.WorkMode
	}
	if workMode == "" {
		workMode = location.WorkModeFromDescription(in.Description)
	}
	class := classify.Parse(in.Title)
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
	isTech := deriveIsTech(category, in.Title)
	// Employment-type precedence: structured source signal (an ATS timeType /
	// typeOfEmployment enum) → free-text description parse. The source's own value wins.
	employmentType := in.EmploymentType
	if employmentType == "" {
		employmentType = jobfacts.EmploymentType(in.Title, in.Description)
	}
	// Experience precedence: structured source signal → description text parse.
	experience := in.ExperienceYearsMin
	if experience == nil {
		experience = jobfacts.ExperienceYearsMin(in.Description)
	}
	return Derived{
		CompanySlug: normalize.Slug(in.Company),
		PublicSlug:  normalize.JobSlug(in.Title, in.Company, in.Source, in.ExternalID),
		Countries:   countries,
		Regions:     regions,
		Cities:      cities,
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
		PostingLanguage:    lang.Detect(in.Description),
		EmploymentType:     employmentType,
		EducationLevel:     jobfacts.EducationLevel(in.Description),
		EnglishLevel:       jobfacts.EnglishLevel(in.Description),
		ExperienceYearsMin: experience,
		SalaryMin:          in.SalaryMin,
		SalaryMax:          in.SalaryMax,
		SalaryCurrency:     in.SalaryCurrency,
		SalaryPeriod:       in.SalaryPeriod,
	}
}

// deriveIsTech computes the tri-state is_tech signal from the already-resolved
// category and the raw title. Technical evidence wins and is checked first: a
// recognized technical category OR a confident software/IT title (classify.IsTech)
// yields true; otherwise a known non-technical category or a confident non-tech
// title yields false; otherwise the signal is unknown (nil), never coerced, so the
// unclassified mass stays measurable. The tech title detector is the symmetric
// counterpart to IsNonTech — it rescues generic software titles ("Software
// Engineer", "COBOL Programmer") that resolve no sub-category. A non-software
// "…Engineer" matches the non-tech detector only where its discipline is named
// outright — "mechanical engineer", "civil engineer" and the rest of that anchored
// family — so "Drainage Engineer" still matches neither detector and stays unknown.
func deriveIsTech(category, title string) *bool {
	if TechEvidence(category, title) {
		t := true
		return &t
	}
	if slices.Contains(vocab.NonTechCategories, category) || classify.IsNonTech(title) {
		f := false
		return &f
	}
	return nil
}

// TechEvidence reports whether a job's category or title states a confidently technical
// role. It is the positive half of the is_tech derivation, exposed because the ordering
// it enforces — technical evidence before the non-tech dictionary — is load-bearing
// outside this package too: the ingest filter and the prune rule both delete on the
// non-tech dictionary, and that dictionary matches anywhere in a title, so without this
// veto "Backend Engineer — Teller Systems" is removed on its accidental match.
func TechEvidence(category, title string) bool {
	return slices.Contains(vocab.TechCategories, category) || classify.IsTech(title)
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

// usOnly reports whether a job's geography is unpinned by the location dictionary —
// no country resolved, and either no region or only the bare-"Remote" global bucket —
// while its description carries a hard US-only eligibility signal. It is the gate for
// the US geography override in Derive: a resolved country or a specific region (e.g.
// "eu" from "Europe") is left untouched, so the override only rescues the exact case a
// bare-"Remote" posting with a US-citizenship/clearance requirement falls into.
func usOnly(countries, regions []string, desc string) bool {
	if len(countries) != 0 {
		return false
	}
	if len(regions) > 1 || (len(regions) == 1 && regions[0] != "global") {
		return false
	}
	return location.USOnlyFromDescription(desc)
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
