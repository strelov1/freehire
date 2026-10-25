package location

import (
	"bufio"
	_ "embed"
	"strings"
	"sync"
)

// smallPlacesTSV lists the GeoNames places below the main city table's population floor
// (cmd/gen-cities' statingPopulation), generated beside cities1000.tsv. Columns:
// canonical-name, ISO country code, '|'-joined lowercase aliases (the place's own name and
// ASCII name only).
//
// It exists for the German-speaking crawl, where most postings name a town of a few
// thousand people beside its Land: measured 2026-10-25 over 124k open postings from seven
// German ATSs, 29.5k carried no city, and 18k of those named a place in this table next to
// a Land or country that already placed them. It is not German-specific — a small town
// beside its region reads the same way anywhere.
//
//go:embed smallplaces.tsv
var smallPlacesTSV string

// smallPlace is one place the table names: its display name and its country.
type smallPlace struct {
	Name    string
	Country string
}

// smallPlaces is loaded on first use rather than at init: the table is as large as the
// main one, and most processes that import this package (the API server among them)
// never parse a location line that misses the main table.
var smallPlaces = sync.OnceValue(func() map[string][]smallPlace {
	return loadSmallPlaces(smallPlacesTSV)
})

func loadSmallPlaces(tsv string) map[string][]smallPlace {
	out := map[string][]smallPlace{}
	sc := bufio.NewScanner(strings.NewReader(tsv))
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if line == "" || line[0] == '#' {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		p := smallPlace{Name: parts[0], Country: parts[1]}
		for _, alias := range strings.Split(parts[2], "|") {
			out[alias] = append(out[alias], p)
		}
	}
	return out
}

// pendingCity is a token whose city NAME is known only per country: a small place, or the
// town in a "Rottenburg am Neckar" / "Köln-Niehl" shape. It names a city only once the
// whole line has been read and has stated one of those countries — see resolvePending.
type pendingCity map[string]string // country -> display name

// add records a candidate, keeping the first name seen per country. Two DIFFERENT names
// for one country under one token are a real ambiguity, and the country is dropped.
func (p pendingCity) add(country, name string) {
	if prev, ok := p[country]; ok && prev != name {
		p[country] = ""
		return
	}
	p[country] = name
}

// smallPlaceWithheld are small-place names that a German location line uses as the
// QUALIFIER after a slash rather than as a place: "Lauenburg/Elbe" (the river; Elbe is
// also a village in Lower Saxony) and "Lunz/See" (the lake; See is a village in Tyrol).
// Found by hand-checking what the table newly named over the German crawl. The other
// river names a slash carries there ("Spree", "Oder", "Erzgebirge") name no small place,
// and the real towns that share a river's name ("Pegnitz", "Regen", "Lech") stay.
var smallPlaceWithheld = map[string]bool{"elbe": true, "see": true}

// smallPlaceCandidates is the pending city a token reads as through the small-places
// table, nil when it names none.
func smallPlaceCandidates(tok string) pendingCity {
	if smallPlaceWithheld[tok] {
		return nil
	}
	places := smallPlaces()[tok]
	if len(places) == 0 {
		return nil
	}
	p := pendingCity{}
	for _, sp := range places {
		p.add(sp.Country, sp.Name)
	}
	return p
}

// townSuffixes separate a German town from the river, region or neighbour that
// disambiguates it ("Rottenburg am Neckar", "Weiden in der Oberpfalz", "Ergolding bei
// Landshut"). The town before them is the place; the main table and the small one both
// know most of these towns by their bare name.
var townSuffixes = []string{" am ", " an der ", " im ", " in der ", " bei ", " ob der ", " b. "}

// stripTownSuffix returns the town before a townSuffixes word, or "" when there is none.
func stripTownSuffix(tok string) string {
	for _, s := range townSuffixes {
		if i := strings.Index(tok, s); i > 0 {
			return strings.TrimSpace(tok[:i])
		}
	}
	return ""
}

// pendingFor collects what an unresolved token may still name, in order of confidence: the
// token itself as a small place; the town before a townSuffixes word; the large city a
// "City-District" token opens with (longest prefix first, so "baden-baden-oos" is
// Baden-Baden and not Baden). A district is read only off the main table: the pattern is a
// city large enough to have named districts, and a small-place lead would read
// "Horn-Bad Meinberg" (one town) as "Horn".
func pendingFor(tok string) pendingCity {
	if p := smallPlaceCandidates(tok); p != nil {
		return p
	}
	if town := stripTownSuffix(tok); town != "" {
		if ce, ok := cityDict[town]; ok {
			return pendingCity{ce.Country: ce.Name}
		}
		if p := smallPlaceCandidates(town); p != nil {
			return p
		}
	}
	segs := strings.Split(tok, "-")
	for n := len(segs) - 1; n >= 1; n-- {
		lead := strings.TrimSpace(strings.Join(segs[:n], "-"))
		if ce, ok := cityDict[lead]; ok {
			return pendingCity{ce.Country: ce.Name}
		}
	}
	return nil
}

// resolvePending names the pending cities whose country the line stated, adding them to
// citySet. It never adds a country: a pending city is a name waiting for a place, and a
// line that placed nothing — or placed two countries the candidate both matches under
// different names — leaves it unnamed.
func resolvePending(pending []pendingCity, countrySet, citySet map[string]struct{}) {
	for _, p := range pending {
		name, ambiguous := "", false
		for country := range countrySet {
			n, ok := p[country]
			if !ok {
				continue
			}
			if n == "" || (name != "" && n != name) {
				ambiguous = true
				break
			}
			name = n
		}
		if name != "" && !ambiguous {
			citySet[name] = struct{}{}
		}
	}
}
