package collections

import (
	"strings"

	"github.com/strelov1/freehire/internal/normalize"
)

// legalSuffixes are the corporate-form words a public register appends to an
// organisation's legal name but that our catalogue's company names never carry.
// Matched as whole trailing tokens after normalization, so "bv" strips from
// "Booking B.V." but not from "BV Sports".
//
// Kept deliberately short: only the forms the UK Companies House, Dutch KvK, and
// USCIS registers actually emit. A speculative list (GmbH, S.A., Pty, …) would
// widen the blast radius of an over-strip for registers we do not read. "Co" is a
// deliberate omission even though USCIS data emits it: unlike the others, it
// collides with ordinary short words and abbreviations inside genuine company
// names, not just as a trailing legal-form token.
var legalSuffixes = map[string]struct{}{
	"ltd": {}, "limited": {}, "plc": {}, "llp": {}, "lp": {}, "cic": {}, "cio": {},
	"bv": {}, "nv": {},
	"inc": {}, "incorporated": {}, "corp": {}, "corporation": {}, "llc": {},
}

// RegisterSlug normalizes an organisation name as a public register publishes it
// into the canonical company slug our catalogue uses, stripping one trailing legal
// form on the way.
//
// The strip is why this exists at all: normalize.Slug("ACME ROBOTICS LIMITED") is
// "acme-robotics-limited", which never equals our "acme-robotics", so an exact
// match against a register would find almost nothing without it.
//
// Two deliberate limits. Only the *last* token is considered, so "Limited Brands"
// keeps its first word — a form word inside a name is part of the name. And a name
// consisting only of a legal form is returned unstripped rather than reduced to the
// empty slug, because an empty slug silently matches nothing while a bad register
// row is worth leaving visible.
func RegisterSlug(name string) string {
	fields := significantFields(name)
	if len(fields) == 0 {
		// Nothing to strip without erasing the name entirely.
		return normalize.Slug(name)
	}
	return normalize.Slug(strings.Join(fields, " "))
}

// significantFields splits name on whitespace and drops one trailing legal-form
// token, the same strip RegisterSlug applies before slugging. Shared so that
// token-counting (RequireCountry) agrees with what RegisterSlug considers the
// name's actual content, rather than re-deriving it from the slug — which has
// already collapsed internal punctuation ("T-Mobile" -> "t-mobile") into the same
// hyphen it uses as a field separator.
func significantFields(name string) []string {
	fields := strings.Fields(name)
	if len(fields) < 2 {
		return fields
	}
	if _, isSuffix := legalSuffixes[letters(fields[len(fields)-1])]; !isSuffix {
		return fields
	}
	return fields[:len(fields)-1]
}

// letters lowercases a token down to its ASCII letters, so the punctuated and bare
// spellings of a legal form ("B.V.", "BV", "Ltd.") compare as one. The check runs
// on the raw token rather than on the slug because normalize.Slug turns "B.V." into
// "b-v" — two tokens, matching no suffix — while leaving a name like "Foo.com"
// intact, which stripping dots globally would not.
func letters(token string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(token) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
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
