package main

import "github.com/strelov1/freehire/internal/dict/normalize"

// curatedAliases records slugs a human has decided are one employer where
// normalize.CompanyKey cannot reach that conclusion on its own. Key is the slug retiring,
// value is the slug that survives.
//
// # Why a list and not a rule
//
// Every entry below has the same shape — a base name with a numeric or parenthesised
// suffix — and that shape is NOT safe to automate. Measured across the catalogue's 447,148
// companies on 2026-09-06, folding "one folded name is a prefix of another" yields 32,880
// pairs, and 1,206 of them differ by a trailing number. Most are board artefacts ("Pwc 1",
// "Accenture 2", "aecom2"). Some are not:
//
//	Intel        (376 open jobs)  vs  intel471   — Intel 471 is a separate company
//	Four Seasons (824 open jobs)  vs  Four Seasons Certified Home Health Agency
//	SpaceX      (1009 open jobs)  vs  SpaceXAI
//	Blend        (141 open jobs)  vs  Blend360
//
// A rule that collapsed the shape would merge those four, and a wrong merge is far more
// expensive than a missed one: the alias is what the 301 reads, so it moves a public URL
// onto the wrong employer and the re-key rewrites closed postings too. Naming each pair is
// the honest cost of a judgement no name-shaped rule can make.
//
// # What earns an entry
//
// The name is the CANDIDATE and the postings are the PROOF. Every pair here was confirmed by
// counting distinct job titles the two slugs share among their open postings, and the count
// is recorded beside it. Twenty is the floor used when this list was seeded — below it the
// overlap stops distinguishing one employer from two that hire for the same roles. `blend` /
// `blend-360` scored 10 and was left out on exactly that evidence, which is also what the
// name alone would have got wrong.
//
// # How to add one
//
//	SELECT count(*) FROM (
//	  SELECT DISTINCT a.title FROM jobs a JOIN jobs b ON a.title = b.title
//	  WHERE a.company_slug = '<canonical>' AND b.company_slug = '<retiring>'
//	    AND a.closed_at IS NULL AND b.closed_at IS NULL) t;
//
// Then run the worker without --apply and read the plan. The invariants a new entry must
// hold — canonical slug is a fixed point of the slug rule, and no entry points at a slug
// that is itself retiring — are enforced by TestCuratedAliasesAreWellFormed, not by review.
var curatedAliases = map[string]string{
	// Exadel runs two Greenhouse boards under different display names, plus two stray
	// artefacts. 110 shared open titles between the first pair — about half of each side's
	// catalogue. Found because both spellings reached the same daily digest, where the
	// per-company cap read them as two employers.
	"exadel-inc-website": "exadel",
	"exadelinc":          "exadel",
	"exadel-1":           "exadel",

	// Numeric board artefacts, each confirmed by shared open titles (the count follows).
	"aecom2":                   "aecom",         // 1063
	"nagarro1":                 "nagarro",       // 323
	"sosi1":                    "sosi",          // 209
	"nbcuniversal3":            "nbc-universal", // 135
	"applusidiada1":            "applus-idiada", // 119
	"ubisoft2":                 "ubisoft",       // 104
	"ifs1":                     "ifs",           // 95
	"soprasteria1":             "sopra-steria",  // 88
	"netcompany1":              "netcompany",    // 85
	"versant3":                 "versant",       // 85
	"eversana1":                "eversana",      // 50
	"inetum2":                  "inetum",        // 47
	"bet3651":                  "bet365",        // 38
	"questronix-corporation-2": "questronix",    // 31
	"quadient1":                "quadient",      // 26
	"avaloq1":                  "avaloq",        // 21
	"flywire1":                 "flywire",       // 21
}

// curatedCanons is the set of slugs the list elects. A canonical slug joins its own curated
// group rather than the folded group its name would otherwise land in — without that it
// stays in the natural grouping and the curated members have no one to merge into.
func curatedCanons(aliases map[string]string) map[string]bool {
	out := make(map[string]bool, len(aliases))
	for _, canon := range aliases {
		out[canon] = true
	}
	return out
}

// curatedGroupKey is the grouping key for a curated decision. It is prefixed with a rune
// normalize.CompanyKey can never emit, so a curated group can never collide with a folded
// one — CompanyKey strips everything but letters and digits.
func curatedGroupKey(canon string) string { return "\x00curated\x1e" + canon }

// curatedCanonIsFixedPoint reports whether a canonical slug survives the slug rule unchanged.
//
// It has to. Every future posting from the surviving board derives its slug through
// CompanySlug, so a canon the rule would rewrite could never be reached by an ordinary
// crawl — the company would depend on an alias row forever to name itself. The same
// requirement is why electCanonical derives the canon from a name rather than reusing a
// stored slug.
func curatedCanonIsFixedPoint(canon string) bool {
	return normalize.CompanySlug(canon) == canon
}
