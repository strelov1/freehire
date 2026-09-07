// Package credentials is the shared curated vocabulary that maps free-text
// certification/license names to canonical slugs. It is a leaf package with no
// freehire dependencies, so callers on either side of a comparison normalize to
// comparable slugs without an import cycle. Dict-only discipline: an unrecognized
// credential resolves to nothing (never guessed).
//
// The set is IT-first (cloud, Kubernetes, security, PM) plus a few genuinely
// global professional licenses; it is deliberately small and curated, not a
// dump of every credential in existence.
//
// It has two consumers, and they read two different kinds of text — which is why there
// are two scanners rather than one:
//
//   - internal/job/jobfacts calls Scan on a job DESCRIPTION. That is prose, so a bare
//     two- or three-letter acronym is a word the text may be using for something else
//     entirely, and the ambiguous ones are suppressed.
//   - internal/candidate/hardconstraint calls ScanLine on ONE line of a résumé's
//     certification list. That is a deliberate field where a bare acronym IS the
//     credential, so nothing is suppressed.
//
// The asymmetry is deliberate and it runs the safe way. On the job side a spurious hit
// becomes a FALSE requirement, which caps a candidate's score and is stated to the model
// as established fact; on the résumé side a missed hit becomes a false blocker and a
// spurious one merely fails to block.
package credentials

import "strings"

// entry pairs a canonical slug with the normalized aliases that resolve to it, plus the
// label a person reads.
//
// Aliases are already in normalized form (see normalize): lowercase, apostrophes
// dropped, and every other non-alphanumeric run collapsed to a single space, with
// '+' preserved so "security+" stays distinct.
//
// label exists because slug is an identifier and Reason/Action are sentences. They are
// rendered verbatim on the job page and go verbatim into all three stages of the analysis
// prompt, so without this a candidate read "Requires the
// gcp-professional-cloud-architect certification." The label cannot be recovered from an
// alias — aliases are folded to lower case. internal/dict/skilltag/labels.go is the same
// shape for the same reason.
//
// proseAmbiguous marks a credential whose BARE acronym is also an ordinary word or a
// different initialism, so Scan requires a multi-word alias for it. The suppression is on
// the acronym, never on the credential: a posting that spells the certification out is
// still read.
type entry struct {
	slug           string
	label          string
	aliases        []string
	proseAmbiguous bool
}

// The three proseAmbiguous entries were each measured against a real posting: "A+
// players", "Salesforce CSM and CRM tooling", and "our CPA firm". All three collide with an
// ordinary phrase, and none is pinned by an existing test.
//
// A fourth measured collision is NOT marked: "CISA guidelines" names the US agency, which
// security postings mention constantly — but "CISSP, CISA and CISM required" is a real
// requirement pinned in internal/job/jobfacts, and a bare-acronym rule cannot tell the two
// apart. Separating them needs a vocabulary of credential words near the acronym, which is
// its own decision with its own false readings; until then CISA is knowingly left reading
// from prose, and jobfacts already narrows the text to the requirements section.
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
	// "A+ players" is the phrase this one kept reading as a certification.
	{slug: "comptia-a-plus", label: "CompTIA A+", aliases: []string{"comptia a+", "a+"}, proseAmbiguous: true},
	{slug: "cissp", label: "CISSP", aliases: []string{"certified information systems security professional", "cissp"}},
	// CISA is also the US agency, named in most security postings.
	{slug: "cisa", label: "CISA (Certified Information Systems Auditor)", aliases: []string{"certified information systems auditor", "cisa"}},
	{slug: "cism", label: "CISM (Certified Information Security Manager)", aliases: []string{"certified information security manager", "cism"}},
	{slug: "ceh", label: "Certified Ethical Hacker (CEH)", aliases: []string{"certified ethical hacker", "ceh"}},
	{slug: "oscp", label: "OSCP (Offensive Security Certified Professional)", aliases: []string{"offensive security certified professional", "oscp"}},
	// Delivery / process.
	{slug: "pmp", label: "PMP (Project Management Professional)", aliases: []string{"project management professional", "pmp"}},
	// CSM is also Customer Success Manager, a role these postings hire for.
	{slug: "csm", label: "Certified ScrumMaster (CSM)", aliases: []string{"certified scrummaster", "certified scrum master", "csm"}, proseAmbiguous: true},
	{slug: "itil", label: "ITIL Foundation", aliases: []string{"itil foundation", "itil"}},
	// Finance.
	// CPA is also cost-per-acquisition, and "our CPA firm" is not a requirement.
	{slug: "cpa", label: "CPA (Certified Public Accountant)", aliases: []string{"certified public accountant", "cpa"}, proseAmbiguous: true},
	{slug: "cfa", label: "CFA (Chartered Financial Analyst)", aliases: []string{"chartered financial analyst", "cfa"}},
	// Global professional licenses (non-IT but common in postings).
	{slug: "pe-license", label: "Professional Engineer (PE) license", aliases: []string{"professional engineer license", "pe license"}},
	{slug: "cdl", label: "commercial driver's license (CDL)", aliases: []string{"commercial drivers license", "cdl"}},
}

// aliasIndex maps every normalized alias to its canonical slug; labels maps every slug to
// the sentence-ready name. Both are built once from table.
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

// Canonical resolves a free-text credential name to its canonical slug. ok is
// false for anything outside the curated vocabulary — the caller emits nothing
// rather than guessing.
func Canonical(raw string) (string, bool) {
	slug, ok := aliasIndex[normalize(raw)]
	return slug, ok
}

// Label returns the sentence-ready name for a canonical slug, or the slug itself for one
// this table does not carry. A caller interpolating an unknown slug gets a poor sentence
// rather than a broken one.
func Label(slug string) string {
	if l, ok := labels[slug]; ok {
		return l
	}
	return slug
}

// Scan returns the canonical slugs a job DESCRIPTION requires, in table order and deduped.
//
// It is the strict half: a proseAmbiguous credential is only read from a multi-word alias,
// because its bare acronym is a word the description may be using for something else. See
// the package comment for why the two directions differ.
func Scan(text string) []string { return scan(text, false) }

// ScanLine returns the canonical slugs named in ONE line of a résumé's certification list.
//
// It is the permissive half: the line is a deliberate field, so a bare acronym is the
// credential and nothing is suppressed. It exists because Canonical matches the WHOLE
// string and a résumé entry almost never is one — "CompTIA Security+ (2022)", "Certified
// Kubernetes Administrator (CKA)" and "AWS Certified Solutions Architect – Associate" all
// returned ("", false), which credited the candidate with none of the three and produced a
// certification blocker for one they hold. degreeMatch already solved this next door.
func ScanLine(text string) []string { return scan(text, true) }

// scan matches aliases as whole words. It normalizes text once and matches on
// space-delimited word boundaries (the leaf package does its own boundary check to avoid a
// dependency), so "PMP" resolves but "pmp" inside another token does not.
//
// bareAcronyms admits the single-word aliases of a proseAmbiguous entry. A multi-word
// alias is never ambiguous — nobody writes "certified information systems auditor" by
// accident — so the distinction needs no second list to drift from the first.
func scan(text string, bareAcronyms bool) []string {
	norm := normalize(text)
	if norm == "" {
		return nil
	}
	padded := " " + norm + " "
	var out []string
	for _, e := range table {
		for _, a := range e.aliases {
			if e.proseAmbiguous && !bareAcronyms && !strings.Contains(a, " ") {
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
