package classify

// coreCategoryTable4 is part 4 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable4 = []aliasEntry{
	{"solution engineer", "solutions_engineering"},
	{"sales engineer", "solutions_engineering"},
	{"presales engineer", "solutions_engineering"},
	{"pre-sales engineer", "solutions_engineering"},
	{"solutions consultant", "solutions_engineering"},
	{"solution consultant", "solutions_engineering"},
	{"sales applications engineer", "solutions_engineering"},
	{"forward deployed engineer", "solutions_engineering"},
	{"пресейл", "solutions_engineering"},
	{"пресейл-инженер", "solutions_engineering"},
	// GTM engineering builds the outbound data pipeline rather than selling, but the
	// split from RevOps is not title-separable, so it rides with the rest of that
	// cluster. Only the phrase resolves — the bare "gtm" names Google Tag Manager in
	// a requirements list, and that meaning belongs to the skill dictionary.
	{"gtm engineer", "sales"},
	{"go-to-market engineer", "sales"},
	{"go to market engineer", "sales"},
	{"sales", "sales"},
	{"account executive", "sales"},
	{"business development", "sales"},
	{"account manager", "sales"},
	{"sdr", "sales"},
	{"bdr", "sales"},
	// RevOps/Sales Ops belong to the GTM cluster; the finance-side rev-rec meaning is
	// not title-separable, so the commercial default routes here (not to finance/ops).
	{"revenue operations", "sales"},
	{"revops", "sales"},
	{"sales operations", "sales"},
	{"продажи", "sales"},
	{"продаж", "sales"},
	{"продажам", "sales"},
	// IT support: the help desk and the service desk, desktop and end-user support,
	// the tiered desk (1st/2nd line, L1-L3), the technical support engineer of a
	// software product and the NOC. Split out of `support` (customer service) into
	// its own TECHNICAL category, so a service-desk veteran finds a facet naming the
	// work and the derivation reads it as technical from the category alone.
	//
	// A phrase earns its place by naming the IT ESTATE, not the mood of the work
	// (the rule tech.go's techTitleTerms carries for the same family). Sampled
	// 2026-10-26 against a 10% sample of the open `support` population (22,638
	// postings): of 624 carrying "help desk"/"service desk", the only non-IT ones
	// were an HR help desk and a customer service desk, pinned first below. Bare
	// "technical support" and bare "support engineer" stay `support`: they also name
	// automotive, field-equipment, robotics and maintenance support, so only the
	// analyst/engineer/specialist forms of technical support are claimed.
	{"customer service desk", "support"},
	{"hr help desk", "hr"},
	{"it support", "it_support"},
	{"it-support", "it_support"},
	// The German/Nordic surface form ("1st Level IT Supporter"): the trailing "er"
	// breaks the word boundary, so "it support" cannot see it.
	{"it supporter", "it_support"},
	{"it-supporter", "it_support"},
	{"it technical support", "it_support"},
	{"ict support", "it_support"},
	{"pc support", "it_support"},
	{"desktop support", "it_support"},
	{"deskside support", "it_support"},
	{"desk side support", "it_support"},
	{"it field support", "it_support"},
	{"desktop engineer", "it_support"},
	{"desktop technician", "it_support"},
	{"desktop analyst", "it_support"},
	// "end user" is an IT term of art; spaced and hyphenated are separate aliases
	// because a hyphen is a word boundary.
	{"end user support", "it_support"},
	{"end-user support", "it_support"},
	{"help desk", "it_support"},
	// The unspaced compound: "help desk" above cannot reach it, the same trap the
	// "back-end"/"back end"/"backend" trio guards against.
	{"helpdesk", "it_support"},
	// The ITIL name for the same desk. Without it the far more common "Service Desk
	// Analyst" fell through to the bare "analyst" alias and read as data_analytics.
	{"service desk", "it_support"},
	{"servicedesk", "it_support"},
	{"technical support engineer", "it_support"},
	{"technical support engineering", "it_support"},
	{"technical support analyst", "it_support"},
	{"technical support specialist", "it_support"},
	{"tech support specialist", "it_support"},
	{"application support", "it_support"},
	{"applications support", "it_support"},
	{"software support", "it_support"},
	{"systems support", "it_support"},
	{"server support", "it_support"},
	{"cloud support engineer", "it_support"},
	{"technology support", "it_support"},
	// French and Spanish: "Technicien support informatique", "Soporte informático".
	{"support informatique", "it_support"},
	{"soporte informático", "it_support"},
	{"soporte informatico", "it_support"},
	// The tiered desk. "Tier 1" is deliberately absent: it heads customer-service
	// titles as often ("Tier 1 Customer Support", "Tier 1 - Customer Service").
	{"1st level support", "it_support"},
	{"2nd level support", "it_support"},
	{"3rd level support", "it_support"},
	{"first level support", "it_support"},
	{"second level support", "it_support"},
	{"third level support", "it_support"},
	{"1st line support", "it_support"},
	{"2nd line support", "it_support"},
	{"3rd line support", "it_support"},
	{"first line support", "it_support"},
	{"second line support", "it_support"},
	{"third line support", "it_support"},
	{"l1 support", "it_support"},
	{"l2 support", "it_support"},
	{"l3 support", "it_support"},
	// The network operations centre watches the estate around the clock.
	{"noc technician", "it_support"},
	{"noc engineer", "it_support"},
	{"noc analyst", "it_support"},
	{"noc specialist", "it_support"},
	{"noc operator", "it_support"},
	// Russian. "Техническая поддержка" is filed by hh.ru under Information
	// Technology ("Специалист технической поддержки") and the sampled titles agree —
	// 1C, L2, "2-я линия", telecoms. The bare "поддержка"/"поддержки" below stay
	// `support`: they also name chat operators and the state-benefits desks.
	{"техподдержка", "it_support"},
	{"техподдержки", "it_support"},
	{"тех поддержки", "it_support"},
	{"техническая поддержка", "it_support"},
	{"технической поддержки", "it_support"},
	{"поддержки пользователей", "it_support"},
	{"хелпдеск", "it_support"},
	{"хелпдеска", "it_support"},
	// Disability and residential care is social service, not a help desk. "Direct
	// Support Professional" is the US title for the worker who supports a person with an
	// intellectual or developmental disability at home or in a group home, and "Support
	// Worker" (with "Personal/Family/Home/Bank Support Worker") is the UK and Canadian
	// title for the same care work. The bare "support" filed both as customer support.
	// The IT, administrative and customer-service spellings of "support worker" are
	// pinned first, so each desk keeps them.
	//
	// The abbreviation "DSP" is deliberately never an alias: in this catalogue it is as
	// often Amazon's Delivery Service Partner ("Amazon DSP Delivery Driver"), a
	// demand-side platform, a downstream-process operator or digital signal processing.
	{"administrative support", "administration"},
	{"customer support worker", "support"},
	{"technical support worker", "support"},
	{"direct support", "social_services"},
	{"support worker", "social_services"},
	{"support workers", "social_services"},
	{"customer service", "support"},
	// The desk phrases that name the customer side outright. The bare "support" is
	// declared last in the table, so without these a "Client Support & Benefits
	// Representative" read as sales through a word further down.
	{"customer support", "support"},
	{"client support", "support"},
	{"call center", "support"},
	{"call-центр", "support"},
	{"колл-центр", "support"},
	{"contact center", "support"},
	{"customer care", "support"},
	{"поддержка", "support"},
	{"поддержки", "support"},
	// customer_success (proactive post-sale: adoption/renewals) is distinct from the
	// reactive customer "support" above; "account manager" stays in sales.
	{"customer success", "customer_success"},
	{"client success", "customer_success"},
	{"customer onboarding", "customer_success"},
	{"onboarding specialist", "customer_success"},
	{"onboarding manager", "customer_success"},
	{"implementation specialist", "customer_success"},
	{"implementation consultant", "customer_success"},
	// NOT "onboarding engineer": unlike "implementation engineer" above (still a
	// customer-facing role, the technical sibling of "implementation
	// specialist"/"consultant"), "onboarding engineer" is genuinely dual-use — it is
	// also a real internal-platform title ("Developer Onboarding Engineer" builds
	// onboarding tooling for a company's own engineers), so a bare entry would
	// misfile that population the way a bare "growth"/"compliance" would elsewhere
	// in this file.
	{"implementation engineer", "customer_success"},
	{"renewals manager", "customer_success"},
	{"renewal manager", "customer_success"},
	{"менеджер по успеху клиентов", "customer_success"},
	{"менеджер по работе с клиентами", "customer_success"},
	{"специалист по адаптации клиентов", "customer_success"},
	// IT-company back-office roles. All are multi-word/anchored, so a functional title
	// ("Financial Analyst", "Operations Manager") names its function outright. hr
	// precedes operations so "People Operations Manager" → hr, not operations.
	{"recruiter", "recruiting"},
	{"tech recruiter", "recruiting"},
	{"technical recruiter", "recruiting"},
	{"it recruiter", "recruiting"},
	{"talent acquisition", "recruiting"},
	{"talent sourcer", "recruiting"},
	{"recruitment consultant", "recruiting"},
	{"recruitment specialist", "recruiting"},
	{"sourcer", "recruiting"},
	{"talent partner", "recruiting"},
	// Employer branding sells the company to candidates, not customers — the
	// recruiting-side twin of the marketing "brand manager" alias above, which this
	// phrase does not contain a substring of ("employer branding manager" has no
	// "brand manager" inside it), so it needs its own entry.
	{"employer branding", "recruiting"},
	{"рекрутер", "recruiting"},
	{"рекрутёр", "recruiting"},
	{"специалист по подбору персонала", "recruiting"},
	{"human resources", "hr"},
	{"hr manager", "hr"},
	{"hr generalist", "hr"},
	{"hr business partner", "hr"},
	{"hrbp", "hr"},
	{"people partner", "hr"},
	{"people operations", "hr"},
	{"people ops", "hr"},
	{"hr director", "hr"},
	{"head of people", "hr"},
	{"chro", "hr"},
	{"learning and development", "hr"},
	{"compensation and benefits", "hr"},
	{"менеджер по персоналу", "hr"},
	{"специалист по персоналу", "hr"},
	{"директор по персоналу", "hr"},
	{"эйчар", "hr"},
	{"chief financial officer", "finance"},
	{"cfo", "finance"},
	{"head of finance", "finance"},
	{"financial controller", "finance"},
	{"finance controller", "finance"},
	{"financial analyst", "finance"},
	{"finance analyst", "finance"},
	{"fp&a", "finance"},
	{"accountant", "finance"},
	{"accounting", "finance"},
	{"accounts payable", "finance"},
	{"accounts receivable", "finance"},
	{"bookkeeper", "finance"},
	{"payroll", "finance"},
	{"treasury", "finance"},
	{"tax accountant", "finance"},
	{"finance manager", "finance"},
	{"financial manager", "finance"},
	{"финансовый директор", "finance"},
	{"главный бухгалтер", "finance"},
	{"главбух", "finance"},
	{"бухгалтер", "finance"},
	{"финансовый аналитик", "finance"},
	{"казначей", "finance"},
	{"general counsel", "legal"},
	{"legal counsel", "legal"},
	{"corporate counsel", "legal"},
	{"legal manager", "legal"},
	{"legal assistant", "legal"},
	{"lawyer", "legal"},
	{"attorney", "legal"},
	{"paralegal", "legal"},
	{"contract manager", "legal"},
	{"contracts manager", "legal"},
	{"compliance officer", "legal"},
	{"compliance manager", "legal"},
	{"compliance analyst", "legal"},
	{"regulatory affairs", "legal"},
	{"data protection officer", "legal"},
	{"privacy officer", "legal"},
	{"data privacy officer", "legal"},
	{"data privacy manager", "legal"},
	{"contracts administrator", "legal"},
	{"contract administrator", "legal"},
	// Immigration practice: must precede the administration block ("Immigration
	// Assistant") and the case-manager spellings so neither steals this family. "immigration paralegal" is
	// listed for completeness — the bare "paralegal" entry above already
	// resolves it to `legal` — but every other entry here is load-bearing.
	{"immigration paralegal", "legal"},
	{"immigration specialist", "legal"},
	{"immigration assistant", "legal"},
	{"immigration consultant", "legal"},
	{"immigration case manager", "legal"},
	{"юрист", "legal"},
	{"юрисконсульт", "legal"},
	{"корпоративный юрист", "legal"},
	{"комплаенс", "legal"},
	{"chief operating officer", "operations"},
	{"coo", "operations"},
	{"chief of staff", "operations"},
	{"business operations", "operations"},
	{"biz ops", "operations"},
	{"operations manager", "operations"},
	{"operations analyst", "operations"},
	{"operations coordinator", "operations"},
	{"operations specialist", "operations"},
	{"ops manager", "operations"},
	{"head of operations", "operations"},
	{"office manager", "operations"},
	// The assistant family is `administration` (see that block below), but these
	// spellings must resolve HERE, at the precedence they always had: "Executive
	// Assistant to the General Manager" names a manager after the role, and the
	// manager aliases declared between this line and the administration block would
	// take it.
	{"executive assistant", "administration"},
	{"administrative assistant", "administration"},
	{"personal assistant", "administration"},
	{"ассистент руководителя", "administration"},
	{"помощник руководителя", "administration"},
	{"procurement", "operations"},
	{"procurement manager", "operations"},
	{"purchasing manager", "operations"},
	{"facilities manager", "operations"},
	{"facilities coordinator", "operations"},
	{"операционный директор", "operations"},
	{"операционный менеджер", "operations"},
	{"офис-менеджер", "operations"},
	{"специалист по закупкам", "operations"},
	{"закупщик", "operations"},
	{"business analyst", "business_analysis"},
	{"business systems analyst", "business_analysis"},
	{"business system analyst", "business_analysis"},
	{"systems analyst", "business_analysis"},
	{"system analyst", "business_analysis"},
	{"business process analyst", "business_analysis"},
	{"process analyst", "business_analysis"},
	{"requirements analyst", "business_analysis"},
	{"functional analyst", "business_analysis"},
	{"it business analyst", "business_analysis"},
	{"business analysis", "business_analysis"},
	{"бизнес-аналитик", "business_analysis"},
	{"бизнес аналитик", "business_analysis"},
	{"системный аналитик", "business_analysis"},
	{"аналитик требований", "business_analysis"},
	{"аналитик бизнес-процессов", "business_analysis"},
	{"developer advocate", "developer_relations"},
	{"developer relations", "developer_relations"},
	{"devrel", "developer_relations"},
	{"developer evangelist", "developer_relations"},
	{"technical evangelist", "developer_relations"},
	{"developer experience engineer", "developer_relations"},
	{"developer community manager", "developer_relations"},
	{"деврел", "developer_relations"},
	{"технический евангелист", "developer_relations"},
	// Community management is marketing's, but only once developer relations has had
	// its say: a "Community Manager, Developer Relations" runs a developer community,
	// not a brand's social presence. Sits below the DevRel block for that reason.
	{"community manager", "marketing"},
}
