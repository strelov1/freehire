package collections

import (
	"strings"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// RegisterSlug normalizes an organisation name as a public register publishes it
// into the canonical company slug our catalogue uses, stripping trailing legal
// forms on the way.
//
// The strip matters: normalize.Slug("ACME ROBOTICS LIMITED") is "acme-robotics-limited",
// which never equals our "acme-robotics".
//
// It is normalize.CompanySlug on purpose: Collection.Members looks this value up in a map
// keyed by the CATALOGUE's company slug, so the two must be one rule. A disagreement silently
// loses the match, and nothing logs it.
func RegisterSlug(name string) string {
	return normalize.CompanySlug(name)
}

// significantFields splits name on WHITESPACE and drops the trailing legal-form
// tokens, as many as the company key drops: "Nova Sdn Bhd" and "Nova Pty Ltd" are one
// significant token, the same `nova` CompanySlug keys them at. Only RequireCountry uses
// it, to judge how specific a register name is.
//
// It deliberately does not reuse normalize.CompanySlug's word breaks, which split on
// punctuation too: "T-Mobile Inc" must count as ONE significant token, or a single-token name
// would look specific enough to skip its headquarters check. Only the legal-form test
// (normalize.IsLegalFormBefore) is shared.
func significantFields(name string) []string {
	fields := strings.Fields(normalize.CloseSpacedForms(name))
	next := ""
	for len(fields) > 1 && normalize.IsLegalFormBefore(fields[len(fields)-1], next) {
		next = fields[len(fields)-1]
		fields = fields[:len(fields)-1]
	}
	return fields
}

// ukWorkRoutes are the GOV.UK sponsorship routes that mean a licence useful to a
// candidate looking for a job. The register also lists Temporary Worker and study routes,
// which would badge a farm or a film crew as a sponsor for an engineering role. Matched as
// prefixes because Global Business Mobility publishes one route per sub-scheme.
var ukWorkRoutes = []string{
	"Skilled Worker",
	"Global Business Mobility",
	"Scale-up",
}

// RequireCountry builds the geography gate: the company must be demonstrably
// present in the register's country before it can hold that country's credential.
//
// The rule is stricter for a single-token name. A two-token name ("Acme Robotics") is
// unlikely to be shared with an unrelated business, so open jobs in the country suffice. A
// one-token name ("Apple", "Nova") is not: a multinational with a local office would inherit
// a licence from a same-named local business, so its headquarters must be in the register's
// country.
//
// An unknown headquarters is not a match. Absence of evidence is not evidence.
func RequireCountry(code string) func(Company, Record) bool {
	return func(co Company, r Record) bool {
		if !hasCountry(co.Countries, code) {
			return false
		}
		if len(significantFields(r.Name)) > 1 {
			return true // multi-token: the country facet carries it
		}
		return countryMatches(co.HQCountry, code)
	}
}

// countryAliases are the spellings a country may appear under in companies.hq_country,
// beyond its ISO code. The current writer emits ISO codes, but rows from a retired importer
// carry an upstream's verbatim `country`. The gate must not depend on third-party formatting:
// a miss is silent.
//
// Whole-value comparison only, never a substring: "New Great Britain Holdings" is a
// company name, not a country.
var countryAliases = map[string][]string{
	"GB": {"uk", "united kingdom", "great britain", "england", "scotland", "wales"},
	"NL": {"netherlands", "the netherlands", "holland", "nederland"},
	"US": {"usa", "u.s.a.", "u.s.", "united states", "united states of america", "america"},
	"CA": {"canada"},
	"IE": {"ireland", "republic of ireland"},
	"DK": {"denmark", "danmark"},
}

// withoutCountrySuffix returns a register slug with a trailing mention of the
// given country removed — "google-uk" for GB gives "google" — or "" when the slug
// does not end in one, or ends in nothing else.
//
// Only that ONE country, spelled as its ISO code or a countryAliases name: "Google (UK)
// Limited" in the UK register is the UK arm of Google, while "Stantec Ireland" in the UK file
// is not that evidence. Cities are not countries here ("JJ London Ltd" is not "jj").
//
// The stripped match still counts as a multi-token name in RequireCountry, since the country
// in the name is what makes it specific. The gate still demands open jobs there.
func withoutCountrySuffix(slug, code string) string {
	for _, name := range append([]string{strings.ToLower(code)}, countryAliases[strings.ToUpper(code)]...) {
		suffix := "-" + strings.Join(strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '.' }), "-")
		if parent, ok := strings.CutSuffix(slug, suffix); ok && parent != "" {
			return parent
		}
	}
	return ""
}

// countryMatches reports whether a stored country value denotes the given ISO code.
func countryMatches(value, code string) bool {
	if value == "" {
		return false
	}
	if strings.EqualFold(value, code) {
		return true
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(value), " "))
	for _, alias := range countryAliases[strings.ToUpper(code)] {
		if normalized == alias {
			return true
		}
	}
	return false
}

// RequireRoute builds the route gate: at least one of the organisation's register
// rows must sit on one of the given routes. Prefix-matched (see ukWorkRoutes).
func RequireRoute(prefixes ...string) func(Company, Record) bool {
	return func(_ Company, r Record) bool {
		route := r.Meta["route"]
		if route == "" {
			return false
		}
		for _, p := range prefixes {
			if strings.HasPrefix(route, p) {
				return true
			}
		}
		return false
	}
}

// AllOf composes gates: every one must admit. No gates admits everything, matching
// a nil Gate.
func AllOf(gates ...func(Company, Record) bool) func(Company, Record) bool {
	return func(co Company, r Record) bool {
		for _, g := range gates {
			if !g(co, r) {
				return false
			}
		}
		return true
	}
}

// DropAmbiguous removes every row whose normalized name is shared by more than one
// organisation, so a name that identifies nobody grants nothing.
//
// identityKey names the Meta field that tells two organisations apart ("town" for the UK
// register, the KvK number for the Dutch one). A shared name alone is not a collision: the UK
// register lists one organisation once per route, and those rows must survive for the route
// gate. Rows sharing a name AND an identity are one body; same name with different identities
// are a collision and all of them go.
//
// With an empty identityKey (or rows that omit it) every same-named row counts as one
// organisation, on purpose: otherwise a register that cannot disambiguate would lose every
// duplicate, and the geography rules are the real defence there.
func DropAmbiguous(records []Record, identityKey string) []Record {
	// RegisterSlug is costly (Unicode transliteration) and the registers are large, so
	// compute it once per record.
	slugs := make([]string, len(records))
	identities := make(map[string]map[string]struct{}, len(records))
	for i, r := range records {
		slug := RegisterSlug(r.Name)
		slugs[i] = slug
		if slug == "" {
			continue
		}
		if identities[slug] == nil {
			identities[slug] = make(map[string]struct{}, 1)
		}
		identities[slug][r.Meta[identityKey]] = struct{}{}
	}

	out := make([]Record, 0, len(records))
	for i, r := range records {
		if len(identities[slugs[i]]) > 1 {
			continue
		}
		out = append(out, r)
	}
	return out
}

// hasCountry reports whether a company's country facet holds the given ISO code,
// case-insensitively (the facet is upper-case by convention, but a gate must not
// depend on that).
func hasCountry(countries []string, code string) bool {
	for _, c := range countries {
		if countryMatches(c, code) {
			return true
		}
	}
	return false
}
