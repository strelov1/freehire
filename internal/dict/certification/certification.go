// Package certification resolves a candidate-claimed certification name to a
// canonical slug from a curated, closed vocabulary — the same "never guess" shape as
// internal/dict/skilltag, deliberately much smaller: certifications are a bounded,
// well-known set (vendor, program, level), not an open-ended tech vocabulary. A name
// outside the dictionary resolves to nothing rather than passing through.
package certification

import (
	"strings"

	"github.com/strelov1/freehire/internal/platform/stringset"
)

// aliases maps a normalized (lowercase, separator-collapsed) certification name or
// shorthand to its canonical slug. Grown from what curators observe missing in real
// CVs, the same way skilltag's own alias table grows — not meant to be exhaustive on
// day one.
var aliases = map[string]string{
	// AWS.
	"aws certified solutions architect associate": "aws-solutions-architect",
	"aws certified solutions architect":           "aws-solutions-architect",
	"aws solutions architect associate":           "aws-solutions-architect",
	"aws solutions architect":                     "aws-solutions-architect",
	"aws certified developer associate":           "aws-developer",
	"aws certified developer":                     "aws-developer",
	"aws certified sysops administrator":          "aws-sysops-administrator",
	"aws certified cloud practitioner":            "aws-cloud-practitioner",

	// Azure.
	"azure fundamentals":                     "azure-fundamentals",
	"microsoft certified azure fundamentals": "azure-fundamentals",
	"azure administrator associate":          "azure-administrator",
	"azure solutions architect expert":       "azure-solutions-architect",

	// GCP.
	"google cloud certified professional cloud architect": "gcp-cloud-architect",
	"gcp professional cloud architect":                    "gcp-cloud-architect",
	"associate cloud engineer":                            "gcp-cloud-engineer",

	// Project management.
	"pmp":                             "pmp",
	"project management professional": "pmp",

	// Occupational safety. This is the first non-IT family in the vocabulary, and it is
	// here rather than in skilltag because these name credentials, not skills — a
	// distinction skilltag enforces itself: a scoped acronym there must resolve to a
	// canonical that already exists, since an acronym is another alias and never a new
	// facet value. Measured on 2,500 live HSE descriptions (2026-09-24): NEBOSH 11%,
	// CSP 11%, IOSH 5%, CIH 5%, CHMM 4%, HAZWOPER 1%.
	"nebosh":                     "nebosh",
	"nebosh general certificate": "nebosh",
	"nebosh international general certificate": "nebosh",
	"iosh":                                  "iosh",
	"iosh managing safely":                  "iosh",
	"csp":                                   "csp",
	"certified safety professional":         "csp",
	"cih":                                   "cih",
	"certified industrial hygienist":        "cih",
	"chmm":                                  "chmm",
	"certified hazardous materials manager": "chmm",
	"hazwoper":                              "hazwoper",

	// Kubernetes / CNCF.
	"cka":                                "cka",
	"certified kubernetes administrator": "cka",
	"ckad":                               "ckad",
	"certified kubernetes application developer": "ckad",

	// Security.
	"cissp": "cissp",
	"certified information systems security professional": "cissp",
	"comptia security+": "comptia-security-plus",
	"security+":         "comptia-security-plus",
	"comptia network+":  "comptia-network-plus",
	"network+":          "comptia-network-plus",

	// Scrum / agile.
	"csm":                       "scrum-master",
	"certified scrummaster":     "scrum-master",
	"psm":                       "scrum-master",
	"professional scrum master": "scrum-master",
}

// Canonicalize resolves caller-supplied certification names to canonical slugs,
// sorted and deduplicated, returning nil when none resolve. A token that does not
// match a known certification exactly is dropped, never guessed at and never passed
// through as a pseudo-canonical.
func Canonicalize(tokens []string) []string {
	out := make(map[string]struct{}, len(tokens))
	for _, tok := range tokens {
		if c, ok := aliases[normalize(tok)]; ok {
			out[c] = struct{}{}
		}
	}
	return stringset.Sorted(out)
}

// normalize lowercases and collapses whitespace/hyphen/underscore runs to a single
// space, so "AWS Certified Solutions Architect - Associate" and "aws certified
// solutions architect associate" share one lookup key.
func normalize(s string) string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return r == '-' || r == '_' || r == ' ' || r == '\t' || r == '\n'
	})
	return strings.Join(fields, " ")
}
