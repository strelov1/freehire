package classify

// coreCategoryTable2 is part 2 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable2 = []aliasEntry{
	{"mobile engineer", "mobile"},
	{"mobile architect", "mobile"},
	{"mobile app", "mobile"},
	{"mobile apps", "mobile"},
	{"mobile application", "mobile"},
	{"mobile applications", "mobile"},
	{"mobile software", "mobile"},
	{"mobile platform", "mobile"},
	{"mobile automation", "mobile"},
	{"mobile qa", "mobile"},
	{"android", "mobile"},
	{"ios", "mobile"},
	// React Native is mobile-only, unlike bare "react" above. Xamarin is too, and it
	// needs its own alias because the titles that carry it write "Mobile/Xamarin
	// Developer" — a slash where the qualified phrases above want a space.
	{"react native developer", "mobile"},
	{"xamarin", "mobile"},
	// Hungarian: "mobilalkalmazás" is the mobile app. Both spellings are listed
	// because a hyphen is a word boundary here, so neither form contains the other.
	{"mobilalkalmazás-fejlesztő", "mobile"},
	{"mobilalkalmazás fejlesztő", "mobile"},
	// Russian qualifies the same way: the app, the craft, or the developer — never the
	// bare adjective, which is what "Мобильный банкир" is built from.
	{"мобильный разработчик", "mobile"},
	{"мобильных приложений", "mobile"},
	{"мобильное приложение", "mobile"},
	{"мобильная разработка", "mobile"},
	{"мобильной разработки", "mobile"},
	// Penetration-testing titles must precede the QA block's bare "tester" fall-through
	// right below — it would otherwise claim "Penetration Tester" for qa.
	{"penetration tester", "security"},
	{"penetration testing", "security"},
	{"pentester", "security"},
	{"pentest", "security"},
	{"qa", "qa"},
	{"quality assurance", "qa"},
	{"tester", "qa"},
	{"test engineer", "qa"},
	{"test automation", "qa"},
	{"sdet", "qa"},
	{"тестировщик", "qa"},
	{"тестирование", "qa"},
	// 524 live postings. Reached only through the bare "analyst" fall-through before it
	// was removed, which called them data analysts.
	{"test analyst", "qa"},
	// Hungarian, qualified only. The bare noun was admitted here for one day on the
	// footing English's bare "tester" has, and a general-population board took it back:
	// the first sample came from the platform's two IT categories, where every
	// "tesztelő" is a testing seat, and the other 21 hold "Szivattyú tesztelő" —
	// whoever tests pumps. English keeps its bare entry because the boards it is read
	// on are not general-population ones; this language is read on one that is.
	//
	// The adjective "tesztelési" is out for the same reason it always was: the sample
	// spends it on calibration and manufacturing process work ("Kalibrálási- és
	// tesztelési folyamatfejlesztő mérnök").
	{"szoftvertesztelő", "qa"},
	{"manuális tesztelő", "qa"},
	{"automata tesztelő", "qa"},
	{"tesztmenedzser", "qa"},
	// Physical security is not information security: a guard is the protective-service
	// craft (SOC 33-9032), filed under `public_safety` with police and firefighters, and
	// an infosec filter must not return him. "Security Officer" — the commonest guard
	// title — is declared at the END of this block, behind the infosec officer spellings
	// that contain it; see the note there.
	{"security guard", "public_safety"},
	{"security guards", "public_safety"},
	{"armed guard", "public_safety"},
	{"unarmed guard", "public_safety"},
	// Bare "security" is NOT an entry, for the reason the Hungarian block below already
	// gives about "biztonsági": the word alone is the guard at least as often as the
	// discipline. Measured over the live catalogue, the single commonest title carrying
	// it is "Security Officer" (1 102 open postings), followed by "Security Specialist"
	// (602) and "Night shift security front desk - receptionist" (483) — none of them
	// technical, and "Security Supervisor" (149) and bare "Security" (91) behind them.
	// The bare alias sent every one of those to the security category, and because
	// `security` is in vocab.TechCategories that category is enough for
	// jobderive to set is_tech TRUE on its own, ahead of the non-tech
	// dictionary. So a mall guard was filed as an IT security role — in search, and
	// (since 2026-09-14) in what cmd/search-ping spends Google's Indexing API quota on.
	//
	// Only the qualified forms below, each in the spelling the live sample carries.
	// "information security" covers the officer/analyst/engineer/manager/specialist
	// family in one entry, which is how "Chief Information Security Officer" keeps its
	// category while "Security Officer" loses it. Deliberately ABSENT because they are
	// genuinely ambiguous rather than merely rare: "security specialist", "security
	// manager", "security supervisor", "security consultant" — these resolve to no
	// category and their is_tech falls to unknown, which is the never-guess contract
	// every dictionary here follows. Same call the bare "analyst" fall-through got.
	{"security engineer", "security"},
	{"security architect", "security"},
	{"security analyst", "security"},
	{"security operations", "security"},
	{"security researcher", "security"},
	{"information security", "security"},
	{"it security", "security"},
	{"application security", "security"},
	{"network security", "security"},
	{"cloud security", "security"},
	{"infosec", "security"},
	{"appsec", "security"},
	{"cybersecurity", "security"},
	{"cyber security", "security"},
	{"безопасность", "security"},
	{"безопасности", "security"},
	{"кибербезопасность", "security"},
	// Hungarian. The bare adjective "biztonsági" is the guard and the fire warden as
	// often as the discipline — the live sample holds "Biztonsági operátor és
	// fegyveres biztonsági őr" and "Tűz- és munkabiztonsági szakértő" — so it is not
	// an entry and only the qualified forms are. Each is listed in the spellings prod
	// actually carries: the closed compound, the spaced pair, and the hyphenated form,
	// which are three different strings to a word-boundary match.
	{"kiberbiztonsági", "security"},
	{"információbiztonsági", "security"},
	{"információ biztonsági", "security"},
	{"it biztonsági", "security"},
	{"it-biztonsági", "security"},
	{"informatikai biztonsági", "security"},
	// Narrower technical niches within security. Deliberately no bare "compliance" —
	// sampled live titles are dominated by non-IT banking/legal/customs compliance
	// (that population already routes to `legal` via "compliance officer/manager/
	// analyst" above); a bare entry here would be a GTM-style word-trap.
	{"iam", "security"},
	{"identity and access management", "security"},
	{"grc", "security"},
	{"vulnerability management", "security"},
	{"vulnerability analyst", "security"},
	// The analyst forms the bare fall-through used to claim: 263 and 80 live postings.
	{"threat analyst", "security"},
	{"cyber analyst", "security"},
	// The security operations centre. Without this the title falls through to the
	// bare "analyst" alias far below and lands in data_analytics.
	{"soc analyst", "security"},
	{"incident response", "security"},
	{"red team", "security"},
	{"red teamer", "security"},
	{"blue team", "security"},
	{"threat intelligence", "security"},
	{"threat intel", "security"},
	// "chief information security officer" needs no entry of its own: the bare
	// "security" alias above already catches it as a whole word.
	{"ciso", "security"},
	// IT-specific audit (controls/access/compliance review), unlike the bare
	// "internal auditor"/"auditor" nouns this file deliberately omits — those span
	// every industry's financial and quality audit functions and are not IT-anchored.
	{"it auditor", "security"},
	// DevSecOps stays security, not devops: the security responsibility (SAST/DAST,
	// container/IaC scanning, policy-as-code) is why the title exists, not incidental.
	{"devsecops", "security"},
	// "Security Officer" is a guard about nine times in ten — measured on a 2% sample of
	// the open catalogue, 175 of 192 titles carrying it named no information-security
	// word ("Unarmed Security Officer", "Mall Security Officers", "Security Officer -
	// Marriott ..."). The rest are infosec officers, and every spelling of those that
	// CONTAINS the phrase is declared first, as a whole phrase ending in "officer".
	//
	// Whole phrases on purpose: `Categories()` returns every matching alias, and an
	// occurrence of a shorter alias lying inside the span a longer, earlier alias already
	// matched is not counted (see matchAllOrdered). "information security" alone would
	// not cover the word "officer", so "Chief Information Security Officer" would come
	// back tagged with the guard category on the multi-category CV path.
	//
	// The defence-industry "Program/Facility Security Officer" administers clearances and
	// classified material: neither a guard nor an infosec engineer, so it names nothing.
	{"information security officer", "security"},
	{"information systems security officer", "security"},
	{"information system security officer", "security"},
	{"it security officer", "security"},
	{"ict security officer", "security"},
	{"cyber security officer", "security"},
	{"data security officer", "security"},
	{"network security officer", "security"},
	{"cloud security officer", "security"},
	{"application security officer", "security"},
	{"program security officer", categoryNone},
	{"facility security officer", categoryNone},
	{"security officer", "public_safety"},
	{"security officers", "public_safety"},
	{"embedded", "embedded"},
	{"firmware", "embedded"},
	// Hungarian: "beágyazott" is embedded. It precedes the plain
	// "szoftverfejlesztő" entry further down, which would otherwise claim it.
	{"beágyazott szoftverfejlesztő", "embedded"},
	{"встраиваемые", "embedded"},
	{"встраиваемых", "embedded"},
	// French: "logiciel embarqué" is embedded software, unambiguous.
	{"logiciel embarqué", "embedded"},
	{"blockchain", "blockchain"},
	{"блокчейн", "blockchain"},
	// "Web3"/"smart contract" name the blockchain domain as unambiguously as the
	// word "blockchain" itself, so these resolve here rather than the generic
	// software_engineering catch-all.
	{"web3 developer", "blockchain"},
	{"smart contract developer", "blockchain"},
	{"hardware", "hardware"},
	{"fpga", "hardware"},
	// Chip-design titles that also carry "architect": declared above the bare
	// "architect" alias, which would otherwise claim them for software architecture.
	// "ASIC Digital Design, <grade>" alone is ~100 open postings from one employer.
	{"asic digital design", "hardware"},
	{"ic digital design", "hardware"},
	{"digital design architect", "hardware"},
	{"digital design flow", "hardware"},
	{"digital design capability architect", "hardware"},
	{"solutions architect", "architecture"},
	{"software architect", "architecture"},
	{"enterprise architect", "architecture"},
	{"cloud architect", "architecture"},
	{"architect", "architecture"},
	{"архитектор", "architecture"},
	// technical_writing before design/ux so "UX Writer"/"Content Designer" (product
	// documentation & content craft) win over the bare "ux"/"designer" design entries.
	// Promotional "copywriter"/"content writer" deliberately stay in marketing (below).
	{"technical writer", "technical_writing"},
	{"technical writing", "technical_writing"},
	{"technical communicator", "technical_writing"},
	{"documentation specialist", "technical_writing"},
	{"documentation manager", "technical_writing"},
	{"documentation engineer", "technical_writing"},
	{"information developer", "technical_writing"},
	{"content designer", "technical_writing"},
	{"ux writer", "technical_writing"},
	{"content strategist", "technical_writing"},
	{"localization specialist", "technical_writing"},
	{"localization manager", "technical_writing"},
	{"localization engineer", "technical_writing"},
	// Instructional/curriculum design is a content craft, not product design — it
	// builds courses and learning materials, not interfaces — so it must precede the
	// bare "designer" entry below for the same reason "content designer" does. It
	// stays out of `hr`'s "learning and development" alias on purpose: L&D is the
	// internal-training FUNCTION, this is the authoring CRAFT, and it shows up at
	// edtech/product companies with no L&D team at all.
	{"instructional designer", "technical_writing"},
	{"instructional design", "technical_writing"},
	{"curriculum designer", "technical_writing"},
	{"learning designer", "technical_writing"},
	{"learning experience designer", "technical_writing"},
	{"e-learning developer", "technical_writing"},
	{"elearning developer", "technical_writing"},
	// Bare "translator"/"переводчик": the role noun names one unambiguous craft in
	// every industry, so no qualifying phrase is needed the way
	// software_engineering's language-anchored forms are. NOT "translation" — that
	// noun also names an NLP/MT discipline ("Machine Translation Engineer",
	// "Translation Engineer" are ml_ai/software roles, not human translators), so a
	// bare entry for it would misfile them the same way a bare "growth"/"compliance"
	// would elsewhere in this file.
	{"translator", "technical_writing"},
	{"технический писатель", "technical_writing"},
	{"техписатель", "technical_writing"},
	{"технический редактор", "technical_writing"},
	{"разработчик документации", "technical_writing"},
	{"специалист по документации", "technical_writing"},
	{"ux-редактор", "technical_writing"},
	{"переводчик", "technical_writing"},
	// The word "design" names two unrelated crafts. Everything below down to the
	// engineering block is a title whose "… design …" is NOT product design: it is
	// engineering draughting (mechanical/electrical/civil), chip and board design, or
	// a network role. They must precede the bare "designer"/"design" entries, which
	// would otherwise claim them by virtue of the word alone — the defect this split
	// fixes (a mining-equipment "Design Engineer" filed under product design).
	//
	// First, the titles that state a craft of their own and must NOT go to the
	// engineering-design bucket the next block builds.
	{"software design engineer in test", "qa"},
	{"design engineer in test", "qa"},
	{"software design engineer", categoryNone},
	{"software design engineering", categoryNone},
	{"network design engineer", "network_engineering"},
	{"cloud design engineer", "devops"},
	{"solution design engineer", "solutions_engineering"},
	{"solutions design engineer", "solutions_engineering"},
	// Silicon and board design belong to `hardware`, which already owns the rest of
	// that team through the earlier "hardware"/"fpga" aliases. Routing them to
	// engineering draughting would split one discipline across two facets and drop
	// them out of the technical treatment (enrichment, embeddings) they have today.
	// The list has to name the whole family: whatever is missing here falls through to
	// the bare "design engineer" at the bottom of the block and lands in draughting.
	{"pcb design", "hardware"},
	{"pcb designer", "hardware"},
	{"pcb layout", "hardware"},
	{"physical design engineer", "hardware"},
	{"analog design engineer", "hardware"},
	{"rtl design engineer", "hardware"},
	{"mixed signal design engineer", "hardware"},
}
