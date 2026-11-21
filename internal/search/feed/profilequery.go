// Package feed holds the match feed's server-side pieces (architecture AD-2).
package feed

import (
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/search/search"
)

// ReasonNoUsableFields is returned when a profile yields no filter param at all, so the
// feed reports it instead of searching the whole index (AD-3).
const ReasonNoUsableFields = "no_usable_fields"

// Character budgets per profile list, the same as the web's (profileFilters.ts).
const (
	skillCharBudget   = 800
	sourceCharBudget  = 200
	companyCharBudget = 400
)

// entryExperienceCap mirrors ENTRY_EXPERIENCE_CAP: only entry levels bound the years.
var entryExperienceCap = map[string]int{"intern": 1, "junior": 2}

// ProfileQuery turns a profile into the search params the web's
// filtersToParams(filtersFromProfile(p)) emits for it (web/src/lib/profileFilters.ts, facetModel.ts).
// The two are pinned equal by web/src/lib/fixtures/profileFeedParity.json; change both
// together. reason is ReasonNoUsableFields when no param is emitted, "" otherwise.
func ProfileQuery(p userprofile.Profile) (url.Values, string) {
	f := map[string]*facet{}
	f["category"] = seed(p.Specializations)
	f["posting_language"] = seed(p.JobLanguages)

	var experienceMax *int
	seniorityUnspecified := false
	levels := p.Seniorities
	if len(levels) > 0 && !everyIn(vocab.SeniorityValues, levels) {
		var picked []string
		for _, v := range vocab.SeniorityValues {
			if slices.Contains(levels, v) {
				picked = append(picked, v)
			}
		}
		f["seniority"] = seed(picked)
		seniorityUnspecified = true
		if c, ok := experienceCap(levels); ok {
			experienceMax = &c
		}
	}

	// Include wins: an excluded skill already wanted is dropped.
	skills := seed(charBudget(p.Skills, skillCharBudget))
	for _, v := range charBudget(p.ExcludedSkills, skillCharBudget) {
		if !skills.has(v) {
			skills.exclude = append(skills.exclude, v)
		}
	}
	f["skills"] = skills
	f["source"] = seedExcluded(charBudget(p.ExcludedSources, sourceCharBudget))
	f["company_slug"] = seedExcluded(charBudget(p.ExcludedCompanies, companyCharBudget))

	var remoteAnywhere bool
	var remoteRegions, remoteCountries []string
	// The stored block was validated on save, so a decode failure cannot happen in
	// practice; treating it as absent matches a profile with no location.
	if loc, ok := decodeLocation(p.LocationPreferences); ok {
		reloc := loc.Relocation
		if !reloc.Open {
			reloc = userprofile.Relocation{}
		}
		// Base says where the user lives; it bounds the work only for physical modes.
		base := userprofile.BaseLocation{}
		if slices.ContainsFunc(loc.WorkModes, isPhysicalMode) {
			base = loc.Base
		}
		reachRegions, reachCountries := loc.Remote.Regions, loc.Remote.Countries
		f["work_mode"] = seed(loc.WorkModes)
		f["regions"] = seed(reloc.Regions)
		f["countries"] = seed(append(nonEmpty(base.Country), reloc.Countries...))
		f["cities"] = seed(append(nonEmpty(base.City), reloc.Cities...))
		if slices.Contains(loc.WorkModes, "remote") && appliesRemoteAnywhere(f) {
			remoteAnywhere = true
			remoteRegions = reachRegions
			remoteCountries = reachCountries
			// A stated reach must not drop remote jobs in the user's own country.
			reaches := len(reachRegions) > 0 || len(reachCountries) > 0
			if reaches && base.Country != "" && !slices.Contains(reachCountries, base.Country) {
				remoteCountries = append(slices.Clone(reachCountries), base.Country)
			}
			// No relocation facet here: the server would AND it around the remote arm too.
		} else {
			f["regions"] = seed(append(slices.Clone(reachRegions), reloc.Regions...))
			countries := append(slices.Clone(reachCountries), nonEmpty(base.Country)...)
			f["countries"] = seed(append(countries, reloc.Countries...))
			if loc.Relocation.Open {
				f["relocation"] = seed([]string{"supported", "required"})
			}
		}
	}

	out := url.Values{}
	for param, st := range f {
		if len(st.include) > 0 {
			out.Set(param, strings.Join(st.include, ","))
		}
		if len(st.exclude) > 0 {
			out.Set(param+"_exclude", strings.Join(st.exclude, ","))
		}
	}
	if experienceMax != nil {
		out.Set("experience_years_max", strconv.Itoa(*experienceMax))
		out.Set(search.ExperienceOnlySeniorityUnspecifiedParam, "true")
	}
	if seniorityUnspecified && len(f["seniority"].include) > 0 {
		out.Set(search.SeniorityIncludeUnspecifiedParam, "true")
	}
	if remoteAnywhere {
		out.Set(search.WorkModeRemoteAnywhereParam, "true")
		if len(remoteRegions) > 0 {
			out.Set(search.WorkModeRemoteRegionsParam, strings.Join(remoteRegions, ","))
		}
		if len(remoteCountries) > 0 {
			out.Set(search.WorkModeRemoteCountriesParam, strings.Join(remoteCountries, ","))
		}
	}
	if len(out) == 0 {
		return out, ReasonNoUsableFields
	}
	return out, ""
}

type facet struct{ include, exclude []string }

func (st *facet) has(v string) bool {
	return slices.Contains(st.include, v) || slices.Contains(st.exclude, v)
}

// seed mirrors `values.reduce(facetAdd, emptyFacet())`: trimmed, blanks and repeats skipped.
func seed(values []string) *facet {
	st := &facet{}
	for _, raw := range values {
		if v := trimJS(raw); v != "" && !st.has(v) {
			st.include = append(st.include, v)
		}
	}
	return st
}

// seedExcluded mirrors facetSetSign(…, 'exclude'): a repeat moves to the end.
func seedExcluded(values []string) *facet {
	st := &facet{}
	for _, v := range values {
		st.exclude = append(slices.DeleteFunc(st.exclude, func(x string) bool { return x == v }), v)
	}
	return st
}

// charBudget keeps values in order until maxChars is spent, stopping at a whole value.
// Length is counted in UTF-16 units, as JS's String.length does.
func charBudget(values []string, maxChars int) []string {
	var out []string
	used := 0
	for _, raw := range values {
		v := trimJS(raw)
		if v == "" {
			continue
		}
		cost := len(utf16.Encode([]rune(v)))
		if len(out) > 0 {
			cost++ // the joining comma
		}
		if used+cost > maxChars {
			break
		}
		out = append(out, v)
		used += cost
	}
	return out
}

// experienceCap is the years bound for an entry-level-only profile; any other level
// (including an unknown one) means no bound.
func experienceCap(levels []string) (int, bool) {
	limit := 0
	for _, l := range levels {
		c, ok := entryExperienceCap[l]
		if !ok {
			return 0, false
		}
		limit = max(limit, c)
	}
	return limit, true
}

// appliesRemoteAnywhere mirrors appliesWorkModeRemoteAnywhere: remote plus a physical
// mode plus a known location value, or the server ignores the modifier.
func appliesRemoteAnywhere(f map[string]*facet) bool {
	wm := f["work_mode"]
	if wm == nil {
		return false
	}
	var remote, physical bool
	for _, m := range wm.include {
		m = strings.ToLower(trimJS(m))
		remote = remote || m == "remote"
		physical = physical || isPhysicalMode(m)
	}
	hasLocation := false
	for _, param := range []string{"regions", "countries", "cities", "metros"} {
		if st := f[param]; st != nil {
			for _, v := range st.include {
				hasLocation = hasLocation || knownLocation(param, v)
			}
		}
	}
	return remote && physical && hasLocation
}

// knownLocation: regions is the one closed location facet, so an unknown region is no location.
func knownLocation(param, value string) bool {
	if param != "regions" {
		return true
	}
	v := strings.ToLower(trimJS(value))
	return v == search.RegionUnspecified || slices.Contains(vocab.RegionValues, v)
}

func isPhysicalMode(m string) bool { return m == "hybrid" || m == "onsite" }

func everyIn(all, picked []string) bool {
	for _, v := range all {
		if !slices.Contains(picked, v) {
			return false
		}
	}
	return true
}

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

func decodeLocation(raw json.RawMessage) (userprofile.LocationPreferences, bool) {
	var loc *userprofile.LocationPreferences
	if len(raw) == 0 || json.Unmarshal(raw, &loc) != nil || loc == nil {
		return userprofile.LocationPreferences{}, false
	}
	return *loc, true
}

// trimJS trims what JS's String.prototype.trim does, which differs from
// strings.TrimSpace on U+FEFF (JS trims it) and U+0085 (JS keeps it).
func trimJS(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return r == '\uFEFF' || (r != '\u0085' && unicode.IsSpace(r))
	})
}
