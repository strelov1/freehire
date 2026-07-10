package userprofile

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/enrich"
)

// Location-preference sentinel errors, mapped to 400 by the handler.
var (
	// ErrInvalidWorkMode is a work mode outside enrich.WorkModeValues.
	ErrInvalidWorkMode = errors.New("userprofile: work mode is not a known value")
	// ErrInvalidRegion is a region outside enrich.RegionValues.
	ErrInvalidRegion = errors.New("userprofile: region is not a known value")
	// ErrInvalidCountry is a country code that is not a well-formed ISO 3166-1 alpha-2 shape.
	ErrInvalidCountry = errors.New("userprofile: country is not a valid two-letter code")
	// ErrTooManyCountries is a country list past maxCountries.
	ErrTooManyCountries = errors.New("userprofile: too many countries")
	// ErrTooManyCities is a city list past maxCities.
	ErrTooManyCities = errors.New("userprofile: too many cities")
)

// Caps on the free-ish list fields — sanity limits, not domain rules (mirroring
// maxSpecializations). Countries are bounded by the ISO set; cities are free text.
const (
	maxCountries = 20
	maxCities    = 10
)

// LocationPreferences is the optional, structured "where & how I want to work" block on a
// profile: the accepted work arrangements plus three independent, freely-combinable
// geographic parts — the remote reach, the current base, and the relocation willingness.
// Every field is optional; an entirely-empty block is treated as "no preferences" (stored
// NULL). Stored whole as JSONB and echoed back verbatim.
type LocationPreferences struct {
	WorkModes  []string     `json:"work_modes,omitempty"`
	Remote     GeoSet       `json:"remote"`
	Base       BaseLocation `json:"base"`
	Relocation Relocation   `json:"relocation"`
}

// GeoSet is a geographic reach: controlled regions and/or ISO country codes. Empty means
// "anywhere" (worldwide) in the remote context.
type GeoSet struct {
	Regions   []string `json:"regions,omitempty"`
	Countries []string `json:"countries,omitempty"`
}

// BaseLocation is the user's current single place: an ISO country code and a free-text city.
type BaseLocation struct {
	Country string `json:"country,omitempty"`
	City    string `json:"city,omitempty"`
}

// Relocation is the willingness to move: an open flag plus the acceptable destinations.
// Open with empty targets means "anywhere".
type Relocation struct {
	Open      bool     `json:"open"`
	Regions   []string `json:"regions,omitempty"`
	Countries []string `json:"countries,omitempty"`
	Cities    []string `json:"cities,omitempty"`
}

// isEmpty reports whether the block carries no preference at all, so the service can store
// NULL instead of a meaningless empty object.
func (l LocationPreferences) isEmpty() bool {
	return len(l.WorkModes) == 0 &&
		len(l.Remote.Regions) == 0 && len(l.Remote.Countries) == 0 &&
		l.Base.Country == "" && l.Base.City == "" &&
		!l.Relocation.Open &&
		len(l.Relocation.Regions) == 0 && len(l.Relocation.Countries) == 0 && len(l.Relocation.Cities) == 0
}

// normalizeLocationPreferences validates and normalizes the optional location block, then
// marshals it to JSONB. A nil input, or a block that is empty after normalization, yields a
// nil payload (stored as NULL — "no preferences"). Any out-of-vocabulary value rejects the
// whole save.
func normalizeLocationPreferences(loc *LocationPreferences) (json.RawMessage, error) {
	if loc == nil {
		return nil, nil
	}

	workModes, err := normalizeEnum(loc.WorkModes, enrich.WorkModeValues, ErrInvalidWorkMode)
	if err != nil {
		return nil, err
	}
	remote, err := normalizeGeoSet(loc.Remote)
	if err != nil {
		return nil, err
	}
	relocationRegions, err := normalizeEnum(loc.Relocation.Regions, enrich.RegionValues, ErrInvalidRegion)
	if err != nil {
		return nil, err
	}
	relocationCountries, err := normalizeCountries(loc.Relocation.Countries)
	if err != nil {
		return nil, err
	}
	relocationCities, err := normalizeCities(loc.Relocation.Cities)
	if err != nil {
		return nil, err
	}
	baseCountry, err := cleanCountryCode(loc.Base.Country)
	if err != nil {
		return nil, err
	}

	out := LocationPreferences{
		WorkModes: workModes,
		Remote:    remote,
		Base:      BaseLocation{Country: baseCountry, City: strings.TrimSpace(loc.Base.City)},
		Relocation: Relocation{
			Open:      loc.Relocation.Open,
			Regions:   relocationRegions,
			Countries: relocationCountries,
			Cities:    relocationCities,
		},
	}
	if out.isEmpty() {
		return nil, nil
	}
	return json.Marshal(out)
}

// normalizeGeoSet normalizes a reach's regions (controlled vocabulary) and countries (ISO).
func normalizeGeoSet(g GeoSet) (GeoSet, error) {
	regions, err := normalizeEnum(g.Regions, enrich.RegionValues, ErrInvalidRegion)
	if err != nil {
		return GeoSet{}, err
	}
	countries, err := normalizeCountries(g.Countries)
	if err != nil {
		return GeoSet{}, err
	}
	return GeoSet{Regions: regions, Countries: countries}, nil
}

// normalizeEnum lowercases, trims, and deduplicates values (first-seen order), rejecting any
// not in the controlled vocabulary (all lowercase) with invalidErr. Lowercasing keeps the
// dedup and membership check case-insensitive. Blanks are dropped; an empty result is valid.
func normalizeEnum(values, vocab []string, invalidErr error) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		v := strings.ToLower(strings.TrimSpace(raw))
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		if !slices.Contains(vocab, v) {
			return nil, invalidErr
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out, nil
}

// normalizeCountries cleans, deduplicates, and caps a country-code list. Blanks are dropped.
func normalizeCountries(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		code, err := cleanCountryCode(raw)
		if err != nil {
			return nil, err
		}
		if code == "" {
			continue
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	if len(out) > maxCountries {
		return nil, ErrTooManyCountries
	}
	return out, nil
}

// cleanCountryCode lowercases and trims a country code, returning "" for blank. A non-blank
// value that is not a well-formed ISO 3166-1 alpha-2 shape is rejected. We validate shape,
// not assignment: jobs derive their country facet from the curated location dictionary
// (a subset that omits real countries like jm/cu), so a membership check would reject
// countries a user legitimately picks — and a well-formed but unused code is harmless (it
// simply matches no jobs, like an unknown city or skill).
func cleanCountryCode(raw string) (string, error) {
	code := strings.ToLower(strings.TrimSpace(raw))
	if code == "" {
		return "", nil
	}
	if !isCountryCode(code) {
		return "", ErrInvalidCountry
	}
	return code, nil
}

// isCountryCode reports whether s is exactly two ASCII letters (an ISO 3166-1 alpha-2 shape).
func isCountryCode(s string) bool {
	if len(s) != 2 {
		return false
	}
	return s[0] >= 'a' && s[0] <= 'z' && s[1] >= 'a' && s[1] <= 'z'
}

// normalizeCities trims cities, drops blanks, and deduplicates case-insensitively (keeping
// the first-seen surface form), capping the list. Cities are free text (no dictionary).
func normalizeCities(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		city := strings.TrimSpace(raw)
		if city == "" {
			continue
		}
		key := strings.ToLower(city)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, city)
	}
	if len(out) > maxCities {
		return nil, ErrTooManyCities
	}
	return out, nil
}
