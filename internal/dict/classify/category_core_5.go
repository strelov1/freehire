package classify

// coreCategoryTable5 is part 5 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable5 = []aliasEntry{
	{"комьюнити-менеджер", "marketing"},
	// Health, safety and environment. The bare "manager" fall-through (since retired) used to claim these titles and file an
	// EHS Manager beside a Sales Manager.
	// SOC codes EHS Managers separately from its 19-5011 specialists. This table does not: `seniority` is its own facet, so
	// splitting one profession across two category values would make a subscriber tick two boxes for one job market.
	// First, the qualifiers that put "safety" in a title naming a DIFFERENT profession (food, public, patient, product,
	// campus, plus the tech-native families further down). Without them the qualified "safety <role>" aliases below
	// would claim them. "fire safety" is absent on purpose: it is a genuine part of the HSE remit on an industrial site.
	// Each ROUTES to the category that is true rather than carrying the blind sentinel, per the rule at the top of this
	// file: a blanket `patient safety` sentinel would blank "Patient Safety Registered Nurse" from `healthcare`, and
	// `product safety` would blank "Product Safety Engineer" from `industrial_engineering`. The sentinel is left only
	// where no category is true.
	{"patient safety", "healthcare"},
	{"drug safety", "healthcare"}, // pharmacovigilance, the sibling of patient safety
	{"product safety", "industrial_engineering"},
	{"food safety", "industrial_engineering"},       // plant quality, where `quality engineer` already sits
	{"functional safety", "industrial_engineering"}, // ISO 26262, automotive and semiconductor
	{"life safety", "skilled_trades"},               // fire alarm and sprinkler trades
	{"ai safety", "ml_ai"},                          // alignment research, not a safety department
	// Protective services (SOC 33), routed to `public_safety`. They stay paired with the role noun rather than bare,
	// because the bare qualifier would claim titles that answer for themselves: "Public Safety Dispatcher" and "Public
	// Safety Communications" are filed by the emergency-dispatch entries in the public_safety block. Only pairs that
	// exist in the live catalogue are listed; the `officer` form alone would leave the other professions filed under HSE.
	{"public safety officer", "public_safety"},
	{"campus safety officer", "public_safety"},
	{"school safety officer", "public_safety"},
	{"public safety manager", "public_safety"},
	{"public safety specialist", "public_safety"},
	{"public safety coordinator", "public_safety"},
	{"public safety director", "public_safety"},
	{"public safety technician", "public_safety"},
	{"public safety trainer", "public_safety"},
	{"public safety representative", "public_safety"},
	{"campus safety specialist", "public_safety"},
	{"campus safety supervisor", "public_safety"},
	{"campus safety coordinator", "public_safety"},
	{"campus safety manager", "public_safety"},
	{"campus safety representative", "public_safety"},
	{"campus safety intern", "public_safety"},
	{"pool safety officer", "public_safety"},
	// Trust & Safety is platform integrity at a consumer-tech employer, the likeliest "safety" collision on an IT job
	// board. It is paired with the role noun because a bare `trust & safety` sentinel would BLANK correctly resolved
	// technology roles: "Trust & Safety Product Manager" resolves `product` and "Software Engineer, Trust & Safety"
	// resolves `software_engineering`, and neither collides with this block ("safety product manager" does not contain
	// "safety manager"; the matcher takes contiguous phrases).
	// The phrase names a DOMAIN, not a craft, so no single category is true for it and only the colliding pairs are
	// listed. None names a category here; a director still reaches search through the `role_type` facet
	// (internal/dict/roletype).
	{"trust and safety manager", categoryNone},
	{"trust & safety manager", categoryNone},
	{"trust and safety director", categoryNone},
	{"trust & safety director", categoryNone},
	{"trust and safety specialist", categoryNone},
	{"trust & safety specialist", categoryNone},
	{"trust and safety lead", categoryNone},
	{"trust & safety lead", categoryNone},
	{"trust and safety engineer", categoryNone},
	{"trust & safety engineer", categoryNone},
	{"trust and safety coordinator", categoryNone},
	{"trust & safety coordinator", categoryNone},
	{"trust and safety representative", categoryNone},
	{"trust & safety representative", categoryNone},
	{"trust and safety intern", categoryNone},
	{"trust & safety intern", categoryNone},
	// Now the profession itself. The acronyms resolve bare: each is a coined initialism with no English-word collision,
	// unlike the two words below them.
	{"hse", "occupational_safety"},
	{"ehs", "occupational_safety"},
	{"hsse", "occupational_safety"},
	{"qhse", "occupational_safety"},
	{"hseq", "occupational_safety"},
	{"sheq", "occupational_safety"},
	{"shes", "occupational_safety"},
	// "SHE" is NOT here bare: it is an ordinary English word and a pronoun that postings carry ("Software Engineer
	// (she/her)"). Only qualified spellings, the same treatment bare "security" and bare "mobile" get.
	{"she manager", "occupational_safety"},
	{"she officer", "occupational_safety"},
	{"she advisor", "occupational_safety"},
	{"she coordinator", "occupational_safety"},
	{"she specialist", "occupational_safety"},
	// The spelled-out forms. "environmental health and safety" and "occupational health and safety" need no entry: both
	// contain "health and safety". The ampersand spelling does need one: it is a different string, not a different boundary.
	{"health and safety", "occupational_safety"},
	{"health & safety", "occupational_safety"},
	// Russian. The non-tech term list already carries "охрана труда"/"охране труда". Russian titles
	// inflect the phrase and the matcher does not, so all three cases are listed: genitive ("Специалист в области охраны
	// труда"), dative ("Инженер по охране труда"), nominative ("Охрана труда"). They are narrow on purpose: bare "охрана"
	// names security guards and forest rangers ("государственный инспектор по охране леса"), so only the two-word
	// phrase is admitted.
	{"охрана труда", "occupational_safety"},
	{"охране труда", "occupational_safety"},
	{"охраны труда", "occupational_safety"},
	// The titles SOC lists as reported for 19-5011, in the spellings prod carries. Bare "safety" is deliberately NOT an
	// alias: it names patient safety in healthcare, campus and public safety in protective services and food safety in
	// manufacturing quality; these qualified forms cover the population without it.
	{"safety coordinator", "occupational_safety"},
	{"safety specialist", "occupational_safety"},
	{"safety officer", "occupational_safety"},
	{"safety technician", "occupational_safety"},
	{"safety supervisor", "occupational_safety"},
	{"safety advisor", "occupational_safety"},
	{"safety director", "occupational_safety"},
	{"safety manager", "occupational_safety"},
	{"safety lead", "occupational_safety"},
	// Spellings found by the corpus probe, which a test written from the list above could only confirm.
	{"safety administrator", "occupational_safety"},
	{"safety professional", "occupational_safety"},
	{"director of safety", "occupational_safety"}, // the inverted form of "safety director"
	{"head of safety", "occupational_safety"},
	{"safety trainer", "occupational_safety"},
	{"safety intern", "occupational_safety"},
	{"safety representative", "occupational_safety"},
	{"safety inspector", "occupational_safety"},
	// The Singapore and site-officer spellings, which name the whole discipline between the two words the qualified
	// aliases above expect to be adjacent.
	{"workplace safety and health", "occupational_safety"},
	{"workplace safety & health", "occupational_safety"},
	{"safety and health officer", "occupational_safety"}, // "Site Safety and Health Officer"
	{"safety & health officer", "occupational_safety"},
	{"industrial hygienist", "occupational_safety"}, // named in SOC's reported-title list
	{"industrial hygiene", "occupational_safety"},
	{"risk control consultant", "occupational_safety"},
	// Bare "risk" is NOT an alias: most titles carrying it are finance ("Risk Analyst", "Credit Risk Manager"), a
	// different profession that already resolves elsewhere.
	{"hse risk", "occupational_safety"},
	{"safety risk", "occupational_safety"},
	// There is deliberately NO bare "manager"/"менеджер" and NO bare "analyst"/"аналитик" here. Both used to be
	// fall-throughs for a title with no recognised function, and both filed it on the wrong axis.
	// "manager" fell through to a `management` category that told a jobseeker nothing about the work (an Assistant
	// Manager at a burger chain, a Case Manager, a Plant Manager in one facet). The category is retired: a manager title
	// now resolves the CRAFT it names (the "restaurant/store/plant/tax/... manager" spellings are in the managers block at
	// the end of this table), and whether it manages PEOPLE is the `role_type` facet's question (internal/dict/roletype).
	// "analyst" fell through to `data_analytics`, which is technical, so an unrecognised analyst was DECLARED a
	// technical data role: most had no data/analytics/BI word at all (Board Certified Behavior Analysts, investment,
	// purchasing, risk, pricing, policy and tax analysts), yet all were eligible for the enrichment budget and shielded
	// from the non-technical dictionary. The technical families it hid are named individually in the blocks above
	// (test, threat/cyber, network, technical, application(s), it, technology, software, infrastructure and cloud
	// analysts). An analyst the dictionary cannot place now resolves to nothing, as for any title this package cannot read.
	// software_engineering: the generic catch-all for a title classify.IsTech's techTitleTerms confirms as software/IT
	// work but that names no sub-discipline ("Software Engineer", "Java Developer" do not say backend vs frontend vs
	// fullstack, and this package never guesses). Every entry below mirrors a techTitleTerms member with no more specific
	// entry above it (cross-checked against tech.go). Kept at the very bottom, second-to-last before the 1C fallback, so
	// any more specific alias above wins first.
	// Deliberately EXCLUDED: bare "programmer": techTitleTerms has it for is_tech, but "CNC Programmer" is a machining
	// role, so a category entry would mislabel it.
	// The base phrases ("software engineer", "software developer", "software development engineer") carry every qualified
	// variant by substring, like the bare "design engineer" above: "Senior Software Engineer, Platform" resolves without
	// its own entry.
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
