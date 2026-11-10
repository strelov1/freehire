package classify

// coreCategoryTable9 is part 9 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable9 = []aliasEntry{
	{"datenbankadministrator", "devops"},
	{"netzwerktechniker", "network_engineering"},
	{"softwaretester", "qa"},
	{"anwendungsentwickler", "software_engineering"},

	// Systemtechniker/Systemelektroniker also name non-IT disciplines in prod
	// titles ("Systemtechniker Elektrotechnik", "Systemtechniker
	// Sicherheitstechnik") — the same cross-domain trap the Systems Engineer
	// family below documents. Only the IT-qualified spellings resolve; the
	// bare word is deliberately absent, and hyphenated/spaced forms are two
	// different strings to this matcher so both need their own entry.
	{"it systemtechniker", "devops"},
	{"it-systemtechniker", "devops"},
	{"it systemelektroniker", "devops"},
	{"it-systemelektroniker", "devops"},

	// Fachinformatiker: the German formal IT-specialist title and
	// apprenticeship. Unlike Systemtechniker above, it never names a non-IT
	// role, so the bare word resolves too — but declared LAST, after its two
	// dominant qualifiers, so a title where the qualifier sits directly next
	// to the word (no intervening "für"/"/in"/"m/w/d") gets the more precise
	// category. SPS-Programmierer (PLC/industrial-controller programming) is
	// deliberately NOT given an entry here — already excluded from a software
	// category, same reasoning as "CNC Programmer" above.
	{"fachinformatiker systemintegration", "devops"},
	{"fachinformatiker für systemintegration", "devops"},
	{"fachinformatiker anwendungsentwicklung", "software_engineering"},
	{"fachinformatiker für anwendungsentwicklung", "software_engineering"},
	{"fachinformatiker", "devops"},

	// The Russian engineering family. Roughly half the industrial residue, and none of
	// it carried an English alias. The two qualified forms that name ANOTHER discipline
	// are declared first: "Инженер-проектировщик" is a draughtsman and
	// "Инженер по защите информации" an information-security engineer, and the bare
	// token below would otherwise claim both.
	//
	// Bare tokens for the same reason the software ones above are: the qualified forms
	// either hyphenate ("Инженер-технолог") or postfix a prepositional phrase
	// ("Инженер по подготовке производства"), and a hyphen is a word boundary — only
	// the bare token reaches every spelling.
	{"инженер-проектировщик", "engineering_design"},
	{"инженер по защите информации", "security"},
	{"инженер", "industrial_engineering"},
	{"технолог", "industrial_engineering"},

	// The Russian consumer vocabularies. `врач` is a bare token for the same reason
	// every Russian entry here is: the qualified forms hyphenate (`Врач-терапевт`,
	// `Врач-акушер-гинеколог`) or postfix a phrase (`Врач ультразвуковой
	// диагностики`), and a hyphen is a word boundary.
	//
	// The hazard runs the other way here — a short alias hiding INSIDE a longer word.
	// `Делопроизводитель` (an office clerk) ends in `водитель`, and `Электромеханик`
	// contains `механик`. wordmatch matches on boundaries and cannot make that
	// mistake, but nothing in this list shows the hazard, so both pairs carry a
	// regression test.
	{"ветеринарный врач", "healthcare"},
	{"врач", "healthcare"},
	{"медсестра", "healthcare"},
	{"медбрат", "healthcare"},
	{"фельдшер", "healthcare"},
	{"санитар", "healthcare"},
	{"электромеханик", "skilled_trades"},
	{"электросварщик", "skilled_trades"},
	{"электромонтёр", "skilled_trades"},
	{"электромонтер", "skilled_trades"},
	{"электрик", "skilled_trades"},
	{"сварщик", "skilled_trades"},
	{"слесарь", "skilled_trades"},
	{"плотник", "skilled_trades"},
	{"маляр", "skilled_trades"},
	{"механик", "skilled_trades"},
	// General building maintenance — 5 256 open across these two spellings, and the
	// same work the trades above already cover.
	{"рабочий по комплексному обслуживанию", "skilled_trades"},
	{"рабочий по благоустройству", "skilled_trades"},

	// The Russian service vocabularies.
	//
	// `делопроизводитель` is declared FIRST and on purpose: it is an office clerk, and
	// it ENDS in `водитель`. wordmatch matches on word boundaries so the bare driver
	// alias could not take it anyway — but the two now exist together in production,
	// in different categories, and a reader scanning this list would have no way to
	// see the hazard. The regression test asserts the clerk resolves to
	// `administration` specifically, not merely to something.
	{"делопроизводитель", "administration"},
	{"секретарь", "administration"},
	{"администратор", "administration"},
	{"офис-менеджер", "administration"},
	{"личный помощник", "administration"},
	{"личный ассистент", "administration"},
	{"виртуальный ассистент", "administration"},
	{"виртуальный помощник", "administration"},

	{"сборщик заказов", "logistics"},
	{"заведующий складом", "logistics"},
	{"кладовщик", "logistics"},
	{"экспедитор", "logistics"},
	{"грузчик", "logistics"},
	{"курьер", "logistics"},
	{"водитель", "logistics"},

	// Russian inflects, and `wordmatch` has no more morphology for it than it has for
	// English plurals: "Помощник воспитателя" (2 097 open across two spellings) is the
	// genitive and the nominative alias cannot reach it. The same trap as the
	// automotive plurals fixed in this change, in another language.
	{"воспитателя", "education"},
	{"воспитатель", "education"},
	{"преподаватель", "education"},
	// A school's social worker, not a teacher: declared above the bare "педагог" that
	// would otherwise take it.
	{"социальный педагог", "social_services"},
	{"педагог", "education"},
	{"методист", "education"},

	{"парикмахер", "personal_services"},
	{"уборщик", "personal_services"},
	{"уборщица", "personal_services"},
	{"сиделка", "personal_services"},

	// The Russian half of the non-technical title dictionary, filed (see the English
	// block above the 1С entries for why). Bare tokens, as everywhere in Russian here:
	// "Продавец-кассир" and "Продавец-консультант" are reached by "продавец", since a
	// hyphen is a word boundary.
	{"медицинская сестра", "healthcare"},
	{"санитарка", "healthcare"},
	{"логопед", "healthcare"},
	{"электрогазосварщик", "skilled_trades"},
	{"сантехник", "skilled_trades"},
	{"токарь", "skilled_trades"},
	{"фрезеровщик", "skilled_trades"},
	{"штукатур", "skilled_trades"},
	{"каменщик", "skilled_trades"},
	{"бетонщик", "skilled_trades"},
	{"арматурщик", "skilled_trades"},
	{"стропальщик", "skilled_trades"},
	{"монтажник", "skilled_trades"},
	{"машинист", "skilled_trades"},
	{"тракторист", "skilled_trades"},
	{"разнорабочий", "skilled_trades"},
	{"подсобный рабочий", "skilled_trades"},
	{"повар", "hospitality"},
	{"пекарь", "hospitality"},
	{"кондитер", "hospitality"},
	{"официант", "hospitality"},
	{"официантка", "hospitality"},
	{"бармен", "hospitality"},
	{"посудомойщик", "hospitality"},
	{"кухонный рабочий", "hospitality"},
	{"продавец", "retail"},
	{"кассир", "retail"},
	{"товаровед", "retail"},
	{"мерчендайзер", "retail"},
	{"директор магазина", "retail"},
	{"комплектовщик", "logistics"},
	{"упаковщик", "logistics"},
	{"фасовщик", "logistics"},
	{"приёмщик", "logistics"},
	{"приемщик", "logistics"},
	{"дворник", "personal_services"},
	{"вахтер", "personal_services"},
	{"вахтёр", "personal_services"},
	{"консьерж", "personal_services"},
	{"няня", "personal_services"},
	{"маникюрша", "personal_services"},
	{"косметолог", "personal_services"},
	{"учитель", "education"},

	// NO bare English "engineer", though 689 open postings spell it exactly. It was
	// tried and the existing suite rejected it, which is the answer: "Product
	// Engineer", "Growth Engineer", "Staff Engineer" and "Developer Onboarding
	// Engineer" are all pinned to NO category on purpose, because the word before
	// "engineer" is what decides and those words are ambiguous. A bare alias overrides
	// every one of those decisions at once.
	//
	// It also breaks `Categories()`, which returns EVERY matching alias rather than the
	// strongest: a bare "engineer" appends this category to "Senior Backend Engineer"
	// and to every other engineering title in the catalogue, polluting the multi-
	// category CV path that reads it.
	//
	// The Russian bare token above does not have the second problem to the same degree
	// — "инженер" heads far fewer resolved titles here — and its qualified forms are
	// declared above it, so `Parse` stays correct.

	// Media production, declared after every discipline block on purpose (only the
	// three SOC families below follow it, and they reach no title anything above
	// resolves). Every craft here is also a tool or a
	// second hat named inside someone else's title — "Marketing Specialist (Photoshop,
	// Illustrator)", "Graphic Designer & Photographer", "Junior Motion Designer /
	// Animator". This table resolves in declaration order, so declaring these anywhere
	// above `design` or `marketing` does not merely add a category: it TAKES those
	// rows, which is the one thing this change promised not to do. Declared last, a
	// title resolves to the craft only when it names no other discipline at all.
	//
	// The cost is stated rather than hidden: a "Social Media Video Editor" resolves to
	// `marketing`, not to the craft. That is the right side to err on — the posting is
	// still findable, on a facet that was already correct for it.
	//
	// The bare craft words ("video", "audio", "art", "sound", "photo") are not aliases
	// in either block: each occurs in titles across every discipline ("Audio DSP
	// Engineer", "Art Director", "State of the Art").
	{"video editor", "creative"},
	{"video producer", "creative"},
	{"videographer", "creative"},
	{"photographer", "creative"},
	{"photo editor", "creative"},
	{"animator", "creative"},
	// "motion graphics artist" is the artist spelling of the craft; the DESIGNER
	// spelling ("Motion Graphics Designer") stays in `design`, where its named role
	// already lives — and would win here regardless, being declared above.
	{"motion graphics artist", "creative"},
	{"concept artist", "creative"},
	{"character artist", "creative"},
	{"environment artist", "creative"},
	{"technical artist", "creative"},
	{"storyboard artist", "creative"},
	{"vfx artist", "creative"},
	{"3d artist", "creative"},
	{"2d artist", "creative"},
	// Also the Adobe product, which is why it is here rather than beside the crafts it
	// belongs with: a design or marketing title that names the tool keeps its own row.
	{"illustrator", "creative"},

	// SOCIAL SERVICES, SCIENCE AND MANUFACTURING — the three families the catalogue
	// filed nowhere once it grew past IT, so ingest refused them and search excluded
	// them. Declared after every other block, the media crafts included, so a title any
	// alias above resolves keeps exactly the category it had: these lines only reach a
	// title that named nothing else. The one deliberate move is the four production-
	// floor spellings above, re-filed from `skilled_trades` in place.
	//
	// Community and social service (SOC 21). Counsellors and therapists who treat
	// mental health stay in `healthcare`, where they already resolve. A bare "case
	// manager" is a nurse as often as a social worker and resolves nothing; only the
	// qualified forms are here.
	{"social worker", "social_services"},
	{"social workers", "social_services"},
	{"social work", "social_services"},
	{"social services", "social_services"},
	{"social service", "social_services"},
	// Qualified: a bare "human services" is also a consulting practice ("Health and
	// Human Services Strategy").
	{"human services worker", "social_services"},
	{"human services specialist", "social_services"},
	{"human services assistant", "social_services"},
	{"human services professional", "social_services"},
	{"lcsw", "social_services"},
	{"lmsw", "social_services"},
	{"licsw", "social_services"},
	{"case worker", "social_services"},
	{"caseworker", "social_services"},
	{"child welfare", "social_services"},
	{"child protective", "social_services"},
	{"foster care", "social_services"},
	{"community liaison", "social_services"},
	{"outreach worker", "social_services"},
	{"community health worker", "social_services"},
	{"youth worker", "social_services"},
	{"youth care worker", "social_services"},
	{"youth counselor", "social_services"},
	{"family advocate", "social_services"},
	{"victim advocate", "social_services"},
	{"victim services", "social_services"},
	{"substance abuse counselor", "social_services"},
	{"substance use counselor", "social_services"},
	{"addiction counselor", "social_services"},
	{"drug and alcohol counselor", "social_services"},
	{"recovery coach", "social_services"},
	{"rehabilitation counselor", "social_services"},
	// An after-school counselor runs the afternoon club, not the guidance office; it
	// would otherwise reach "school counselor" across the hyphen.
	{"after school counselor", "education"},
	{"after-school counselor", "education"},
	{"school counselor", "social_services"},
	{"guidance counselor", "social_services"},
	{"probation officer", "social_services"},
	{"parole officer", "social_services"},
	{"chaplain", "social_services"},
	{"pastor", "social_services"},
	{"социальный работник", "social_services"},
	{"соцработник", "social_services"},
	{"социальной работе", "social_services"}, // "специалист по социальной работе"
	{"социальной работы", "social_services"}, // "... ведения социальной работы"
	// German has no morphology here either: the -in and -en forms are their own
	// words, while the gender star and colon are word boundaries ("Sozialpädagog*in").
	{"sozialarbeiter", "social_services"},
	{"sozialarbeiterin", "social_services"},
	{"schulsozialarbeiter", "social_services"},
	{"schulsozialarbeiterin", "social_services"},
	{"sozialpädagoge", "social_services"},
	{"sozialpädagogin", "social_services"},
	{"sozialpädagogen", "social_services"},
	{"sozialpädagog", "social_services"},
	{"travailleur social", "social_services"},
	{"travailleuse sociale", "social_services"},
	{"assistant social", "social_services"},
	{"assistante sociale", "social_services"},
	{"éducateur spécialisé", "social_services"},
	{"éducatrice spécialisée", "social_services"},
	{"assistente social", "social_services"},
	{"trabajador social", "social_services"},
	{"trabajadora social", "social_services"},

	// Science (SOC 19): the laboratory and the named sciences.
	//
	// The clinical laboratory comes first and goes to `healthcare`: SOC files medical
	// and clinical laboratory technologists and technicians under 29-2010, the
	// healthcare practitioners, not with the science technicians of 19-4000. The
	// hospital lab is staffed and hired as part of patient care. A cath lab is a
	// cardiac procedure room, not a laboratory at all.
	{"medical lab technician", "healthcare"},
	{"medical laboratory technician", "healthcare"},
	{"medical lab tech", "healthcare"},
	{"medical laboratory technologist", "healthcare"},
	{"medical laboratory scientist", "healthcare"},
	{"medical lab scientist", "healthcare"},
	{"clinical lab technician", "healthcare"},
	{"clinical laboratory technician", "healthcare"},
	{"clinical laboratory technologist", "healthcare"},
}
