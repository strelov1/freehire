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
	// React Native is mobile-only, unlike bare "react" above. Xamarin needs its own alias: its titles write "Mobile/Xamarin
	// Developer", a slash where the qualified phrases above want a space.
	{"react native developer", "mobile"},
	{"xamarin", "mobile"},
	// Hungarian: "mobilalkalmazás" is the mobile app. Both spellings are listed because a hyphen is a word boundary, so neither contains the other.
	{"mobilalkalmazás-fejlesztő", "mobile"},
	{"mobilalkalmazás fejlesztő", "mobile"},
	// Russian qualifies the same way, never the bare adjective ("Мобильный банкир" is built from it).
	{"мобильный разработчик", "mobile"},
	{"мобильных приложений", "mobile"},
	{"мобильное приложение", "mobile"},
	{"мобильная разработка", "mobile"},
	{"мобильной разработки", "mobile"},
	// Penetration-testing titles must precede the QA block's bare "tester" fall-through below.
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
	// Without this entry the bare "analyst" fall-through would call them data analysts.
	{"test analyst", "qa"},
	// Hungarian, qualified only. The bare "tesztelő" is not an entry: on a general-population board it also matches
	// "Szivattyú tesztelő" (pump testing). English keeps bare "tester" because its boards are not general-population.
	// The adjective "tesztelési" is out too: it is mostly calibration and manufacturing process work.
	{"szoftvertesztelő", "qa"},
	{"manuális tesztelő", "qa"},
	{"automata tesztelő", "qa"},
	{"tesztmenedzser", "qa"},
	// Physical security is not information security: a guard is the protective-service craft (SOC 33-9032), filed under
	// `public_safety`. "Security Officer", the commonest guard title, is declared at the END of this block, behind the
	// infosec officer spellings that contain it.
	{"security guard", "public_safety"},
	{"security guards", "public_safety"},
	{"armed guard", "public_safety"},
	{"unarmed guard", "public_safety"},
	// Bare "security" is NOT an entry: the word alone is the guard at least as often as the discipline ("Security
	// Officer", "Security Specialist", night-shift front desk). `security` is in vocab.TechCategories, so a bare alias
	// would make jobderive set is_tech TRUE for a mall guard, ahead of the non-tech dictionary.
	// Only the qualified forms below resolve. "information security" covers the officer/analyst/engineer/manager/
	// specialist family, so "Chief Information Security Officer" keeps its category while "Security Officer" loses it.
	// Deliberately absent as genuinely ambiguous: "security specialist", "security manager", "security supervisor",
	// "security consultant". They resolve to no category and is_tech stays unknown (the never-guess contract).
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
	// Hungarian. The bare adjective "biztonsági" is the guard and the fire warden as often as the discipline, so only
	// the qualified forms are entries, in each spelling prod carries: closed compound, spaced pair, hyphenated form
	// (three different strings to a word-boundary match).
	{"kiberbiztonsági", "security"},
	{"információbiztonsági", "security"},
	{"információ biztonsági", "security"},
	{"it biztonsági", "security"},
	{"it-biztonsági", "security"},
	{"informatikai biztonsági", "security"},
	// Narrower technical niches within security. No bare "compliance": titles are dominated by non-IT banking/legal/
	// customs compliance (routed to `legal` via "compliance officer/manager/analyst" above).
	{"iam", "security"},
	{"identity and access management", "security"},
	{"grc", "security"},
	{"vulnerability management", "security"},
	{"vulnerability analyst", "security"},
	// The analyst forms the bare fall-through used to claim.
	{"threat analyst", "security"},
	{"cyber analyst", "security"},
	// The security operations centre; without it the title falls to the bare "analyst" alias and lands in data_analytics.
	{"soc analyst", "security"},
	{"incident response", "security"},
	{"red team", "security"},
	{"red teamer", "security"},
	{"blue team", "security"},
	{"threat intelligence", "security"},
	{"threat intel", "security"},
	// "chief information security officer" needs no entry: the bare "security" alias above already catches it.
	{"ciso", "security"},
	// IT-specific audit (controls/access/compliance review), unlike the bare "internal auditor"/"auditor" nouns this
	// file omits: those span every industry's financial and quality audit.
	{"it auditor", "security"},
	// DevSecOps stays security, not devops: the security responsibility (SAST/DAST, container/IaC scanning,
	// policy-as-code) is why the title exists.
	{"devsecops", "security"},
	// "Security Officer" is a guard about nine times in ten, so every infosec spelling that CONTAINS the phrase is
	// declared first, as a whole phrase ending in "officer". Whole phrases on purpose: `Categories()` returns every
	// matching alias, and a shorter alias inside the span of a longer, earlier match is not counted (see
	// matchAllOrdered). "information security" alone would not cover "officer", so "Chief Information Security
	// Officer" would also come back tagged with the guard category on the multi-category CV path.
	// The defence-industry "Program/Facility Security Officer" administers clearances: neither a guard nor an
	// infosec engineer, so it names nothing.
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
	// Hungarian: "beágyazott" is embedded. It precedes the plain "szoftverfejlesztő" entry below, which would claim it.
	{"beágyazott szoftverfejlesztő", "embedded"},
	{"встраиваемые", "embedded"},
	{"встраиваемых", "embedded"},
	// French: "logiciel embarqué" is embedded software, unambiguous.
	{"logiciel embarqué", "embedded"},
	{"blockchain", "blockchain"},
	{"блокчейн", "blockchain"},
	// "Web3"/"smart contract" name the blockchain domain as unambiguously as "blockchain", so these resolve here, not
	// in the software_engineering catch-all.
	{"web3 developer", "blockchain"},
	{"smart contract developer", "blockchain"},
	{"hardware", "hardware"},
	{"fpga", "hardware"},
	// Chip-design titles that also carry "architect": declared above the bare "architect" alias, which would claim
	// them for software architecture.
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
	// technical_writing before design/ux so "UX Writer"/"Content Designer" win over the bare "ux"/"designer" entries.
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
	// Instructional/curriculum design is a content craft (courses, learning materials), not product design, so it
	// must precede the bare "designer" entry. It stays out of `hr`'s "learning and development" alias: L&D is the
	// internal-training function, this is the authoring craft, and it appears at companies with no L&D team.
	{"instructional designer", "technical_writing"},
	{"instructional design", "technical_writing"},
	{"curriculum designer", "technical_writing"},
	{"learning designer", "technical_writing"},
	{"learning experience designer", "technical_writing"},
	{"e-learning developer", "technical_writing"},
	{"elearning developer", "technical_writing"},
	// Bare "translator"/"переводчик" names one craft in every industry, so no qualifier is needed. NOT "translation":
	// it also names an NLP/MT discipline ("Machine Translation Engineer" is ml_ai/software), so a bare entry would
	// misfile those.
	{"translator", "technical_writing"},
	{"технический писатель", "technical_writing"},
	{"техписатель", "technical_writing"},
	{"технический редактор", "technical_writing"},
	{"разработчик документации", "technical_writing"},
	{"специалист по документации", "technical_writing"},
	{"ux-редактор", "technical_writing"},
	{"переводчик", "technical_writing"},
	// "design" names two unrelated crafts. Everything below down to the engineering block is a title whose "design"
	// is NOT product design: engineering draughting, chip and board design, or a network role. These must precede
	// the bare "designer"/"design" entries, which would claim them by the word alone.
	// First, the titles that state a craft of their own and must NOT go to the engineering-design bucket built next.
	{"software design engineer in test", "qa"},
	{"design engineer in test", "qa"},
	{"software design engineer", categoryNone},
	{"software design engineering", categoryNone},
	{"network design engineer", "network_engineering"},
	{"cloud design engineer", "devops"},
	{"solution design engineer", "solutions_engineering"},
	{"solutions design engineer", "solutions_engineering"},
	// Silicon and board design belong to `hardware`, which already owns the rest of that team via the earlier
	// "hardware"/"fpga" aliases; routing them to draughting would split one discipline across two facets. The list
	// must name the whole family: whatever is missing falls through to the bare "design engineer" at the bottom of
	// the block and lands in draughting.
	{"pcb design", "hardware"},
	{"pcb designer", "hardware"},
	{"pcb layout", "hardware"},
	{"physical design engineer", "hardware"},
	{"analog design engineer", "hardware"},
	{"rtl design engineer", "hardware"},
	{"mixed signal design engineer", "hardware"},
}
