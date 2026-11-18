// Package credentials is the shared curated vocabulary that maps free-text certification and
// license names to canonical slugs. It is a leaf package with no freehire dependencies, so
// both sides of a comparison normalize to comparable slugs without an import cycle. Dict-only:
// an unrecognized credential resolves to nothing. The set is deliberately small and
// IT-first, plus a few global professional licenses.
//
// Two scanners, because the two consumers read different text:
//
//   - internal/job/jobfacts calls Scan on a job DESCRIPTION. That is prose, so an ambiguous
//     bare acronym may mean something else and is suppressed.
//   - internal/candidate/hardconstraint calls ScanLine on ONE line of a CV's certification
//     list, where a bare acronym IS the credential, so nothing is suppressed.
//
// The asymmetry runs the safe way: on the job side a spurious hit becomes a FALSE requirement
// that caps the score and is stated to the model as fact; on the CV side a missed hit becomes a
// false blocker and a spurious one merely fails to block.
package credentials

import "strings"

// entry pairs a canonical slug with the normalized aliases that resolve to it, plus the label a
// person reads.
//
// Aliases are in normalized form (see normalize): lowercase, apostrophes dropped, every other
// non-alphanumeric run collapsed to a space, '+' preserved so "security+" stays distinct.
//
// label exists because Reason/Action are sentences rendered verbatim on the job page and in the
// analysis prompt, and it cannot be recovered from a lowercased alias. See
// internal/dict/skilltag/labels.go for the same shape.
//
// proseAmbiguous marks a credential whose BARE acronym is also an ordinary word or another
// initialism, so Scan requires a multi-word alias for it. The suppression is on the acronym,
// never the credential: a posting that spells it out is still read.
type entry struct {
	slug           string
	label          string
	aliases        []string
	proseAmbiguous bool
}

// The three proseAmbiguous entries collide with ordinary phrases: "A+ players", "Salesforce CSM
// and CRM tooling", "our CPA firm".
//
// CISA is knowingly NOT marked: "CISA guidelines" names the US agency, but "CISSP, CISA and CISM
// required" is a real requirement pinned in internal/job/jobfacts, and a bare-acronym rule
// cannot tell them apart. jobfacts already narrows the text to the requirements section.
var table = []entry{
	// Cloud.
	{slug: "aws-solutions-architect", label: "AWS Certified Solutions Architect", aliases: []string{"aws certified solutions architect", "aws solutions architect", "aws sa", "saa c03"}},
	{slug: "aws-developer", label: "AWS Certified Developer", aliases: []string{"aws certified developer", "aws developer associate", "dva c02"}},
	{slug: "aws-sysops", label: "AWS Certified SysOps Administrator", aliases: []string{"aws certified sysops administrator", "aws sysops"}},
	{slug: "gcp-associate-cloud-engineer", label: "Google Associate Cloud Engineer", aliases: []string{"google associate cloud engineer", "gcp associate cloud engineer", "associate cloud engineer"}},
	{slug: "gcp-professional-cloud-architect", label: "Google Professional Cloud Architect", aliases: []string{"google professional cloud architect", "gcp professional cloud architect", "professional cloud architect"}},
	{slug: "azure-administrator", label: "Microsoft Azure Administrator", aliases: []string{"microsoft azure administrator", "azure administrator associate", "az 104"}},
	{slug: "azure-solutions-architect", label: "Azure Solutions Architect Expert", aliases: []string{"azure solutions architect expert", "az 305"}},
	// Kubernetes / IaC.
	{slug: "cka", label: "Certified Kubernetes Administrator (CKA)", aliases: []string{"certified kubernetes administrator", "cka"}},
	{slug: "ckad", label: "Certified Kubernetes Application Developer (CKAD)", aliases: []string{"certified kubernetes application developer", "ckad"}},
	{slug: "terraform-associate", label: "HashiCorp Certified: Terraform Associate", aliases: []string{"hashicorp certified terraform associate", "terraform associate"}},
	// Security.
	{slug: "comptia-security-plus", label: "CompTIA Security+", aliases: []string{"comptia security+", "security+", "sec+"}},
	{slug: "comptia-network-plus", label: "CompTIA Network+", aliases: []string{"comptia network+", "network+"}},
	{slug: "comptia-a-plus", label: "CompTIA A+", aliases: []string{"comptia a+", "a+"}, proseAmbiguous: true},
	{slug: "cissp", label: "CISSP", aliases: []string{"certified information systems security professional", "cissp"}},
	{slug: "cisa", label: "CISA (Certified Information Systems Auditor)", aliases: []string{"certified information systems auditor", "cisa"}},
	{slug: "cism", label: "CISM (Certified Information Security Manager)", aliases: []string{"certified information security manager", "cism"}},
	{slug: "ceh", label: "Certified Ethical Hacker (CEH)", aliases: []string{"certified ethical hacker", "ceh"}},
	{slug: "oscp", label: "OSCP (Offensive Security Certified Professional)", aliases: []string{"offensive security certified professional", "oscp"}},
	// Delivery / process.
	{slug: "pmp", label: "PMP (Project Management Professional)", aliases: []string{"project management professional", "pmp"}},
	{slug: "csm", label: "Certified ScrumMaster (CSM)", aliases: []string{"certified scrummaster", "certified scrum master", "csm"}, proseAmbiguous: true},
	{slug: "itil", label: "ITIL Foundation", aliases: []string{"itil foundation", "itil"}},
	// Finance.
	{slug: "cpa", label: "CPA (Certified Public Accountant)", aliases: []string{"certified public accountant", "cpa"}, proseAmbiguous: true},
	{slug: "cfa", label: "CFA (Chartered Financial Analyst)", aliases: []string{"chartered financial analyst", "cfa"}},
	// Global professional licenses (non-IT but common in postings).
	{slug: "pe-license", label: "Professional Engineer (PE) license", aliases: []string{"professional engineer license", "pe license"}},
	{slug: "cdl", label: "commercial driver's license (CDL)", aliases: []string{"commercial drivers license", "cdl"}},
}

// aliasIndex maps every normalized alias to its canonical slug; labels maps every slug to its
// sentence-ready name. Both are built once from table.
var (
	aliasIndex = buildAliasIndex()
	labels     = buildLabels()
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

func buildLabels() map[string]string {
	m := make(map[string]string, len(table))
	for _, e := range table {
		m[e.slug] = e.label
	}
	return m
}

// Canonical resolves a free-text credential name to its canonical slug. ok is false for
// anything outside the vocabulary; the caller emits nothing rather than guessing.
func Canonical(raw string) (string, bool) {
	slug, ok := aliasIndex[normalize(raw)]
	return slug, ok
}

// Label returns the sentence-ready name for a canonical slug, or the slug itself for an unknown
// one.
func Label(slug string) string {
	if l, ok := labels[slug]; ok {
		return l
	}
	return slug
}

// Scan returns the canonical slugs a job DESCRIPTION requires, in table order and deduped. It is
// the strict half: a proseAmbiguous credential is read only from a multi-word alias. See the
// package comment.
func Scan(text string) []string { return scan(text, false) }

// ScanLine returns the canonical slugs named in ONE line of a CV's certification list. It is
// the permissive half: a bare acronym is the credential and nothing is suppressed. Canonical
// matches the WHOLE string, and a CV entry ("CompTIA Security+ (2022)") almost never is one.
func ScanLine(text string) []string { return scan(text, true) }

// scan matches aliases as whole words on space-delimited boundaries of the normalized text
// (the leaf package does its own check to avoid a dependency), so "PMP" resolves but "pmp"
// inside another token does not. bareAcronyms admits the single-word aliases of a
// proseAmbiguous entry.
func scan(text string, bareAcronyms bool) []string {
	norm := normalize(text)
	if norm == "" {
		return nil
	}
	padded := " " + norm + " "
	var out []string
	for _, e := range table {
		// A prose-ambiguous entry is read only from an alias that SPELLS the credential out.
		// Multi-word is the test, so no second list is needed to drift from the first.
		speltOutOnly := e.proseAmbiguous && !bareAcronyms
		for _, a := range e.aliases {
			if speltOutOnly && !strings.Contains(a, " ") {
				continue
			}
			if strings.Contains(padded, " "+a+" ") {
				out = append(out, e.slug)
				break
			}
		}
	}
	return out
}

// normalize lowercases, drops apostrophes, and collapses every other non-alphanumeric run to a
// single space, preserving '+', so "CompTIA Security+" and "driver's license" match.
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
