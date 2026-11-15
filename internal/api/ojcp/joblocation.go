package ojcp

import (
	"encoding/json"
	"strings"

	"github.com/strelov1/freehire/internal/job/jobview"
)

// Place is OJCP's location block: one schema.org Place, singular, though a posting can be
// open across several countries.
type Place struct {
	Type    string  `json:"@type,omitempty"`
	Address Address `json:"address"`
}

// Address is schema.org's postal address, as OJCP narrows it.
type Address struct {
	AddressLocality string `json:"addressLocality,omitempty"`
	// AddressRegion is a state or province. Never fill it from our Regions facet, which
	// holds macro-regions ("europe", "global").
	AddressRegion  string `json:"addressRegion,omitempty"`
	AddressCountry string `json:"addressCountry,omitempty"`
}

// Places is jobLocation: one Place or an array. A single place is written as a bare object
// so consumers built on the singular shape keep working.
type Places []Place

// MarshalJSON writes a single Place as an object and anything longer as an array.
func (p Places) MarshalJSON() ([]byte, error) {
	if len(p) == 1 {
		return json.Marshal(p[0])
	}
	return json.Marshal([]Place(p))
}

// jobLocationFor states every place the posting names, as precisely as the facets allow.
//
// Countries and cities are separate facets, so a city is paired with a country only when
// there is exactly one country. With several, cities are left out and each country is its
// own Place (Warsaw paired with DE would be a workplace that does not exist). A remote
// posting states countries only: a city on it is usually the employer's office.
func jobLocationFor(j jobview.Job) Places {
	countries := make([]string, len(j.Countries))
	for i, country := range j.Countries {
		// The facet is lowercased (jobview.normalizeSet); addressCountry is canonically
		// uppercase ISO 3166-1 alpha-2.
		countries[i] = strings.ToUpper(country)
	}
	if j.WorkMode == "remote" || len(countries) > 1 || len(j.Cities) == 0 {
		var places Places
		for _, country := range countries {
			places = append(places, place(Address{AddressCountry: country}))
		}
		return places
	}
	country := ""
	if len(countries) == 1 {
		country = countries[0]
	}
	places := make(Places, len(j.Cities))
	for i, city := range j.Cities {
		places[i] = place(Address{AddressLocality: city, AddressCountry: country})
	}
	return places
}

func place(address Address) Place {
	return Place{Type: "Place", Address: address}
}
