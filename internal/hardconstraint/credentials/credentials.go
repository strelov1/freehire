// Package credentials is the shared curated vocabulary that maps free-text
// certification/license names to canonical slugs. It is a leaf package with no
// freehire dependencies so the enrichment extractor, the résumé extractor, and
// the hard-constraint evaluator can all normalize to comparable slugs without an
// import cycle. Dict-only discipline: an unrecognized credential resolves to
// nothing (never guessed).
//
// The set is IT-first (cloud, Kubernetes, security, PM) plus a few genuinely
// global professional licenses; it is deliberately small and curated, not a
// dump of every credential in existence.
package credentials

import "strings"

// entry pairs a canonical slug with the normalized aliases that resolve to it.
// Aliases are already in normalized form (see normalize): lowercase, apostrophes
// dropped, and every other non-alphanumeric run collapsed to a single space, with
// '+' preserved so "security+" stays distinct.
type entry struct {
	slug    string
	aliases []string
}

var table = []entry{
	// Cloud.
	{"aws-solutions-architect", []string{"aws certified solutions architect", "aws solutions architect", "aws sa", "saa c03"}},
	{"aws-developer", []string{"aws certified developer", "aws developer associate", "dva c02"}},
	{"aws-sysops", []string{"aws certified sysops administrator", "aws sysops"}},
	{"gcp-associate-cloud-engineer", []string{"google associate cloud engineer", "gcp associate cloud engineer", "associate cloud engineer"}},
	{"gcp-professional-cloud-architect", []string{"google professional cloud architect", "gcp professional cloud architect", "professional cloud architect"}},
	{"azure-administrator", []string{"microsoft azure administrator", "azure administrator associate", "az 104"}},
	{"azure-solutions-architect", []string{"azure solutions architect expert", "az 305"}},
	// Kubernetes / IaC.
	{"cka", []string{"certified kubernetes administrator", "cka"}},
	{"ckad", []string{"certified kubernetes application developer", "ckad"}},
	{"terraform-associate", []string{"hashicorp certified terraform associate", "terraform associate"}},
	// Security.
	{"comptia-security-plus", []string{"comptia security+", "security+", "sec+"}},
	{"comptia-network-plus", []string{"comptia network+", "network+"}},
	{"comptia-a-plus", []string{"comptia a+", "a+"}},
	{"cissp", []string{"certified information systems security professional", "cissp"}},
	{"cisa", []string{"certified information systems auditor", "cisa"}},
	{"cism", []string{"certified information security manager", "cism"}},
	{"ceh", []string{"certified ethical hacker", "ceh"}},
	{"oscp", []string{"offensive security certified professional", "oscp"}},
	// Delivery / process.
	{"pmp", []string{"project management professional", "pmp"}},
	{"csm", []string{"certified scrummaster", "certified scrum master", "csm"}},
	{"itil", []string{"itil foundation", "itil"}},
	// Finance.
	{"cpa", []string{"certified public accountant", "cpa"}},
	{"cfa", []string{"chartered financial analyst", "cfa"}},
	// Global professional licenses (non-IT but common in postings).
	{"pe-license", []string{"professional engineer license", "pe license"}},
	{"cdl", []string{"commercial drivers license", "cdl"}},
}

// aliasIndex maps every normalized alias to its canonical slug. slugSet is the
// controlled set for IsCanonical. Both are built once from table.
var (
	aliasIndex = buildAliasIndex()
	slugSet    = buildSlugSet()
)

func buildAliasIndex() map[string]string {
	m := make(map[string]string)
	for _, e := range table {
		for _, a := range e.aliases {
			m[a] = e.slug
		}
	}
	return m
}

func buildSlugSet() map[string]bool {
	m := make(map[string]bool, len(table))
	for _, e := range table {
		m[e.slug] = true
	}
	return m
}

// Canonical resolves a free-text credential name to its canonical slug. ok is
// false for anything outside the curated vocabulary — the caller emits nothing
// rather than guessing.
func Canonical(raw string) (string, bool) {
	slug, ok := aliasIndex[normalize(raw)]
	return slug, ok
}

// IsCanonical reports whether slug is a member of the controlled set. Used by the
// enrichment/résumé sanitize gates to drop out-of-vocabulary slugs.
func IsCanonical(slug string) bool {
	return slugSet[slug]
}

// Scan returns the canonical slugs whose aliases appear as whole words in text,
// in table order and deduped. It normalizes text once and matches on
// space-delimited word boundaries (the leaf package does its own boundary check to
// avoid a dependency), so "PMP" in prose resolves but "pmp" inside another token
// does not. Used to derive a job's required certifications from its description.
func Scan(text string) []string {
	norm := normalize(text)
	if norm == "" {
		return nil
	}
	padded := " " + norm + " "
	var out []string
	for _, e := range table {
		for _, a := range e.aliases {
			if strings.Contains(padded, " "+a+" ") {
				out = append(out, e.slug)
				break
			}
		}
	}
	return out
}

// normalize lowercases, drops apostrophes, and collapses every other
// non-alphanumeric run to a single space while preserving '+', so "CompTIA
// Security+" and "commercial driver's license" match their aliases.
func normalize(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	prevSpace := true // trims leading space
	for _, r := range strings.ToLower(raw) {
		switch {
		case r == '\'' || r == '’':
			// drop apostrophes so "driver's" == "drivers"
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+':
			b.WriteRune(r)
			prevSpace = false
		case !prevSpace:
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimRight(b.String(), " ")
}
