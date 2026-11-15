package main

// curatedAliases records slugs a human has decided are one employer where
// normalize.CompanyKey cannot reach that conclusion on its own. Key is the slug retiring,
// value is the slug that survives.
//
// # Why a list and not a rule
//
// Every entry has the same shape (a base name with a numeric or parenthesised suffix), and
// that shape is NOT safe to automate. Most such pairs are board artefacts ("Pwc 1",
// "aecom2"), but some are separate companies:
//
//	Intel        vs  intel471   (Intel 471 is a separate company)
//	Four Seasons vs  Four Seasons Certified Home Health Agency
//	SpaceX       vs  SpaceXAI
//	Blend        vs  Blend360
//
// A rule that collapsed the shape would merge those, and a wrong merge is far worse than a
// missed one: the alias drives the 301, moving a public URL onto the wrong employer, and the
// re-key rewrites closed postings too. So each pair is named.
//
// # What earns an entry
//
// The name is the candidate and the postings are the proof: each pair was confirmed by
// counting the roles the two slugs share among their open postings, recorded beside it.
//
// Read the overlap as a share of the SMALLER side, not an absolute: `blend` / `blend-360`
// shared only 10 of blend's 87 open postings despite the name, while a small real duplicate
// like `micro1-ai` (nine postings in total) could never reach an absolute floor.
//
// Compare roles, not raw titles: a syndicated "General Counsel - Remote" never equals
// "General Counsel", so strip the trailing clause first (the query splits on " - ").
// That split is a cheap stand-in for jobhash.RoleKey and deliberately looser (RoleKey keeps
// "Data Engineer (Semi Senior)" distinct); it is read by a person deciding one entry, where
// over-counting shows. Use RoleKey's exact fold only if that stops being true.
//
// A shared DOMAIN outranks both: if the surviving slug's postings link to the employer's own
// site and the retiring name reads as that domain (`req.micro1.ai` beside "micro1 AI"), the
// employer is naming itself.
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
	// Exadel runs two Greenhouse boards under different display names; 110 shared open titles,
	// about half of each side.
	"exadel-inc-website": "exadel",

	// Two aggregator spellings (one and two postings), the WEAKEST entries here: with so few
	// postings no overlap count is possible, so they rest on the name alone ("Exadel Inc"
	// without spaces, an aggregator's numbering). Admitted only because the cost is bounded:
	// if wrong, three postings sit under a neighbour, not a 301 on a real employer. This is
	// no licence to add a pair on the name alone where counts can be measured.
	"exadelinc": "exadel",
	"exadel-1":  "exadel",

	// micro1 posts from its own domain (req.micro1.ai); the duplicate is an Adzuna feed calling
	// the employer "micro1 AI" (the domain read as a name). 3 of its 9 postings share a role,
	// and none a raw title because the feed suffixes " - Remote".
	"micro1-ai": "micro1",

	// The Himalayas company is MTR Legal Rechtsanwälte (mtrlegal.com, also the domain of the
	// employer assets on its Personio board). Keeps the canonical identity if a closed
	// syndicated copy reopens later.
	"mtr-legal-rechtsanwalte": "mtr-legal",

	// Krka's full register name, "KRKA, tovarna zdravil, d.d., Novo mesto" ("pharmaceutical
	// factory"), as public-employment feeds copy it. The seat and form are rules
	// (normalize.CloseSpacedForms) but the business description in the middle is a judgement
	// about words, so the pair is named. `krka-d-d-novo-mesto` is named too because the seat
	// rule leaves a one-word name its seat.
	"krka-tovarna-zdravil-d-d-novo-mesto": "krka",
	"krka-d-d-novo-mesto":                 "krka",
	// The seat rule's own key for the same spelling; covers postings ingested before the alias.
	"krka-tovarna-zdravil": "krka",

	// BRG's Workday tenant is named for its domain (thinkbrg.wd5.myworkdayjobs.com), so the
	// board stored the host label as the company. The domain settles it: the
	// berkeley-research-group company records homepage thinkbrg.com.
	"thinkbrg": "berkeley-research-group",

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
// group rather than the folded group its name would land in, or the curated members would
// have no one to merge into.
func curatedCanons(aliases map[string]string) map[string]bool {
	out := make(map[string]bool, len(aliases))
	for _, canon := range aliases {
		out[canon] = true
	}
	return out
}

// curatedGroupKey is the grouping key for a curated decision, prefixed with a rune
// normalize.CompanyKey never emits (it keeps only letters and digits) so it cannot collide
// with a folded group.
func curatedGroupKey(canon string) string { return "\x00curated\x1e" + canon }
