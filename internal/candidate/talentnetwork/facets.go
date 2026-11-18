package talentnetwork

import "context"

// How many members stand behind each filter value, for the visitor's current filter.
// This makes an OPEN vocabulary (thousands of skill canonicals) usable. Computed by
// walking the catalogue's in-memory snapshot, so it needs no index.

// countFloor is the smallest count reported as a number. Below it the value is still
// OFFERED, carrying CountWithheld: the list already shows the member, and removing the
// value would only make the filter unusable.
//
// It is friction, not a guarantee; what protects a member is the projection, since a
// filter narrowed to one person yields one ANONYMOUS card.
const countFloor = 2

// CountWithheld marks a value that is present but whose number is not reported. It is
// NEGATIVE because 0 already means "nobody carries this", and a pane must tell the two apart.
const CountWithheld = -1

// Counts is the facet distribution behind one query. The shape mirrors the job search's
// facet response so the same panes render either.
type Counts struct {
	Total  int                       `json:"total"`
	Facets map[string]map[string]int `json:"facets"`
	// Stats is the ranged-facet summary the job search fills in; always empty here, but
	// kept because the panes read one shape.
	Stats map[string]FacetRange `json:"stats"`
}

// FacetRange is one ranged facet's bounds.
type FacetRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// countableFacet is one facet a pane counts: how to read its values off a member, and how
// to take its OWN selection out of the query. They share an entry so a facet cannot be
// added with one half and silently counted under its own selection (see Facets).
type countableFacet struct {
	values func(CatalogueMember) []string
	clear  func(*Query)
}

// facetOf names every countable facet.
var facetOf = map[string]countableFacet{
	paramCategories: {
		values: func(m CatalogueMember) []string { return single(m.Card.Category) },
		clear:  func(q *Query) { q.Categories = nil },
	},
	paramSeniorities: {
		values: func(m CatalogueMember) []string { return single(m.Card.Seniority) },
		clear:  func(q *Query) { q.Seniorities = nil },
	},
	paramSkills: {
		values: func(m CatalogueMember) []string { return m.Card.Skills },
		clear:  func(q *Query) { q.Skills = nil },
	},
	paramTimezoneRegions: {
		values: func(m CatalogueMember) []string { return single(m.TimezoneRegion) },
		clear:  func(q *Query) { q.TimezoneRegions = nil },
	},
	paramCities: {
		values: func(m CatalogueMember) []string { return m.Cities },
		clear:  func(q *Query) { q.Cities = nil },
	},
	paramSpecializations: {
		values: func(m CatalogueMember) []string { return m.Specializations },
		clear:  func(q *Query) { q.Specializations = nil },
	},
}

// Facets counts the members behind each value of each facet, under q.
//
// EACH FACET IS COUNTED WITH ITS OWN SELECTION REMOVED, while every other filter still
// applies. Otherwise picking "Go" makes every other skill read 0 and the control silently
// becomes single-select. A test enforces it.
func (c *Catalogue) Facets(ctx context.Context, q Query) (Counts, error) {
	snap, err := c.current(ctx)
	if err != nil {
		return Counts{}, err
	}

	counts := Counts{
		Facets: make(map[string]map[string]int, len(facetOf)),
		Stats:  map[string]FacetRange{},
	}

	for _, m := range snap.members {
		if q.matches(m) {
			counts.Total++
		}
	}

	for param, facet := range facetOf {
		without := q.without(param)

		tally := map[string]int{}
		for _, m := range snap.members {
			if !without.matches(m) {
				continue
			}
			for _, v := range facet.values(m) {
				if v != "" {
					tally[v]++
				}
			}
		}
		// Three states a pane must tell apart: counted, withheld (negative), and absent
		// (nobody carries it, so it never entered the tally).
		for v, n := range tally {
			if n < countFloor {
				tally[v] = CountWithheld
			}
		}
		if len(tally) > 0 {
			counts.Facets[param] = tally
		}
	}

	return counts, nil
}

// without returns a copy of q with one facet's selection cleared. Paging is left alone.
func (q Query) without(param string) Query {
	if facet, ok := facetOf[param]; ok {
		facet.clear(&q)
	}
	return q
}

// single wraps a scalar facet value, dropping the empty one.
func single(v string) []string {
	if v == "" {
		return nil
	}
	return []string{v}
}
