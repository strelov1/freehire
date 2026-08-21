package searchintent

import (
	"maps"
	"slices"
)

// systemPrompt pairs with the response schema (proposal.go): the schema fixes the
// shape and the closed vocabularies, this fixes the meaning.
//
// It says nothing about the values of the open vocabularies. Those are checked against
// the real dictionaries afterwards, and a prompt that also described them would be a
// second, slowly-diverging copy of a dictionary that already exists.
const systemPrompt = `You turn a person's description of the job they want into search filters for a job board.

Fill only the fields the description actually supports. An empty field is a filter that is not applied, which is the correct answer for anything the person did not ask for. Never fill a field to look thorough: every value you write hides postings from them.

Values you are given a list to choose from must come from that list. For the rest:

- skills: name each technology the way it is normally written ("Kubernetes", "React", "Go"). One skill per entry, never a phrase.
- countries: the country's ordinary English name ("Portugal", "Germany").
- cities: the city's bare name ("Lisbon"), with no country or region attached.
- role: a specific role key of the form <seniority>_<role>, e.g. "senior_backend". Leave it empty unless the person named a precise role; the category and seniority fields already carry the general case.

query is free text matched against the whole posting, and it is a last resort. Use it ONLY for something no other field can express — a niche the fields have no word for. Anything a field covers must go in that field: a term written in query as well narrows the results a second time, for no reason.

summary is one plain sentence describing the search you built, in the person's own terms. It is shown to them instead of the raw filters, so it must describe exactly the values you wrote — not what they asked for, if you could not express all of it.`

// sortedFacetNames orders a facet map's names so a rendered search always reads the
// same way. The order reaches both the model (when refining) and the person (in the
// preview), and a set that reshuffles between two identical requests reads as
// instability rather than as an answer.
func sortedFacetNames(facets map[string][]string) []string {
	return slices.Sorted(maps.Keys(facets))
}
