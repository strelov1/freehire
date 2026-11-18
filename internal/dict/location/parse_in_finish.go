package location

import (
	"strings"

	"github.com/strelov1/freehire/internal/platform/stringset"
)

// finish resolves what the loop held back until the whole line was read, and builds the Geo.
func (p *parseState) finish() Geo {
	// Before the long-tail fallback below, never after: a pending city waits for a country
	// the line STATED, and a country inferred from another city is not that.
	stated := p.countrySet
	if len(stated) == 0 && p.placeToks == 1 {
		stated = p.hint
	}
	resolvePending(p.pending, stated, p.citySet)
	// Keep aliases separate until their metros are resolved: two segments can name
	// different places called Franklin, each with its own metro.
	aliasesBySegment := map[string]map[string]struct{}{}
	for _, h := range p.aliasCities {
		if aliasesBySegment[h.segment] == nil {
			aliasesBySegment[h.segment] = map[string]struct{}{}
		}
		addAliasCity(aliasesBySegment[h.segment], h.ce, h.tok, stated, p.segmentSubdivisions[h.segment])
	}
	p.applyCityFallback()
	metroCountry := p.metroCountry()
	metros := p.withRegionMetros(metroCountry, p.cityMetros(metroCountry, aliasesBySegment))

	countries := stringset.Sorted(p.countrySet)
	regions := WithoutShadowedGlobal(countries, stringset.Sorted(p.regionSet), p.lower)
	mode := detectWorkMode(cutIndiaWidePhrases(p.lower))

	return Geo{
		Countries: countries,
		Regions:   regions,
		Cities:    stringset.Sorted(p.citySet),
		WorkMode:  mode,
		Metros:    metros,
	}
}

// applyCityFallback states the long-tail cities' country when the line stated none.
//
// Nothing in the line stated a country, so the long-tail cities get to — but only
// with one voice. Two guards, and the change is wrong without either:
//
// The country must be unstated. A city is the WEAKEST geographic statement in a
// location line, so it never contributes alongside a stated country: "Anna,
// Illinois, United States" names a town in Russia AND the state that actually
// places the job, and "Crossroads - London" names a US locality beside the city
// that matters.
//
// The cities must agree. Each is individually unambiguous, but a line naming two
// of them ("Recife, Benidorm") is not, and picking the first would make the answer
// depend on word order — a guess wearing a determinism costume.
func (p *parseState) applyCityFallback() {
	if len(p.countrySet) != 0 || len(p.cityFallbackCountries) != 1 {
		return
	}
	for country := range p.cityFallbackCountries {
		p.countrySet[country] = struct{}{}
		if r, ok := countryToRegion[country]; ok {
			p.regionSet[r] = struct{}{}
		}
	}
}

// metroCountry is the one country metros are looked up in, or "".
// A metro needs one stated country: a line naming two cannot place a city in either
// without guessing.
func (p *parseState) metroCountry() string {
	switch {
	case len(p.countrySet) == 1:
		for c := range p.countrySet {
			return c
		}
	case len(p.countrySet) == 0 && len(p.hint) == 1:
		for c := range p.hint {
			return c
		}
	}
	return ""
}

// cityMetros adds the held aliases to the cities and returns the sorted metros of all of
// them. The plain cities' metros are read before the aliases join citySet: an alias is
// matched against its own segment's subdivisions only.
func (p *parseState) cityMetros(metroCountry string, aliasesBySegment map[string]map[string]struct{}) []string {
	metroSet := map[string]struct{}{}
	if metroCountry != "" {
		for name := range p.citySet {
			if v, ok := metroFor(name, metroCountry, p.statedSubdivisions); ok {
				metroSet[v] = struct{}{}
			}
		}
	}
	for segment, names := range aliasesBySegment {
		for name := range names {
			p.citySet[name] = struct{}{}
			if metroCountry != "" {
				if v, ok := metroFor(name, metroCountry, p.segmentSubdivisions[segment]); ok {
					metroSet[v] = struct{}{}
				}
			}
		}
	}
	return stringset.Sorted(metroSet)
}

// withRegionMetros merges the metros of the metropolitan regions the line named.
// A metropolitan region the line named ("Delhi NCR", "GBA", a Buenos Aires barrio) adds
// its metro, in that metro's country only.
func (p *parseState) withRegionMetros(metroCountry string, metros []string) []string {
	if metroCountry == "" || len(p.regionMetros) == 0 {
		return metros
	}
	suffix := " (" + strings.ToUpper(metroCountry) + ")"
	for m := range p.regionMetros {
		if !strings.HasSuffix(m, suffix) {
			delete(p.regionMetros, m)
		}
	}
	if len(p.regionMetros) == 0 {
		return metros
	}
	for _, m := range metros {
		p.regionMetros[m] = struct{}{}
	}
	return stringset.Sorted(p.regionMetros)
}
