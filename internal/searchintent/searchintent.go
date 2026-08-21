// Package searchintent turns a natural-language description of a job search into
// canonical, filter-ready facet values.
//
// # The grounding rule
//
// search.FilterFromValues ignores parameters and values it does not recognise, so a
// value that never resolves does not narrow anything — it produces an UNFILTERED
// result set that reads as a confident answer. That is the failure this package
// exists to prevent, and it shapes every decision here:
//
//   - A value no dictionary resolves is dropped AND reported. An unreported drop is,
//     to the caller, indistinguishable from a value that was applied.
//   - A name no filter has ever had is refused outright, taking the whole
//     interpretation with it. The model is confused about what this product can do,
//     and returning "the rest" would quietly answer a different question.
//   - A real filter this surface cannot ground — a company name, which no dictionary
//     here can tell from a typo — is an ordinary miss: dropped and reported, while
//     everything else the caller asked for still stands.
//
// The model proposes; the dictionaries dispose. Nothing the model writes reaches a
// filter unresolved.
package searchintent

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/collections"
	"github.com/strelov1/freehire/internal/location"
	"github.com/strelov1/freehire/internal/roletag"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/skilltag"
	"github.com/strelov1/freehire/internal/vocab"
)

// Result is one interpreted search: the values a filter can be built from, and an
// honest account of what could not be turned into one.
type Result struct {
	// Facets are canonical values keyed by the public facet name, ready to be handed
	// to the search filter verbatim.
	Facets map[string][]string
	// Exclude are canonical values the search must NOT match, keyed the same way. An
	// exclusion hides postings rather than showing too many, so an unresolved one is
	// the worse half of the silent-filter failure.
	Exclude map[string][]string
	// Scalars are the numeric and boolean bounds the search accepts beside the facets.
	Scalars Scalars
	// Query is free text for a concept no facet expresses, and nothing else. It is an
	// AND against the whole document, so a term that duplicates a facet narrows the
	// results twice; kept separate from Facets so a caller can lift it on its own.
	Query string
	// Summary is one sentence describing the search this result builds. It comes from
	// the same model response as the values, so it can never describe a different
	// search than the one that was resolved.
	Summary string
	// Unresolved names what was dropped, verbatim as the model wrote it, so the caller
	// can show what was not understood rather than silently narrowing their search.
	Unresolved []string
}

// Scalars are the non-facet bounds. Each is a pointer because "unset" and "zero" are
// different searches: a zero experience ceiling is the entry-level filter, and a zero
// salary floor is no filter at all.
type Scalars struct {
	SalaryMin          *int
	PostedWithinDays   *int
	ExperienceYearsMax *int
	VisaSponsorship    bool
}

// Empty reports that nothing survived resolution — no facet, no bound, no query. The
// caller must be able to tell this apart from a successful interpretation, because
// applying an empty filter shows the whole catalogue, which reads as "everything
// matches you" rather than as "I did not understand".
func (r Result) Empty() bool {
	return len(r.Facets) == 0 &&
		len(r.Exclude) == 0 &&
		r.Query == "" &&
		r.Scalars.SalaryMin == nil &&
		r.Scalars.PostedWithinDays == nil &&
		r.Scalars.ExperienceYearsMax == nil &&
		!r.Scalars.VisaSponsorship
}

// intent is the model's raw proposal, before any dictionary has seen it. Its values
// are ordinary words — "Golang", "Portugal", "senior" — because enumerating the open
// vocabularies in the prompt would cost more than resolving them afterwards and would
// still leave the model free to invent. Only resolve turns this into a Result.
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

// resolver decides what one written value means, returning the canonical form the
// index stores. Reporting !ok is how a resolver says "no dictionary places this" —
// there is no third answer, because a guess is what this package refuses to make.
type resolver func(raw string) (canonical string, ok bool)

// facetResolvers is the vocabulary: every facet the model may write, mapped to the
// rule that canonicalises one of its values. A facet absent from this map cannot be
// expressed, which is why resolve refuses the whole interpretation rather than
// returning a narrower search than the one that was asked for.
var facetResolvers = map[string]resolver{
	// Closed vocabularies. Small enough to name in the prompt, so the model is asked
	// for canonical values directly and only has to be checked, not translated. They
	// come from internal/vocab, the single definition these values have — a value
	// accepted here is a value the index actually holds.
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

	// Curated lists, closed but assembled at startup rather than written out here.
	"collections": oneOf(collections.Slugs()),
	"role":        oneOf(slices.Collect(maps.Keys(roletag.Catalog()))),

	// Open vocabularies: thousands of values each, so the model writes ordinary words
	// and the dictionary that already owns the vocabulary decides what they mean.
	"skills":    resolveSkill,
	"countries": resolveCountry,
	"cities":    resolveCity,
}

// refuseAll is the resolver for a real filter this surface cannot ground. Its values
// are dropped and reported like any other miss, so one unofferable filter costs the
// caller that filter and not the rest of their search.
func refuseAll(string) (string, bool) { return "", false }

// oneOf accepts a value that is already canonical, case-insensitively. Building the
// set once per facet keeps the check O(1) and, more importantly, makes the vocabulary
// a fact about the facet rather than a loop written at each call site.
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

// resolveSkill reads one written skill through the alias dictionary ingest tags jobs
// with, so "Golang" and "go" reach the same postings. Canonicalize never guesses: a
// token outside the dictionary yields nothing rather than passing through.
func resolveSkill(raw string) (string, bool) {
	canonical := skilltag.Canonicalize([]string{raw})
	if len(canonical) != 1 {
		return "", false
	}
	return canonical[0], true
}

// resolveCountry takes only the country the location dictionary reads, never the
// region it also derives — a caller who asked for Portugal did not ask for the EU, and
// writing both would widen the search past the question.
func resolveCountry(raw string) (string, bool) {
	countries := location.Parse(raw).Countries
	if len(countries) != 1 {
		return "", false
	}
	return countries[0], true
}

// resolveCity requires the dictionary to return a city whose name IS what was written.
//
// location.Parse cannot be used here: it passes an unrecognised token through as a
// city, which is exactly the unresolved-value-in-a-filter failure this package exists
// to prevent. SearchCities is a real lookup, but it matches on prefix, so it answers
// "Ber" with Berlin — a city nobody asked for. Demanding the whole name back is what
// separates a lookup from a guess.
func resolveCity(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	matches := location.SearchCities(name, "", 1)
	if len(matches) == 0 || !strings.EqualFold(matches[0].Name, name) {
		return "", false
	}
	return matches[0].Name, true
}

// Bounds on the scalar filters. They are absurdity guards, not policy: a salary floor
// of a billion or a four-century experience ceiling is a model that miscounted a zero,
// and letting it through would filter the catalogue down to nothing while looking like
// a search. Real-but-unusual values (a $500k floor) pass, because the sidebar renders
// whatever bound is live and a person can see and lift it.
// A freshness ceiling of a year is already "everything the catalogue holds", so a
// larger number is not a search — it is a bound that filters nothing while looking
// like one.
const (
	maxSalaryFloor      = 10_000_000
	maxExperienceYears  = 60
	maxPostedWithinDays = 365
)

// bound accepts a proposed scalar inside [low, high] and reports anything else as a
// drop, appended to unresolved — a bound that vanished without a word is the same lie
// as a hallucinated facet.
func bound(name string, v *int, low, high int, unresolved []string) (*int, []string) {
	if v == nil {
		return nil, unresolved
	}
	// A zero is how the model writes "I am not setting this" — observed on every live
	// call, whatever the schema says about null. Reporting it would put a line in front
	// of the user about a value that was never meant. This is safe only because both
	// callers have a low bound above zero; the experience ceiling, where zero IS a
	// filter, is asked for as text precisely so it never arrives here ambiguous.
	if *v == 0 && low > 0 {
		return nil, unresolved
	}
	if *v < low || *v > high {
		return nil, append(unresolved, fmt.Sprintf("%s=%d", name, *v))
	}
	return v, unresolved
}

// resolve canonicalises the model's proposal. See the package comment for the rule it
// enforces; an unknown facet name is the one condition it reports as an error rather
// than as a drop.
//
// Facets are walked in name order so one interpretation always renders the same way —
// the unresolved report is shown to a person, and a set that reshuffles between two
// identical requests reads as instability rather than as an answer.
func (in intent) resolve() (Result, error) {
	out := Result{
		Query:   strings.TrimSpace(in.Query),
		Summary: strings.TrimSpace(in.Summary),
	}
	var err error
	out.Scalars.VisaSponsorship = in.VisaSponsorship
	out.Scalars.SalaryMin, out.Unresolved = bound("salary_min", in.SalaryMin, 1, maxSalaryFloor, out.Unresolved)
	out.Scalars.ExperienceYearsMax, out.Unresolved = bound("experience_years_max", in.ExperienceYearsMax, 0, maxExperienceYears, out.Unresolved)
	out.Scalars.PostedWithinDays, out.Unresolved = bound("posted_within_days", in.PostedWithinDays, 1, maxPostedWithinDays, out.Unresolved)

	if out.Facets, out.Unresolved, err = canonicaliseFacets(in.Facets, out.Unresolved); err != nil {
		return Result{}, err
	}
	if out.Exclude, out.Unresolved, err = canonicaliseFacets(in.Exclude, out.Unresolved); err != nil {
		return Result{}, err
	}
	dropContradictions(out.Facets, out.Exclude)
	dropRedundantRegionExclusions(out.Facets, out.Exclude)
	return out, nil
}

// dropRedundantRegionExclusions removes an excluded region once any region is chosen.
//
// The regions are disjoint areas — the UK is its own region, NOT part of eu — so
// choosing one already answers "which area". Excluding another on top can only strip
// the roles that span both, and "somewhere in Europe but not the UK" is not a request
// to lose the pan-European roles: those are exactly what that person wants.
//
// The prompt says this too, and saying it there is the cheaper fix. It is not enough:
// measured against the live gateway, the same request came back clean 3 times in 5 and
// carried the redundant exclusion the other 2. A rule that holds three fifths of the
// time is not a rule, and the failure is invisible — a struck-through UK chip looks
// like the filter working.
//
// Scoped to regions on purpose. A COUNTRY sits inside a region, so choosing Europe says
// nothing about Germany and "not Germany" still needs its exclusion. And a skill is a
// requirement rather than a place: "Go but not PHP" excludes real postings by design.
func dropRedundantRegionExclusions(facets, exclude map[string][]string) {
	chosen := facets["regions"]
	if len(chosen) == 0 {
		return
	}
	// `global` is "open anywhere", so it chooses every area rather than one of them.
	// Under it an exclusion is the ONLY thing narrowing the search — "anywhere remote,
	// but not the USA" — and dropping it would widen the result to the whole catalogue
	// while the summary still promised otherwise.
	if slices.Contains(chosen, "global") {
		return
	}
	delete(exclude, "regions")

	// Half the models say "not the UK" as a COUNTRY rather than as the region it is.
	// Same redundancy and the same struck-through chip, so the rule has to follow it
	// there — but only for a country whose whole area is already left out by the
	// regions chosen. Germany under a chosen `eu` is the opposite case: its region WAS
	// chosen, so "not Germany" is the only way to say it and must survive.
	countryRegion := location.CountryToRegion()
	kept := exclude["countries"][:0:0]
	for _, code := range exclude["countries"] {
		if !slices.Contains(chosen, countryRegion[code]) {
			continue
		}
		kept = append(kept, code)
	}
	if len(kept) == 0 {
		delete(exclude, "countries")
		return
	}
	exclude["countries"] = kept
}

// dropContradictions removes an excluded value that is also included. The inclusion
// wins, the same way the profile seed and the URL parser settle the overlap.
//
// A filter that excludes what it includes matches nothing, and a zero-result list reads
// as "we carry no such jobs" rather than as "we contradicted ourselves". Observed live:
// asked to change an onsite Berlin search to "remote in Europe", the model returned
// regions=[eu] alongside exclude.regions=[eu].
//
// The drop is NOT reported. Nothing failed to resolve — both sides resolved perfectly,
// and the value IS applied, as an inclusion. Naming it among the values we could not
// place would be a second, different lie.
func dropContradictions(facets, exclude map[string][]string) {
	for name, excluded := range exclude {
		kept := excluded[:0:0]
		for _, v := range excluded {
			if !slices.Contains(facets[name], v) {
				kept = append(kept, v)
			}
		}
		if len(kept) == 0 {
			delete(exclude, name)
			continue
		}
		exclude[name] = kept
	}
}

// canonicaliseFacets resolves one side of the filter — what to match, or what to rule
// out. Both sides run the same dictionaries: an unresolved exclusion is not the safer
// mistake, it is the worse one, because it hides postings instead of showing extra.
//
// Names are walked in order so one interpretation always renders the same way. The
// unresolved report is shown to a person, and a set that reshuffles between two
// identical requests reads as instability rather than as an answer.
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
