package main

import (
	"fmt"
	"sort"
	"strings"
)

var project = mollweide

type fua struct {
	id                     int
	country                string // lower-case alpha-2
	geom                   multiPolygon
	minX, minY, maxX, maxY float64
}

type gnPlace struct {
	name     string // GeoNames display name
	country  string // lower-case alpha-2
	admin1   string // postal code for us/ca, "" otherwise
	lat, lon float64
	pop      int64
}

type assigned struct {
	place gnPlace
	fuaID int
}

// assign returns, for each place, the FUA of ITS country containing it (FUAs are
// already split per country in the source: Tijuana MEX and Tijuana USA are two
// features). Places in no FUA are dropped.
func assign(places []gnPlace, fuas []fua) []assigned {
	byCountry := map[string][]fua{}
	for _, f := range fuas {
		byCountry[f.country] = append(byCountry[f.country], f)
	}
	var out []assigned
	for _, p := range places {
		x, y := project(p.lat, p.lon)
		for _, f := range byCountry[p.country] {
			if x < f.minX || x > f.maxX || y < f.minY || y > f.maxY {
				continue
			}
			if f.geom.contains(x, y) {
				out = append(out, assigned{place: p, fuaID: f.id})
				break
			}
		}
	}
	return out
}

// metroNames names each FUA by the most populous place assigned to it, unless
// overrides names it; returns fuaID → "Name, CC". Two FUAs of one country that
// end up with the same value get " <admin1>" inserted before the comma when both
// have one ("Springfield MA, US"); a collision that survives that is an error
// naming both FUA ids, to be settled with an override.
func metroNames(as []assigned, overrides map[int]string) (map[int]string, error) {
	best := map[int]gnPlace{}
	for _, a := range as {
		if b, ok := best[a.fuaID]; !ok || a.place.pop > b.pop {
			best[a.fuaID] = a.place
		}
	}
	names := map[int]string{}
	owners := map[string][]int{}
	for id, b := range best {
		name := b.name
		if o, ok := overrides[id]; ok {
			name = o
		}
		v := name + ", " + strings.ToUpper(b.country)
		names[id] = v
		owners[v] = append(owners[v], id)
	}
	var clashes []string
	for v, ids := range owners {
		if len(ids) < 2 {
			continue
		}
		resolved := true
		for _, id := range ids {
			if best[id].admin1 == "" {
				resolved = false
				break
			}
		}
		if resolved {
			name, cc, _ := strings.Cut(v, ", ")
			for _, id := range ids {
				names[id] = name + " " + best[id].admin1 + ", " + cc
			}
			continue
		}
		sort.Ints(ids)
		clashes = append(clashes, fmt.Sprintf("%q: FUA ids %v", v, ids))
	}
	if len(clashes) > 0 {
		sort.Strings(clashes)
		return nil, fmt.Errorf("metro names collide, add an override for each: %s", strings.Join(clashes, "; "))
	}
	return names, nil
}
