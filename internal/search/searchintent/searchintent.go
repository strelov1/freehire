// Package searchintent turns a natural-language description of a job search into
// canonical, filter-ready facet values.
//
// # The grounding rule
//
// search.FilterFromValues ignores parameters and values it does not recognise, so an
// unresolved value yields an unfiltered result set that reads as a confident answer.
// This package exists to prevent that:
//
//   - A value no dictionary resolves is dropped and reported.
//   - A name no filter has ever had is refused, failing the whole interpretation: the
//     model is confused about the product, and "the rest" would answer another question.
//   - A real filter this surface cannot ground (a company name) is an ordinary miss:
//     dropped and reported, the rest stands.
//
// The model proposes; the dictionaries dispose. Nothing unresolved reaches a filter.
package searchintent

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/collections"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/search/search"
)

// Result is one interpreted search: the values a filter can be built from, and what
// could not be resolved.
type Result struct {
	// Facets are canonical values keyed by public facet name, ready for the search filter.
	Facets map[string][]string
	// Exclude are canonical values the search must NOT match, keyed the same way.
	Exclude map[string][]string
	// Scalars are the numeric and boolean bounds the search accepts beside the facets.
	Scalars Scalars
	// Query is free text for a concept no facet expresses. It is ANDed against the whole
	// document, so a term duplicating a facet narrows twice.
	Query string
	// Summary is one sentence describing the search, from the same response as the values.
	Summary string
	// Unresolved lists dropped values verbatim as the model wrote them.
	Unresolved []string
}

// Scalars are the non-facet bounds. Pointers because unset and zero differ: a zero
// experience ceiling is the entry-level filter.
type Scalars struct {
	SalaryMin          *int
	PostedWithinDays   *int
	ExperienceYearsMax *int
	VisaSponsorship    bool
}

// Empty reports that nothing survived resolution. Applying an empty filter shows the
// whole catalogue, so callers must tell it from a successful interpretation.
func (r Result) Empty() bool {
	return len(r.Facets) == 0 &&
		len(r.Exclude) == 0 &&
		r.Query == "" &&
		r.Scalars.SalaryMin == nil &&
		r.Scalars.PostedWithinDays == nil &&
		r.Scalars.ExperienceYearsMax == nil &&
		!r.Scalars.VisaSponsorship
}

// reground puts a result the caller echoed back through the same dictionaries a model's
// proposal faces. The payload reaches the prompt and is untrusted: an arbitrary key or
// value would otherwise inject text into the model's instructions.
//
// Unknown names are dropped, not refused as resolve does: from a model an unknown name
// means confusion, but a key we never wrote in an echoed payload is just noise.
func (r Result) reground() Result {
	out, err := intent{
		Facets:             onlyOfferedFacets(r.Facets),
		Exclude:            onlyOfferedFacets(r.Exclude),
		Query:              r.Query,
		SalaryMin:          r.Scalars.SalaryMin,
		PostedWithinDays:   r.Scalars.PostedWithinDays,
		ExperienceYearsMax: r.Scalars.ExperienceYearsMax,
		VisaSponsorship:    r.Scalars.VisaSponsorship,
	}.resolve()
	if err != nil {
		return Result{}
	}
	return out
}

// onlyOfferedFacets keeps the entries this surface actually serves. Nil in, nil out.
func onlyOfferedFacets(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for name, values := range in {
		if _, offered := facetResolvers[name]; offered {
			out[name] = values
		}
	}
	return out
}

// intent is the model's raw proposal, before any dictionary has seen it. Values are
// ordinary words ("Golang", "Portugal"); only resolve turns this into a Result.
type intent struct {
	Facets             map[string][]string `json:"facets"`
	Exclude            map[string][]string `json:"exclude"`
	Query              string              `json:"query"`
	Summary            string              `json:"summary"`
	SalaryMin          *int                `json:"salary_min"`
	PostedWithinDays   *int                `json:"posted_within_days"`
	ExperienceYearsMax *int                `json:"experience_years_max"`
	VisaSponsorship    bool                `json:"visa_sponsorship"`
}

// resolver returns the canonical form the index stores for one written value, or !ok when
// no dictionary places it. It never guesses.
type resolver func(raw string) (canonical string, ok bool)

// facetResolvers maps every facet the model may write to its canonicalising rule. A facet
// absent here cannot be expressed, so resolve refuses the interpretation.
var facetResolvers = map[string]resolver{
	// Closed vocabularies: the model is asked for canonical values directly and only
	// checked. Defined in internal/dict/vocab, so an accepted value is one the index holds.
	"work_mode":       oneOf(vocab.WorkModeValues),
	"regions":         oneOf(vocab.RegionValues),
	"employment_type": oneOf(vocab.EmploymentTypeValues),
	"relocation":      oneOf(vocab.RelocationValues),
	"salary_period":   oneOf(vocab.SalaryPeriodValues),
	"seniority":       oneOf(vocab.SeniorityValues),
	"role_type":       oneOf(vocab.RoleTypeValues),
	"english_level":   oneOf(vocab.EnglishLevelValues),
	"education_level": oneOf(vocab.EducationLevelValues),
	"category":        oneOf(vocab.CategoryValues),
	"company_type":    oneOf(vocab.CompanyTypeValues),
	"company_size":    oneOf(vocab.CompanySizeValues),
	"ai_archetype":    oneOf(vocab.AIArchetypeValues),
	"domains":         oneOf(vocab.DomainValues),

	// A curated closed list, assembled at startup.
	"collections": oneOf(collections.Slugs()),

	// Open vocabularies: the model writes ordinary words and the owning dictionary decides.
	"skills":    resolveSkill,
	"countries": resolveCountry,
	"cities":    resolveCity,
}

// refuseAll is the resolver for a real filter this surface cannot ground: its values are
// dropped and reported like any other miss.
func refuseAll(string) (string, bool) { return "", false }

// oneOf accepts a value that is already canonical, case-insensitively.
func oneOf(values []string) resolver {
	set := make(map[string]string, len(values))
	for _, v := range values {
		set[strings.ToLower(v)] = v
	}
	return func(raw string) (string, bool) {
		canonical, ok := set[strings.ToLower(strings.TrimSpace(raw))]
		return canonical, ok
	}
}

// resolveSkill reads a skill through the alias dictionary ingest tags jobs with, so
// "Golang" and "go" match the same postings. Unknown tokens yield nothing.
func resolveSkill(raw string) (string, bool) {
	canonical := skilltag.Canonicalize([]string{raw})
	if len(canonical) != 1 {
		return "", false
	}
	return canonical[0], true
}

// resolveCountry takes only the country, never the region location also derives: asking
// for Portugal is not asking for the EU.
func resolveCountry(raw string) (string, bool) {
	countries := location.Parse(raw).Countries
	if len(countries) != 1 {
		return "", false
	}
	return countries[0], true
}

// cityMatchWindow is how many SearchCities answers to scan for the written name.
// Results rank by population, so an exact name can sit behind a longer prefix match
// ("Bath" after "Bathinda").
const cityMatchWindow = 12

// resolveCity requires the dictionary to return a city whose name equals what was
// written. location.Parse passes unrecognised tokens through as cities, and SearchCities
// matches on prefix ("Ber" finds Berlin), so neither is usable alone.
func resolveCity(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	for _, match := range location.SearchCities(name, "", cityMatchWindow) {
		if strings.EqualFold(match.Name, name) {
			return match.Name, true
		}
	}
	return "", false
}

// Ceilings on the scalar filters. Absurdity guards, not policy: a miscounted zero would
// filter the catalogue to nothing. Unusual but real values (a $500k floor) pass. A year
// of freshness is the whole catalogue, so anything past it bounds nothing.
const (
	maxSalaryFloor      = 10_000_000
	maxExperienceYears  = 60
	maxPostedWithinDays = 365
	// The free-text fields are shown to a person and echoed into the next prompt, so
	// they are bounded.
	maxQueryRunes   = 200
	maxSummaryRunes = 400
)

// bound accepts a proposed scalar inside [low, high] and appends anything else to
// unresolved.
func bound(name string, v *int, low, high int, unresolved []string) (*int, []string) {
	if v == nil {
		return nil, unresolved
	}
	// The model writes 0 for "not setting this"; drop it silently. Safe only for bounds
	// with low > 0 — the experience ceiling, where 0 is a filter, is asked for as text.
	if *v == 0 && low > 0 {
		return nil, unresolved
	}
	if *v < low || *v > high {
		return nil, append(unresolved, fmt.Sprintf("%s=%d", name, *v))
	}
	return v, unresolved
}

// resolve canonicalises the model's proposal. An unknown facet name is the one condition
// reported as an error rather than a drop. Facets are walked in name order so the
// unresolved report is stable.
func (in intent) resolve() (Result, error) {
	out := Result{
		// Bounded here for both producers: the model and a caller echoing a result back.
		Query:   llm.TruncateRunes(strings.TrimSpace(in.Query), maxQueryRunes),
		Summary: llm.TruncateRunes(strings.TrimSpace(in.Summary), maxSummaryRunes),
	}
	keep := func(name string, v *int, low, high int) *int {
		var kept *int
		kept, out.Unresolved = bound(name, v, low, high, out.Unresolved)
		return kept
	}
	out.Scalars.VisaSponsorship = in.VisaSponsorship
	out.Scalars.SalaryMin = keep("salary_min", in.SalaryMin, 1, maxSalaryFloor)
	out.Scalars.PostedWithinDays = keep("posted_within_days", in.PostedWithinDays, 1, maxPostedWithinDays)
	out.Scalars.ExperienceYearsMax = keep("experience_years_max", in.ExperienceYearsMax, 0, maxExperienceYears)

	var err error

	if out.Facets, out.Unresolved, err = canonicaliseFacets(in.Facets, out.Unresolved); err != nil {
		return Result{}, err
	}
	if out.Exclude, out.Unresolved, err = canonicaliseFacets(in.Exclude, out.Unresolved); err != nil {
		return Result{}, err
	}
	dropContradictions(out.Facets, out.Exclude)
	dropRedundantGeography(out.Facets, out.Exclude)
	return out, nil
}

// keepExclusions narrows one excluded facet to the values keep accepts, deleting the
// facet when nothing survives so no empty heading renders.
func keepExclusions(exclude map[string][]string, name string, keep func(value string) bool) {
	kept := exclude[name][:0:0]
	for _, v := range exclude[name] {
		if keep(v) {
			kept = append(kept, v)
		}
	}
	if len(kept) == 0 {
		delete(exclude, name)
		return
	}
	exclude[name] = kept
}

// dropContradictions removes an excluded value that is also included; the inclusion
// wins, as in the profile seed and URL parser. Such a filter matches nothing.
//
// The drop is not reported: both sides resolved and the value is applied as an inclusion.
func dropContradictions(facets, exclude map[string][]string) {
	for name := range exclude {
		keepExclusions(exclude, name, func(v string) bool {
			return !slices.Contains(facets[name], v)
		})
	}
}

// dropRedundantGeography removes a place that choosing another place already left out.
//
// Regions are disjoint (the UK is its own region, not part of eu), so excluding a second
// region only strips roles spanning both: "Europe but not the UK" must keep pan-European
// roles. Countries are covered too, since models often say "not the UK" as a country.
//
// It does not generalise: a country sits inside a region, so "not Germany" survives
// choosing Europe, and "Go but not PHP" excludes real postings by design.
//
// The prompt says this too but the model does not follow it reliably.
func dropRedundantGeography(facets, exclude map[string][]string) {
	chosen := facets["regions"]
	// `global` chooses every area, so an exclusion is the only thing narrowing the search
	// ("anywhere remote, but not the USA") and must stay.
	if len(chosen) == 0 || slices.Contains(chosen, "global") {
		return
	}
	delete(exclude, "regions")

	countryRegion := location.CountryToRegion()
	keepExclusions(exclude, "countries", func(code string) bool {
		region, known := countryRegion[code]
		// An unplaced country proves nothing; dropping its exclusion would widen the search.
		return !known || slices.Contains(chosen, region)
	})
}

// canonicaliseFacets resolves one side of the filter (include or exclude) through the
// same dictionaries. Names are walked in order so the unresolved report is stable.
func canonicaliseFacets(in map[string][]string, unresolved []string) (map[string][]string, []string, error) {
	out := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(in)) {
		canonicalise, offered := facetResolvers[name]
		if !offered {
			if _, real := search.StringFacets[name]; !real {
				return nil, nil, fmt.Errorf("searchintent: no filter named %q", name)
			}
			canonicalise = refuseAll
		}
		for _, raw := range in[name] {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			canonical, ok := canonicalise(raw)
			if !ok {
				unresolved = append(unresolved, raw)
				continue
			}
			if !slices.Contains(out[name], canonical) {
				out[name] = append(out[name], canonical)
			}
		}
	}
	return out, unresolved, nil
}
