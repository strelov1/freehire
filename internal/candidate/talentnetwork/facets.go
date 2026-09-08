package talentnetwork

import "context"

// How many members stand behind each filter value, for the filter the visitor currently
// has. This is what makes an OPEN vocabulary usable: skills are thousands of canonicals,
// and a control offering them without counts cannot tell a value nobody carries from one
// whose members have all left.
//
// Computed by walking the snapshot the catalogue already holds. At a membership in the
// hundreds that is microseconds and no storage — the reason the catalogue is not indexed
// at all is the same reason this needs no index.

// countFloor is the smallest count reported as a number. Below it the value is still
// OFFERED, carrying CountWithheld, because withholding the option would be the worse
// answer: the member is in the catalogue either way, the list already shows them, and
// removing the value only makes the filter unusable for whoever wants it.
//
// It is friction, not a guarantee. What actually protects a member is the projection: a
// filter narrowed to one person yields one ANONYMOUS card, which is what they agreed to.
// A number beside a name would be a disclosure; a number beside nothing is arithmetic.
const countFloor = 2

// CountWithheld marks a value that is present but whose number is not reported. It is
// NEGATIVE rather than zero on purpose: zero already means something — a value nobody
// carries — and a pane cannot tell "nobody has this" from "somebody has this and we are
// not saying how many" if both arrive as 0. Rendering the first as a bare option and the
// second as "0" would be exactly backwards.
const CountWithheld = -1

// Counts is the facet distribution behind one query. The shape mirrors the job search's
// own facet response so the same panes render either without learning a second
// vocabulary — `stats` is carried for that reason and is empty here, since the catalogue
// has no ranged facet.
type Counts struct {
	Total  int                       `json:"total"`
	Facets map[string]map[string]int `json:"facets"`
	// Stats is the ranged-facet summary the job search fills in (salary, years). The
	// catalogue has no ranged facet, so it is always empty — carried anyway because the
	// panes read one shape, and a response missing a key they expect is a client-side
	// branch nobody would otherwise have to write.
	Stats map[string]FacetRange `json:"stats"`
}

// FacetRange is one ranged facet's bounds. Named rather than inlined twice as an
// anonymous struct, which is what it was and which read as noise at both sites.
type FacetRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// countableFacet is one facet a pane counts: how to read its values off a member, and how
// to take that facet's OWN selection back out of the query. The two halves live in one
// entry on purpose. As a map and a separate switch they were two lists over one set, and a
// facet added to the map alone would then be counted under its own selection — every other
// value in its pane reading zero, the control silently turning single-select. That is the
// exact failure Facets' rule below exists to prevent, and it would arrive silently.
type countableFacet struct {
	values func(CatalogueMember) []string
	clear  func(*Query)
}

// facetOf names every countable facet, so Facets cannot count one the query does not
// filter on, or miss one it does.
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
// applies. That is the whole subtlety. Counting a facet under its own selection makes
// every other value in its pane read zero — pick "Go" and every other skill reads 0 — so
// a visitor can never add a second value and the control silently becomes single-select.
// The rule is invisible from the outside, which is why it has its own test.
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
		// The query as it stands MINUS this facet: everything else still narrows.
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
		// Below the floor the value stays offered, carrying CountWithheld; the surface
		// renders a negative count as an option without a number. A value nobody carries
		// never entered the tally at all, so it is ABSENT — three states, and a pane has
		// to be able to tell them apart.
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

// without returns q with one facet's selection cleared. Paging is irrelevant to a count
// and is left alone rather than zeroed, so the copy stays a plain q minus one field.
func (q Query) without(param string) Query {
	// q is a COPY — clearing a field here cannot reach the caller's query.
	if facet, ok := facetOf[param]; ok {
		facet.clear(&q)
	}
	return q
}

// single wraps a scalar facet value, dropping the empty one — a member whose title
// resolved to no category contributes to no category's count rather than to an empty one.
func single(v string) []string {
	if v == "" {
		return nil
	}
	return []string{v}
}
