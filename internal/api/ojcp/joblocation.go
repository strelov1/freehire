package ojcp

import (
	"strings"

	"github.com/strelov1/freehire/internal/job/jobview"
)

// Place is OJCP's location block — one schema.org Place, singular. That shape is the whole
// difficulty of this projection: a posting open across several countries does not have one
// place, and the standard offers nowhere else to say so.
type Place struct {
	Type    string  `json:"@type,omitempty"`
	Address Address `json:"address"`
}

// Address is schema.org's postal address, as OJCP narrows it.
type Address struct {
	AddressLocality string `json:"addressLocality,omitempty"`
	// AddressRegion is a STATE OR PROVINCE. It is deliberately never filled from our own
	// Regions facet, which holds macro-regions ("europe", "global") — a different kind of
	// thing entirely. An agent reading addressRegion="europe" would file the posting under
	// a province by that name.
	AddressRegion  string `json:"addressRegion,omitempty"`
	AddressCountry string `json:"addressCountry,omitempty"`
}

// jobLocationFor states a place only where the posting names exactly one, per facet.
//
// Where a facet is ambiguous — three countries, two cities — that facet is left out rather
// than resolved by picking the first value. The list order carries no precedence, so a
// choice would be arbitrary, and an agent filtering on the result would drop the posting
// everywhere the employer also accepts. The two facets are judged independently: one
// country with two cities still states the country, because withholding a fact because a
// neighbouring one is unknown serves less than the posting says.
//
// Multi-country reach is simply not expressible in OJCP v0.1. It is the clearest gap this
// implementation has found in the schema so far, and a candidate for the same RFC that
// argues for a posting-reality field.
func jobLocationFor(j jobview.Job) *Place {
	// The country facet is served lowercased (jobview.normalizeSet), while addressCountry is
	// the ISO 3166-1 alpha-2 code, canonically UPPERCASE. An agent comparing against "DE"
	// or feeding the value to a country library misses every posting otherwise.
	address := Address{
		AddressLocality: onlyValue(j.Cities),
		AddressCountry:  strings.ToUpper(onlyValue(j.Countries)),
	}
	if address == (Address{}) {
		return nil
	}
	return &Place{Type: "Place", Address: address}
}

// onlyValue returns the single element of values, or "" when there is not exactly one.
func onlyValue(values []string) string {
	if len(values) != 1 {
		return ""
	}
	return values[0]
}
