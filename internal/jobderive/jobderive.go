// Package jobderive computes the deterministic, dictionary-derived facets of a job
// from its raw content fields: the company and public slugs, the geography
// (countries/regions/work-mode), and the skill tags. It is the single home for the
// derivation shared by the ingest pipeline and the moderator write path, so both
// produce identical facets from the same inputs.
package jobderive

import (
	"sort"

	"github.com/strelov1/freehire/internal/classify"
	"github.com/strelov1/freehire/internal/jobfacts"
	"github.com/strelov1/freehire/internal/lang"
	"github.com/strelov1/freehire/internal/location"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/skilltag"
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
type Input struct {
	Title              string
	Company            string
	Source             string
	ExternalID         string
	Location           string
	Description        string
	WorkMode           string
	Seniority          string
	Category           string
	Skills             []string
	ExperienceYearsMin *int
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
	// Synthetic enrichment facets (category B): deterministic stand-ins for fields
	// the LLM also emits, served dict-only like the facets above. ExperienceYearsMin
	// is nil when the description states no figure.
	PostingLanguage    string
	EmploymentType     string
	EducationLevel     string
	EnglishLevel       string
	ExperienceYearsMin *int
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
	// Category precedence: structured source signal → title dictionary. (Description
	// prose is too noisy to derive a category deterministically.)
	category := in.Category
	if category == "" {
		category = class.Category
	}
	// Experience precedence: structured source signal → description text parse.
	experience := in.ExperienceYearsMin
	if experience == nil {
		experience = jobfacts.ExperienceYearsMin(in.Description)
	}
	return Derived{
		CompanySlug: normalize.Slug(in.Company),
		PublicSlug:  normalize.JobSlug(in.Title, in.Company, in.Source, in.ExternalID),
		Countries:   geo.Countries,
		Regions:     geo.Regions,
		Cities:      geo.Cities,
		WorkMode:    workMode,
		// Skills is a set: the structured source skills are unioned with the
		// dictionary skills, neither replacing the other.
		Skills:             unionSkills(in.Skills, skilltag.Parse(in.Description)),
		Seniority:          seniority,
		Category:           category,
		PostingLanguage:    lang.Detect(in.Description),
		EmploymentType:     jobfacts.EmploymentType(in.Title, in.Description),
		EducationLevel:     jobfacts.EducationLevel(in.Description),
		EnglishLevel:       jobfacts.EnglishLevel(in.Description),
		ExperienceYearsMin: experience,
	}
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
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
