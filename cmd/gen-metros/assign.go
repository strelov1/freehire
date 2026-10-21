package main

import (
	"fmt"
	"sort"
	"strings"
)

var project = mollweide

type fua struct {
	id                     int
	pop                    float64 // FUA_p_2015
	country                string  // lower-case alpha-2
	geom                   multiPolygon
	minX, minY, maxX, maxY float64
}

type gnPlace struct {
	name     string // GeoNames display name
	country  string // lower-case alpha-2
	admin1   string // postal code for us/ca, "" otherwise
	lat, lon float64
	pop      int64
	region   string // GeoNames admin1 name, any country
	district string // GeoNames admin2 name
}

type assigned struct {
	place gnPlace
	fuaID int // noFUA when the place lies in no FUA
}

// noFUA marks a place that lies in no functional urban area. Such places are still
// written: the loader needs them to see that a bare name is shared with a place in
// no metro ("Jackson" is Jackson MS, in no FUA, as well as Jackson NJ, in one).
const noFUA = -1

// assign returns, for each place, the FUA of ITS country containing it (FUAs are
// already split per country in the source: Tijuana MEX and Tijuana USA are two
// features), or noFUA. Every place is returned.
func assign(places []gnPlace, fuas []fua) []assigned {
	byCountry := map[string][]fua{}
	for _, f := range fuas {
		byCountry[f.country] = append(byCountry[f.country], f)
	}
	var out []assigned
	for _, p := range places {
		x, y := project(p.lat, p.lon)
		id := noFUA
		for _, f := range byCountry[p.country] {
			if x < f.minX || x > f.maxX || y < f.minY || y > f.maxY {
				continue
			}
			if f.geom.contains(x, y) {
				id = f.id
				break
			}
		}
		out = append(out, assigned{place: p, fuaID: id})
	}
	return out
}

// qualifyStats counts how many metros were qualified by region (admin1) vs district (admin2).
var qualifyStats = map[string]int{}

// metroNames names each FUA by the most populous place assigned to it, unless
// overrides names it; returns fuaID → "Name (CC)". When several FUAs of one country
// share a value the one with the largest population (pops) keeps it plain and every
// other gets a qualifier inserted before the country suffix: the postal admin1 code for us/ca
// ("Springfield MO (US)"), the admin1 name elsewhere ("Fuyang Anhui (CN)"). A qualifier
// that is empty, repeats the name or collides with another value falls through to
// the admin2 name; a collision that survives that is an error naming the FUA ids,
// to be settled with an override.
func metroNames(as []assigned, pops map[int]float64, overrides map[int]string) (map[int]string, error) {
	best := map[int]gnPlace{}
	for _, a := range as {
		if a.fuaID == noFUA {
			continue
		}
		if b, ok := best[a.fuaID]; !ok || a.place.pop > b.pop {
			best[a.fuaID] = a.place
		}
	}
	base := map[int]string{}
	names := map[int]string{}
	owners := map[string][]int{}
	for id, b := range best {
		name := b.name
		if o, ok := overrides[id]; ok {
			name = o
		}
		base[id] = name
		v := name + " (" + strings.ToUpper(b.country) + ")"
		names[id] = v
		owners[v] = append(owners[v], id)
	}
	taken := map[string]bool{}
	var losers []int
	for v, ids := range owners {
		sort.Slice(ids, func(i, j int) bool {
			if pops[ids[i]] != pops[ids[j]] {
				return pops[ids[i]] > pops[ids[j]]
			}
			return ids[i] < ids[j]
		})
		taken[v] = true
		losers = append(losers, ids[1:]...)
	}
	sort.Ints(losers)
	var clashes []string
	for _, id := range losers {
		p := best[id]
		cc := strings.ToUpper(p.country)
		first, kind := p.region, "admin1"
		if p.admin1 != "" {
			first = p.admin1
		}
		done := false
		for _, c := range []struct{ q, kind string }{{first, kind}, {p.district, "admin2"}} {
			if c.q == "" || strings.EqualFold(c.q, base[id]) {
				continue
			}
			v := base[id] + " " + c.q + " (" + cc + ")"
			if taken[v] {
				continue
			}
			taken[v] = true
			names[id] = v
			qualifyStats[c.kind]++
			done = true
			break
		}
		if !done {
			clashes = append(clashes, fmt.Sprintf("%q: FUA id %d", names[id], id))
		}
	}
	if len(clashes) > 0 {
		return nil, fmt.Errorf("metro names collide, add an override for each: %s", strings.Join(clashes, "; "))
	}
	seen := map[string]int{}
	for id, v := range names {
		if strings.Contains(v, ",") {
			return nil, fmt.Errorf("metro name %q (FUA id %d) contains a comma, add an override", v, id)
		}
		if other, ok := seen[v]; ok {
			return nil, fmt.Errorf("metro name %q is held by FUA ids %d and %d", v, min(id, other), max(id, other))
		}
		seen[v] = id
	}
	return names, nil
}
