package classify

// coreCategoryTable5 is part 5 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable5 = []aliasEntry{
	{"комьюнити-менеджер", "marketing"},
	// Health, safety and environment. The block sat above the bare "manager"
	// fall-through (since retired), because that one line is what the profession was
	// losing to: measured on prod 2026-09-24, 1,454 live postings carrying an HSE acronym
	// resolved `management` on the strength of the word "Manager" alone, filing an EHS
	// Manager beside a Sales Manager. Another 4,515 resolved nothing at all.
	//
	// SOC codes EHS Managers separately from its 19-5011 specialists. This table
	// deliberately does not: `seniority` is its own facet here, so splitting one
	// profession across two category values would make a subscriber tick two boxes to
	// see one job market.
	//
	// First, the qualifiers that put the word "safety" in a title naming a DIFFERENT
	// profession. Without them the qualified "safety <role>" aliases below claim roughly
	// 2,300 live postings that are not this craft — food 616, public 843, patient 481,
	// product 163, campus 157 on 2026-09-24 — plus the tech-native families further down.
	// "fire safety" is deliberately absent: it is a genuine part of the HSE remit on an
	// industrial site, and 164 postings is too few to be worth splitting the
	// building-warden reading out of.
	//
	// Each ROUTES to the category that is true rather than carrying the blind sentinel,
	// because the rule at the top of this file says so and a review found the first
	// draft breaking it: a blanket `patient safety` sentinel blanked "Patient Safety
	// Registered Nurse" from `healthcare`, and `product safety` blanked "Product Safety
	// Engineer" from `industrial_engineering` — making a facet unreachable, which is the
	// harm this whole change exists to undo. The sentinel is left only where no category
	// is true.
	{"patient safety", "healthcare"},
	{"drug safety", "healthcare"}, // pharmacovigilance, the sibling of patient safety
	{"product safety", "industrial_engineering"},
	{"food safety", "industrial_engineering"},       // plant quality, where `quality engineer` already sits
	{"functional safety", "industrial_engineering"}, // ISO 26262, automotive and semiconductor
	{"life safety", "skilled_trades"},               // fire alarm and sprinkler trades
	{"ai safety", "ml_ai"},                          // alignment research, not a safety department
	// Protective services (SOC 33). They carried the blind sentinel until `public_safety`
	// existed, and now route to it. They stay paired with the role noun rather than bare,
	// because the bare qualifier would claim titles that answer for themselves: "Public
	// Safety Dispatcher" is filed by the emergency-dispatch entries in the public_safety
	// block, and "Public Safety Communications" the same way.
	//
	// The pairs are measured, not enumerated from imagination — every combination below
	// exists in the live catalogue, and the ones that do not are deliberately absent. A
	// first pass listed only the `officer` form, which left 27 postings of these same
	// professions filed under HSE.
	{"public safety officer", "public_safety"},     // 383
	{"campus safety officer", "public_safety"},     // 83
	{"school safety officer", "public_safety"},     // 8
	{"public safety manager", "public_safety"},     // 5
	{"public safety specialist", "public_safety"},  // 4
	{"public safety coordinator", "public_safety"}, // 3
	{"public safety director", "public_safety"},    // 3
	{"public safety technician", "public_safety"},  // 1
	{"public safety trainer", "public_safety"},
	{"public safety representative", "public_safety"},
	{"campus safety specialist", "public_safety"}, // 5
	{"campus safety supervisor", "public_safety"},
	{"campus safety coordinator", "public_safety"},
	{"campus safety manager", "public_safety"},
	{"campus safety representative", "public_safety"},
	{"campus safety intern", "public_safety"},
	{"pool safety officer", "public_safety"},
	// Trust & Safety is platform integrity at a consumer-tech employer — the most likely
	// "safety" collision on an IT job board.
	//
	// It is paired with the role noun for a sharper reason than the protective services
	// above. A bare `trust & safety` sentinel does not merely fail to improve things, it
	// BLANKS correctly-resolved technology roles: "Trust & Safety Product Manager" (7
	// live) resolves `product`, "Software Engineer, Trust & Safety" (4) resolves
	// `software_engineering`, and neither ever collided with this block, because the
	// matcher takes contiguous phrases and "safety product manager" does not contain
	// "safety manager". The first pass shipped that sentinel and blanked both.
	//
	// What the phrase names is a DOMAIN, not a craft — the live titles span product
	// managers, software engineers, analytics engineers, compliance, operations,
	// associates and directors — which is exactly why no single category is true for it
	// and why only the colliding pairs are listed. None of them names a category here.
	// Manager and director resolved `management` until that category was retired; a
	// director still reaches search through the `role_type` facet (internal/dict/roletype).
	{"trust and safety manager", categoryNone},
	{"trust & safety manager", categoryNone}, // 2 + 2 across the two spellings
	{"trust and safety director", categoryNone},
	{"trust & safety director", categoryNone}, // 2
	{"trust and safety specialist", categoryNone},
	{"trust & safety specialist", categoryNone}, // 14, the most common of these
	{"trust and safety lead", categoryNone},
	{"trust & safety lead", categoryNone}, // 4
	{"trust and safety engineer", categoryNone},
	{"trust & safety engineer", categoryNone}, // 3 + 1
	{"trust and safety coordinator", categoryNone},
	{"trust & safety coordinator", categoryNone},
	{"trust and safety representative", categoryNone},
	{"trust & safety representative", categoryNone},
	{"trust and safety intern", categoryNone},
	{"trust & safety intern", categoryNone},
	// Now the profession itself. The acronyms resolve bare: each is a coined initialism
	// with no English-word collision, unlike the two words below them.
	{"hse", "occupational_safety"},
	{"ehs", "occupational_safety"},
	{"hsse", "occupational_safety"},
	{"qhse", "occupational_safety"},
	{"hseq", "occupational_safety"},
	{"sheq", "occupational_safety"},
	{"shes", "occupational_safety"},
	// "SHE" is NOT here bare, and that is the one exclusion in this block with a
	// live-title reason rather than a dictionary one: it is an ordinary English word and
	// a pronoun that postings carry ("Software Engineer (she/her)"). Only the qualified
	// spellings, the same treatment bare "security" and bare "mobile" already get.
	{"she manager", "occupational_safety"},
	{"she officer", "occupational_safety"},
	{"she advisor", "occupational_safety"},
	{"she coordinator", "occupational_safety"},
	{"she specialist", "occupational_safety"},
	// The spelled-out forms. "environmental health and safety" and "occupational health
	// and safety" need no entry of their own — both contain "health and safety", and the
	// matcher is word-boundary based. The ampersand spelling does need one: it is a
	// different string, not a different boundary.
	{"health and safety", "occupational_safety"},
	{"health & safety", "occupational_safety"},
	// Russian. `internal/dict/classify`'s non-tech term list already carries "охрана
	// труда"/"охране труда", which is why these titles were being turned away at ingest
	// and hard-deleted rather than merely going uncategorised — see ConfirmedNonTech.
	// All three cases, because Russian titles inflect the phrase and the matcher does
	// not: "Специалист в области охраны труда" (112 live) is genitive, "Инженер по
	// охране труда" is dative, "Охрана труда" is nominative. The corpus probe found the
	// genitive; the hand-written list had only the other two.
	//
	// Note how narrow these are. The same corpus carries hundreds of postings for bare
	// "охрана" — "младший инспектор отдела охраны", "государственный инспектор по охране
	// леса" — which are security guards and forest rangers, not this profession. Only
	// the two-word phrase is admitted.
	{"охрана труда", "occupational_safety"},
	{"охране труда", "occupational_safety"},
	{"охраны труда", "occupational_safety"},
	// The titles SOC lists as reported for 19-5011, in the spellings prod carries, with
	// live counts. Bare "safety" is deliberately NOT an alias: it names patient safety
	// in healthcare, campus and public safety in protective services and food safety in
	// manufacturing quality, and these qualified forms cover the population without it.
	{"safety coordinator", "occupational_safety"}, // 283
	{"safety specialist", "occupational_safety"},  // 278
	{"safety officer", "occupational_safety"},     // 130
	{"safety technician", "occupational_safety"},  // 67
	{"safety supervisor", "occupational_safety"},  // 55
	{"safety advisor", "occupational_safety"},     // 42
	{"safety director", "occupational_safety"},    // 34
	{"safety manager", "occupational_safety"},     // 1,649 once the qualifiers above are excluded
	{"safety lead", "occupational_safety"},
	// Added after the corpus probe, which is the only thing that could have found them:
	// a test written from the list above can only confirm the list. Counts are live.
	{"safety administrator", "occupational_safety"}, // 24
	{"safety professional", "occupational_safety"},  // 23
	{"director of safety", "occupational_safety"},   // 20; the inverted form of "safety director"
	{"head of safety", "occupational_safety"},
	{"safety trainer", "occupational_safety"},        // 20
	{"safety intern", "occupational_safety"},         // 18
	{"safety representative", "occupational_safety"}, // 18
	{"safety inspector", "occupational_safety"},      // 14
	// The Singapore and site-officer spellings, which name the whole discipline between
	// the two words the qualified aliases above expect to be adjacent.
	{"workplace safety and health", "occupational_safety"}, // 25 across its spellings
	{"workplace safety & health", "occupational_safety"},
	{"safety and health officer", "occupational_safety"}, // 10, "Site Safety and Health Officer"
	{"safety & health officer", "occupational_safety"},
	{"industrial hygienist", "occupational_safety"}, // named in SOC's reported-title list
	{"industrial hygiene", "occupational_safety"},
	{"risk control consultant", "occupational_safety"},
	// Bare "risk" is NOT an alias. It matched 41,468 live postings and most of them are
	// finance — "Risk Analyst", "Credit Risk Manager" — which is a different profession
	// that happens to share a word, and which already resolves correctly elsewhere.
	{"hse risk", "occupational_safety"},
	{"safety risk", "occupational_safety"},
	// There is deliberately NO bare "manager"/"менеджер" and NO bare "analyst"/"аналитик"
	// here. Both used to be fall-throughs for a title with no recognised function, and
	// both filed it on the wrong axis.
	//
	// "manager" fell through to a `management` category, which held about 6% of the open
	// catalogue and told a jobseeker nothing about the work: an Assistant Manager at a
	// burger chain, a Case Manager, a Tax Manager and a Plant Manager in one facet. The
	// category is retired. A manager title now resolves the CRAFT it names — the
	// "restaurant/store/plant/tax/... manager" spellings are filed in the managers block
	// at the end of this table — and whether it manages PEOPLE is the `role_type` facet's
	// question (internal/dict/roletype), which reaches the generic General Manager and
	// Assistant Manager titles across every craft. Search keeps a posting either facet
	// reaches (search.CategoryUnresolved).
	//
	// "analyst" fell through to `data_analytics`, which is technical: an unrecognised
	// analyst was thereby DECLARED a technical data role.
	//
	// Measured on the open catalogue on 2026-09-06, that facet held 102,222 postings and
	// 77,086 of them (75%) carried no data, analytics or BI word at all. 13,748 were
	// Board Certified Behavior Analysts — school ABA therapists — and the rest
	// investment, purchasing, risk, pricing, research, policy and tax analysts. All
	// 102,222 had is_tech = true, so all of them were eligible for the enrichment budget,
	// and that technical category shielded every one from the non-technical dictionary.
	//
	// The technical families it was hiding are named individually instead, each in its
	// own block above and each resolving MORE precisely than this ever did: test,
	// threat/cyber, network, technical, application(s), it, technology, software,
	// infrastructure and cloud analysts, ~3.4k live postings between them. An analyst
	// the dictionary cannot place now resolves to nothing, which is what the rest of
	// this package does with a title it cannot read.
	// software_engineering: the generic catch-all for a title classify.IsTech's
	// techTitleTerms already confirms as software/IT work but that names no
	// sub-discipline — "Software Engineer" and "Java Developer" do not say
	// backend vs frontend vs fullstack, and this package never guesses. Every
	// entry below mirrors a techTitleTerms member that has no more specific
	// categoryTable entry above it (cross-checked against tech.go so the two
	// lists cannot silently drift). Kept at the very bottom, second-to-last
	// before the 1C fallback, so any more specific alias anywhere above always
	// wins first — these only fire once nothing else has.
	//
	// Deliberately EXCLUDED: bare "programmer" (techTitleTerms has it as an
	// "unambiguous" single word for is_tech, but prod titles include "CNC
	// Programmer" — a machining role, not software — so a category entry here
	// would mislabel it; is_tech's false positive on it is a separate, smaller
	// bug left alone).
	//
	// The base phrases ("software engineer", "software developer", "software
	// development engineer") deliberately carry every qualified variant with
	// them by substring, the same convention the "design engineer" bare form
	// uses above: "Senior Software Engineer, Platform" and "AI Software
	// Engineer, Internal Tools" resolve without their own entry.
	{"software engineer", "software_engineering"},
	{"software development engineer", "software_engineering"},
	{"software developer", "software_engineering"},
	{"web developer", "software_engineering"},
	{"web engineer", "software_engineering"},
	{"app developer", "software_engineering"},
	{"application developer", "software_engineering"},
	{"game developer", "software_engineering"},
	{"go engineer", "software_engineering"},
	{"golang engineer", "software_engineering"},
	{"go developer", "software_engineering"},
	{"golang developer", "software_engineering"},
	{"python developer", "software_engineering"},
	{"java developer", "software_engineering"},
	{"javascript developer", "software_engineering"},
	{"typescript developer", "software_engineering"},
	{".net developer", "software_engineering"},
	{"dotnet developer", "software_engineering"},
	{"php developer", "software_engineering"},
}
