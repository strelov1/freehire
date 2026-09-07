package main

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
// counting the roles the two slugs share among their open postings, and the count is recorded
// beside it.
//
// Read the overlap as a SHARE of the SMALLER side, not as an absolute. `blend` / `blend-360`
// is the rejection that shape is for: 10 shared against blend's own 87 open postings — the
// smaller side, and barely a tenth of it — while the name alone said they were one employer.
// The absolute says nothing at all about a small duplicate: `micro1-ai` carries nine postings
// in total, so a floor of twenty was unreachable no matter how completely it duplicated the
// other side, and it would have rejected a pair the domain settles outright.
//
// Compare roles, not raw titles. The same posting syndicated twice is "General Counsel" on
// one board and "General Counsel - Remote" on the other, and an equality on the title scores
// that pair at ZERO — micro1's did, across 76 legal postings on one side and 9 on the other.
// Strip the trailing clause first, which is why the query below splits on " - ".
//
// That split is a cheap STAND-IN for jobhash.RoleKey, not the same normalisation: RoleKey also
// unwraps parentheses and KEEPS the words inside, so "Data Engineer (Semi Senior)" stays
// distinct from "Data Engineer" there and collapses onto it here. It is deliberately the
// looser of the two — this query is scoped to two slugs and read by a person deciding one
// entry, where over-counting shows up as a pair that does not survive the eye. Reach for
// RoleKey's exact fold only if that ever stops being true.
//
// A shared DOMAIN outranks both. If the surviving slug's postings link to the employer's own
// site and the retiring name reads as that domain (micro1's `req.micro1.ai` beside a board
// calling itself "micro1 AI"), that is the employer naming itself, and no title count is
// going to say it more clearly.
//
// # How to add one
//
//	WITH n AS (
//	  SELECT company_slug, lower(btrim(split_part(title, ' - ', 1))) AS role
//	  FROM jobs WHERE company_slug IN ('<canonical>', '<retiring>') AND closed_at IS NULL)
//	SELECT count(DISTINCT a.role) FROM n a JOIN n b ON a.role = b.role
//	WHERE a.company_slug = '<canonical>' AND b.company_slug = '<retiring>';
//
// Then run the worker without --apply and read the plan. The invariants a new entry must
// hold — canonical slug is a fixed point of the slug rule, and no entry points at a slug
// that is itself retiring — are enforced by TestCuratedAliasesAreWellFormed, not by review.
var curatedAliases = map[string]string{
	// Exadel runs two Greenhouse boards under different display names. 110 shared open titles
	// between this pair — about half of each side's catalogue. Found because both spellings
	// reached the same daily digest, where the per-company cap read them as two employers.
	"exadel-inc-website": "exadel",

	// Two aggregator spellings of the same employer, and the WEAKEST entries in this file —
	// recorded as such rather than dressed up. `exadelinc` is one whatjobs-pl posting and
	// `exadel-1` is two from aijobs; neither is a Greenhouse board of Exadel's, so neither
	// inherits the proof above, and a side of one or two postings can never reach an overlap
	// count in the first place.
	//
	// What is left is the name: "exadelinc" is "Exadel Inc" with the spaces gone, and
	// "Exadel 1" is an aggregator's numbering. That is the reasoning this file otherwise
	// calls unsafe — and it is admitted here only because the cost is bounded on both sides.
	// Three postings cannot bend a company page, and if the judgement is wrong the damage is
	// three postings filed under a neighbour, not a 301 pointing a real employer's URL
	// somewhere else. Do not read these two as licence to add a pair on the name alone where
	// the counts are large enough to be measured.
	"exadelinc": "exadel",
	"exadel-1":  "exadel",

	// micro1 posts from its own domain (req.micro1.ai, 336 open postings); the duplicate is an
	// Adzuna feed calling the employer "micro1 AI" — the domain read as a name — with 9 legal
	// postings against micro1's own 76. 3 shared roles out of those 9, and ZERO shared raw
	// titles, because the feed suffixes every one of them with " - Remote". The pair the
	// seeded absolute floor would have thrown away, and the reason the paragraph above now
	// talks about shares and domains.
	"micro1-ai": "micro1",

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
