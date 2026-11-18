// Package logodomain builds the name-to-domain map the logo proxy consults.
//
// Logos are requested from logo.freehire.me by company name, and the upstream
// sometimes answers 200 with a different company's mark (g2i resolves to an unrelated
// firm). A domain removes the ambiguity, and the proxy accepts one.
//
// This package owns the two facts the proxy cannot have: which spellings belong to one
// company (the stored company_slug, from normalize.CompanySlug) and its website
// (companies.company_info->>'website').
package logodomain

import (
	"net/url"
	"strings"
)

// NormalizeName collapses the spellings of one company onto one lookup key. It MUST
// stay equivalent to freehire-logo's store.Normalize: case-folded, whitespace runs
// reduced to one space, ends trimmed.
//
// Punctuation is deliberately left alone, matching the proxy: "3M" and "3-M" may be
// different companies, and merging them would serve one's logo for the other. So "g2i"
// and "g2i inc." stay separate keys, each with an entry.
//
// The two copies live in two repositories, so the snapshot declares its normalization
// (see NormalizationID) and the proxy refuses a file whose value it does not implement.
func NormalizeName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

// Domain reduces a stored website value to the bare registrable domain the proxy's
// resolve.Query documents ("g2i.co": no scheme, port, path or leading "www.").
//
// Returns "" for anything that is not plausibly a hostname. An unusable website is not
// an error: the company gets no entry and the proxy keeps resolving it by name.
func Domain(website string) string {
	trimmed := strings.TrimSpace(website)
	if trimmed == "" {
		return ""
	}
	// url.Parse only fills Host when there is an authority; stored values are both
	// "https://acme.com/x" and a bare "acme.com".
	if !strings.Contains(trimmed, "//") {
		trimmed = "//" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	// Hostname drops the port and any [] around an IPv6 literal; userinfo is discarded.
	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimSuffix(host, ".") // the DNS root label, which no upstream wants
	host = strings.TrimPrefix(host, "www.")
	// A registrable domain has a dot and no whitespace or path punctuation. Not a
	// public-suffix check: the value came from a curated company record.
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
// websites is keyed by company slug and holds the raw stored value; repeated
// (slug, name) spellings are harmless. dropped counts names two different companies
// share while disagreeing about the domain: those are omitted entirely, since either
// one would return the other company's logo.
func Build(websites map[string]string, spellings []Spelling) (entries map[string]string, dropped int) {
	entries = make(map[string]string, len(websites))
	// conflicted remembers refused keys, so a name shared by three companies counts once.
	conflicted := make(map[string]bool)
	// domains caches the extraction per slug, so many spellings do not re-parse one URL.
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
