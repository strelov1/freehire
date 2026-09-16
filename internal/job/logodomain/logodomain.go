// Package logodomain builds the name-to-domain map the logo proxy consults.
//
// Company logos are requested from logo.freehire.me by the company's NAME, taken verbatim
// from whichever adapter ingested the row. One employer arrives spelled several ways
// ("g2i", "G2i", "G2i Inc."), and the upstream that resolves a name answers a confident
// 200 with a DIFFERENT company's mark often enough to matter — g2i's name resolves to an
// unrelated leadership-training firm. A domain removes the ambiguity at the source, and
// the proxy has accepted one since it was written; nothing has ever supplied it.
//
// This package owns the two facts the proxy cannot have: which name spellings belong to
// one company (the stored company_slug, computed by normalize.CompanySlug at ingest) and
// what that company's website is (companies.company_info->>'website').
package logodomain

import (
	"net/url"
	"strings"
)

// NormalizeName collapses the spellings of one company onto one lookup key. It MUST stay
// equivalent to freehire-logo's store.Normalize, which is what the proxy hashes its cache
// under and what it will look this map up by: case-folded, runs of whitespace reduced to
// one space, ends trimmed.
//
// Punctuation is deliberately left alone, matching the proxy: "3M" and "3-M" may be
// different companies, and merging them would serve one company's logo for the other —
// worse than serving two keys. That is why "g2i" and "g2i inc." remain separate keys here
// and both get an entry, rather than one being folded into the other.
//
// The two copies of this rule live in two repositories and would drift silently, so the
// snapshot declares its normalization (see NormalizationID) and the proxy refuses a file
// whose value it does not implement.
func NormalizeName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

// Domain reduces a stored website value to the bare registrable domain the proxy's
// resolve.Query documents — "g2i.co", no scheme, no port, no path, no leading "www.".
//
// Returns "" for anything that is not plausibly a hostname. An unusable website is not an
// error: the company simply gets no entry and the proxy keeps resolving it by name, which
// is what happens today.
func Domain(website string) string {
	trimmed := strings.TrimSpace(website)
	if trimmed == "" {
		return ""
	}
	// url.Parse only fills Host when there is an authority, and stored values are both
	// "https://acme.com/x" and a bare "acme.com".
	if !strings.Contains(trimmed, "//") {
		trimmed = "//" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	// Hostname drops the port and any [] around an IPv6 literal; userinfo lands in
	// parsed.User and is discarded with it.
	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimSuffix(host, ".") // the DNS root label, which no upstream wants
	host = strings.TrimPrefix(host, "www.")
	// A registrable domain has a dot and no whitespace or path punctuation. This is
	// deliberately not a public-suffix check: the value came from a curated company
	// record, not from a user.
	if !strings.Contains(host, ".") || strings.ContainsAny(host, " \t/?#") {
		return ""
	}
	return host
}

// Spelling is one observed way a company's name is written in the catalogue.
type Spelling struct {
	Slug string
	Name string
}

// Build turns the company websites and the observed spellings into the published map.
//
// websites is keyed by company slug and holds the raw stored value; spellings may repeat
// a (slug, name) pair harmlessly. dropped counts the names two DIFFERENT companies share
// while disagreeing about the domain: those are omitted entirely, because publishing
// either one returns a confident logo belonging to the other company.
func Build(websites map[string]string, spellings []Spelling) (entries map[string]string, dropped int) {
	entries = make(map[string]string, len(websites))
	// conflicted remembers the keys already refused, so a name shared by three companies
	// is counted once rather than once per extra company.
	conflicted := make(map[string]bool)
	// domains caches the extraction per slug: a company with many spellings would
	// otherwise re-parse its own URL once per spelling, across ~412k rows.
	domains := make(map[string]string, len(websites))
	for _, s := range spellings {
		key := NormalizeName(s.Name)
		if key == "" || conflicted[key] {
			continue
		}
		domain, resolved := domains[s.Slug]
		if !resolved {
			domain = Domain(websites[s.Slug])
			domains[s.Slug] = domain
		}
		if domain == "" {
			continue
		}
		switch existing, seen := entries[key]; {
		case !seen:
			entries[key] = domain
		case existing != domain:
			delete(entries, key)
			conflicted[key] = true
			dropped++
		}
	}
	return entries, dropped
}
