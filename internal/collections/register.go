package collections

import (
	"strings"

	"github.com/strelov1/freehire/internal/normalize"
)

// RegisterSlug normalizes an organisation name as a public register publishes it
// into the canonical company slug our catalogue uses, stripping trailing legal
// forms on the way.
//
// The strip is why this exists at all: normalize.Slug("ACME ROBOTICS LIMITED") is
// "acme-robotics-limited", which never equals our "acme-robotics", so an exact
// match against a register would find almost nothing without it.
//
// It is normalize.CompanySlug, not a rule of its own, and that is load-bearing:
// Collection.Members looks this value up in a map keyed by the CATALOGUE's company
// slug, so the two must be one rule. When they disagreed, a register row whose form
// only one side stripped matched nothing — and nothing logged the miss, so the
// company simply never earned a credential it qualified for.
func RegisterSlug(name string) string {
	return normalize.CompanySlug(name)
}

// significantFields splits name on WHITESPACE and drops one trailing legal-form
// token. Only RequireCountry uses it, to judge how specific a register name is.
//
// It deliberately does not reuse normalize.CompanySlug's word breaks, which split on
// punctuation too: "T-Mobile Inc" must count as ONE significant token, or a
// single-token name would look specific enough to skip its headquarters check. The
// form test is shared (normalize.IsLegalForm) even though the tokenization is not —
// the list is what had to stop being duplicated, not the splitting.
func significantFields(name string) []string {
	fields := strings.Fields(name)
	if len(fields) < 2 || !normalize.IsLegalForm(fields[len(fields)-1]) {
		return fields
	}
	return fields[:len(fields)-1]
}

// ukWorkRoutes are the GOV.UK sponsorship routes that mean a licence useful to a
// candidate looking for a job. The register also lists Temporary Worker routes
// (seasonal, creative, charity, religious) and study routes, which sit alongside
// these in the same file and would badge a farm or a film crew as a sponsor for an
// engineering role. Matched as prefixes because the Global Business Mobility family
// publishes one route per sub-scheme.
var ukWorkRoutes = []string{
	"Skilled Worker",
	"Global Business Mobility",
	"Scale-up",
}

// RequireCountry builds the geography gate: the company must be demonstrably
// present in the register's country before it can hold that country's credential.
//
// The rule is stricter for a single-token name, and that asymmetry is the point. A
// two-token name ("Acme Robotics") is specific enough that sharing it with an
// unrelated business is unlikely, so open jobs in the country suffice. A one-token
// name ("Apple", "Spark", "Nova") is not: the UK register holds ~120k organisations
// across every industry, and a multinational with a local office would otherwise
// inherit a licence from a same-named local business. For those, the company's
// headquarters must be in the register's country — which Monzo and Revolut satisfy
// and Apple does not.
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
// beyond its ISO code. cmd/import-yc, the column's writer, runs values through
// location.Parse, which already emits ISO codes — but a retired one-time importer
// wrote a small number of rows with its upstream's `country` field verbatim, and
// those rows outlive the tool that wrote them. A bare code comparison happens to
// work for what's left today, but the gate must not depend on a third party's
// formatting, because the failure is silent: a company simply never earns a
// credential it qualifies for, and nothing logs it.
//
// Whole-value comparison only, never a substring: "New Great Britain Holdings" is a
// company name, not a country.
var countryAliases = map[string][]string{
	"GB": {"uk", "united kingdom", "great britain", "england", "scotland", "wales"},
	"NL": {"netherlands", "the netherlands", "holland"},
	"US": {"usa", "u.s.a.", "u.s.", "united states", "united states of america", "america"},
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
// identityKey names the Meta field that tells two organisations apart — "town" for
// the UK register, the KvK number for the Dutch one. It is needed because a shared
// name is not by itself a collision: the UK register lists one organisation once
// per route it holds, and those rows must survive for the route gate to read them.
// Rows sharing a name AND an identity are one body; rows sharing a name with
// different identities are a collision and all of them go.
//
// With an empty identityKey (or rows that omit it) every same-named row counts as
// one organisation. That is the permissive reading on purpose: firing the guard on
// a register that simply cannot disambiguate would delete its every duplicate,
// and the geography rules are the real defence in that case.
func DropAmbiguous(records []Record, identityKey string) []Record {
	// RegisterSlug does a non-trivial Unicode transliteration; computed once per record
	// here and reused below instead of recomputed in the filter loop, which matters on
	// the UK register's ~120k rows.
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
