package classify

// coreCategoryTable9 is part 9 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable9 = []aliasEntry{
	{"datenbankadministrator", "devops"},
	{"netzwerktechniker", "network_engineering"},
	{"softwaretester", "qa"},
	{"anwendungsentwickler", "software_engineering"},

	// Systemtechniker/Systemelektroniker also name non-IT disciplines ("Systemtechniker Elektrotechnik"), like the Systems Engineer family below. Only IT-qualified spellings resolve; the bare word is absent. Hyphenated and spaced forms are different strings to the matcher, so each needs its own entry.
	{"it systemtechniker", "devops"},
	{"it-systemtechniker", "devops"},
	{"it systemelektroniker", "devops"},
	{"it-systemelektroniker", "devops"},

	// Fachinformatiker never names a non-IT role, so the bare word resolves, but it is declared LAST so a title with the qualifier right next to it gets the more precise category. SPS-Programmierer is deliberately absent: already excluded from software, like "CNC Programmer" above.
	{"fachinformatiker systemintegration", "devops"},
	{"fachinformatiker für systemintegration", "devops"},
	{"fachinformatiker anwendungsentwicklung", "software_engineering"},
	{"fachinformatiker für anwendungsentwicklung", "software_engineering"},
	{"fachinformatiker", "devops"},

	// The Russian engineering family. The qualified forms naming another discipline come first ("Инженер-проектировщик" is a draughtsman, "Инженер по защите информации" a security engineer), or the bare token below would claim both.
	//
	// Bare tokens throughout: qualified forms hyphenate or postfix a phrase, and a hyphen is a word boundary, so only the bare token reaches every spelling.
	{"инженер-проектировщик", "engineering_design"},
	{"инженер по защите информации", "security"},
	{"инженер", "industrial_engineering"},
	{"технолог", "industrial_engineering"},

	// The Russian consumer vocabularies. `врач` is bare because qualified forms hyphenate or postfix a phrase.
	//
	// Hazard: a short alias hiding inside a longer word (`Делопроизводитель` ends in `водитель`, `Электромеханик` contains `механик`). wordmatch matches on boundaries so it cannot happen, but both pairs carry a regression test.
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
	// General building maintenance, the same work the trades above cover.
	{"рабочий по комплексному обслуживанию", "skilled_trades"},
	{"рабочий по благоустройству", "skilled_trades"},

	// The Russian service vocabularies.
	//
	// `делопроизводитель` is declared FIRST on purpose: an office clerk whose word ends in `водитель`. Word boundaries already keep the driver alias off it; the order makes the hazard visible. The regression test asserts it resolves to `administration`.
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

	// Russian inflects and wordmatch has no morphology: "Помощник воспитателя" is genitive, which the nominative alias cannot reach.
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

	// The Russian half of the non-technical title dictionary (see the English block above the 1С entries). Bare tokens, since a hyphen is a word boundary: "Продавец-кассир" is reached by "продавец".
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

	// No bare English "engineer": "Product Engineer", "Growth Engineer" and "Staff Engineer" are pinned to NO category on purpose, and a bare alias would override all of them. It would also break `Categories()`, which returns every matching alias, adding this category to every engineering title. The Russian bare token has this problem far less ("инженер" heads fewer resolved titles) and its qualified forms are declared above it.

	// Media production, declared after every discipline block (only the three SOC families below follow). Each craft is also a tool or second hat in someone else's title ("Marketing Specialist (Photoshop, Illustrator)"). Resolution is in declaration order, so declaring these above `design` or `marketing` would take those rows. Declared last, a title resolves to the craft only when it names no other discipline.
	//
	// Cost: "Social Media Video Editor" resolves to `marketing`, which is still findable on a correct facet.
	//
	// Bare craft words ("video", "audio", "art", "sound", "photo") are not aliases: each occurs across every discipline ("Audio DSP Engineer", "Art Director").
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

	// SOCIAL SERVICES, SCIENCE AND MANUFACTURING, declared after every other block so a title any alias above resolves keeps its category; these lines only reach a title that named nothing else. The one deliberate move is the four production-floor spellings above, re-filed from `skilled_trades`.
	//
	// Community and social service (SOC 21). Mental-health counsellors and therapists stay in `healthcare`. A bare "case manager" is a nurse as often as a social worker, so only qualified forms are here.
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

	// Science (SOC 19). The clinical laboratory comes first and goes to `healthcare`: SOC files clinical laboratory technologists under 29-2010 (healthcare practitioners), not 19-4000. A cath lab is a cardiac procedure room, not a laboratory.
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
