package classify

// coreCategoryTable6 is part 6 of 17 of coreCategoryTable; the parts concatenate in order.
var coreCategoryTable6 = []aliasEntry{
	{"ruby developer", "software_engineering"},
	{"rails developer", "software_engineering"},
	{"c# developer", "software_engineering"},
	{"c++ developer", "software_engineering"},
	{"node developer", "software_engineering"},
	{"nodejs developer", "software_engineering"},
	{"node.js developer", "software_engineering"},
	{"salesforce developer", "software_engineering"},
	{"sharepoint developer", "software_engineering"},
	{"database developer", "software_engineering"},
	{"rpa developer", "software_engineering"},
	{"erp developer", "software_engineering"},
	{"sap developer", "software_engineering"},
	// MES is the manufacturing execution system; the role writes and integrates that
	// software, so it is software engineering with a plant for a customer.
	{"mes developer", "software_engineering"},
	{"oracle developer", "software_engineering"},
	{"abap developer", "software_engineering"},
	{"wordpress developer", "software_engineering"},
	{"drupal developer", "software_engineering"},
	{"magento developer", "software_engineering"},
	{"shopify developer", "software_engineering"},
	// The `engineer` spelling of everything above, and the `developer` spelling of the
	// software titles that only had an `engineer` one. Which noun an employer reaches for
	// is a habit — "Python Developer" and "Python Engineer" are the same job — and the
	// list carried only one side of 38 of them.
	//
	// The cost of the omission is not a missing facet. A title this dictionary cannot
	// read gets no category AND no is_tech; EnqueuePendingJobs gates enrichment on
	// `is_tech IS TRUE`, so the LLM never sees the posting and never supplies the
	// category the dictionary missed; and search.CategoryUnresolved then hides it
	// FOREVER rather than until the next enrichment cycle. Measured on prod 2026-09-16:
	// "senior java engineer" alone was 233 open postings inside that loop.
	//
	// TestBothSpellingsOfACraftResolveTheSame derives the pairs from this table, so a
	// technology added under one noun from now on fails the build rather than quietly
	// losing its postings.
	{"python engineer", "software_engineering"},
	{"java engineer", "software_engineering"},
	{"javascript engineer", "software_engineering"},
	{"typescript engineer", "software_engineering"},
	{".net engineer", "software_engineering"},
	{"dotnet engineer", "software_engineering"},
	{"php engineer", "software_engineering"},
	{"ruby engineer", "software_engineering"},
	{"rails engineer", "software_engineering"},
	{"c# engineer", "software_engineering"},
	{"c++ engineer", "software_engineering"},
	{"node engineer", "software_engineering"},
	{"nodejs engineer", "software_engineering"},
	{"node.js engineer", "software_engineering"},
	{"abap engineer", "software_engineering"},
	{"app engineer", "software_engineering"},
	{"game engineer", "software_engineering"},
	{"mainframe engineer", "software_engineering"},
	{"sharepoint engineer", "software_engineering"},
	{"rpa engineer", "software_engineering"},
	{"erp engineer", "software_engineering"},
	{"sap engineer", "software_engineering"},
	{"mes engineer", "software_engineering"},
	{"oracle engineer", "software_engineering"},
	{"wordpress engineer", "software_engineering"},
	{"drupal engineer", "software_engineering"},
	{"magento engineer", "software_engineering"},
	{"shopify engineer", "software_engineering"},
	{"integration developer", "software_engineering"},
	{"it developer", "software_engineering"},
	{"founding developer", "software_engineering"},
	{"ai-native developer", "software_engineering"},
	{"ai native developer", "software_engineering"},
	// "Software Engineering <anything>" was the same omission in one letter: matching is
	// whole-word, so `software engineer` never occurs inside "Software Engineering
	// Intern" and that title resolved to nothing while "Software Engineer Intern"
	// resolved fine — 192 open postings apart on a gerund. It sits AFTER
	// `engineering manager` far above, which resolves "Software Engineering Manager"
	// first — to this same category.
	{"software engineering", "software_engineering"},
	// The `developer` spelling of the two AI entries below, which had only `engineer`.
	// "AI Developer" alone was 152 open postings resolving to nothing.
	{"ai developer", "ai_engineering"},
	{"ml developer", "ml_ai"},
	// "Member of Technical Staff" reads as software on the same evidence tech.go
	// cites (294/300 sampled prod postings software or AI). "Founding Engineer"
	// is the early-startup twin of the same generalist population.
	{"member of technical staff", "software_engineering"},
	{"member of the technical staff", "software_engineering"},
	{"founding engineer", "software_engineering"},
	// "AI-native"/"AI-enabled" describe the toolchain, not the discipline (same
	// reasoning as tech.go's techTitleTerms entry for these).
	{"ai-native engineer", "software_engineering"},
	{"ai native engineer", "software_engineering"},
	// software_engineering, non-English forms. Same doctrine as the English block
	// above: only software- or language-ANCHORED phrases, never a bare
	// developer/engineer/programmer noun. Prod titles show the bare noun is
	// genuinely dangerous in every language sampled, not just English — "SPS-
	// Programmierer"/"Roboterprogrammierer" (DE) and "Programador CNC" (ES) are
	// industrial/machine programming, not software, and Spanish/Portuguese
	// "desarrollador/desenvolvedor" alone also names a real-estate developer.
	// Feminine/inclusive forms are listed where prod data showed them in use
	// ("Engenheira de Software", "Pessoa Desenvolvedora"); an exhaustive sweep of
	// every language's gendered and inclusive spellings is future work, not done
	// here.
	//
	// Hungarian. The bare noun is "fejlesztő", and on the open catalogue it names
	// materials, supplier, process, product, business, operations and training
	// development ("Anyagfejlesztő", "Beszállítófejlesztő mérnök",
	// "Folyamatfejlesztő", "Termékfejlesztő szakértő", "Üzletfejlesztési menedzser",
	// "Működésfejlesztési szakértő", "Képzés-fejlesztési gyakornok") alongside the
	// software roles. Hungarian also writes closed compounds where English writes two
	// words, so a compound never contains its spaced twin on a word boundary. Where
	// prod showed both spellings in use, both are listed; the language is
	// agglutinative and an exhaustive sweep of its plurals, hyphenations and case
	// endings is future work, not done here — the same stance this file takes on
	// gendered forms above. That is why "java fejlesztők" and "odoo-fejlesztő" have
	// entries and the other twelve entries' plurals do not: those are the surface
	// forms the catalogue actually carries.
	{"szoftverfejlesztő", "software_engineering"},
	{"szoftver fejlesztő", "software_engineering"},
	{"szoftvermérnök", "software_engineering"},
	{"software mérnök", "software_engineering"},
	{"alkalmazásfejlesztő", "software_engineering"},
	{"alkalmazás fejlesztő", "software_engineering"},
	{"webfejlesztő", "software_engineering"},
	{"java fejlesztő", "software_engineering"},
	// The plural surface form does not contain the singular as a whole word.
	{"java fejlesztők", "software_engineering"},
	{"php fejlesztő", "software_engineering"},
	{"python fejlesztő", "software_engineering"},
	{"node.js fejlesztő", "software_engineering"},
	{".net fejlesztő", "software_engineering"},
	{"c# fejlesztő", "software_engineering"},
	{"adatbázis fejlesztő", "software_engineering"},
	{"abap fejlesztő", "software_engineering"},
	{"pega fejlesztő", "software_engineering"},
	{"odoo fejlesztő", "software_engineering"},
	{"odoo-fejlesztő", "software_engineering"},
	//
	// Spanish.
	{"desarrollador de software", "software_engineering"},
	{"desarrolladora de software", "software_engineering"},
	{"ingeniero de software", "software_engineering"},
	{"ingeniera de software", "software_engineering"},
	{"desarrollador java", "software_engineering"},
	{"desarrolladora java", "software_engineering"},
	// Portuguese.
	{"desenvolvedor de software", "software_engineering"},
	{"desenvolvedora de software", "software_engineering"},
	{"engenheiro de software", "software_engineering"},
	{"engenheira de software", "software_engineering"},
	{"desenvolvedor java", "software_engineering"},
	{"desenvolvedora java", "software_engineering"},
	// French. No bare "développeur" here: it also names a real-estate, regional-economic
	// or business developer in French job titles. The French fallback (french.go) reads
	// the bare noun only after vetoing those ("Développeur foncier", "Développeur
	// commercial"), so it never reaches an English or Dutch title.
	{"ingénieur logiciel", "software_engineering"},
	{"développeur java", "software_engineering"},
	{"développeur salesforce", "software_engineering"},
	// German. Compound, hyphenated, and spaced forms are three different strings
	// to this matcher — a hyphen and a space both break a compound differently,
	// so each spelling needs its own alias (same trap "middle-east" documents).
	{"softwareentwickler", "software_engineering"},
	{"software-entwickler", "software_engineering"},
	{"software entwickler", "software_engineering"},
	{"entwickler software", "software_engineering"},
	{"softwareingenieur", "software_engineering"},
	{"java entwickler", "software_engineering"},
	{"java-entwickler", "software_engineering"},
	{"abap entwickler", "software_engineering"},
	// Polish.
	{"inżynier oprogramowania", "software_engineering"},
	{"deweloper oprogramowania", "software_engineering"},
	{"programista .net", "software_engineering"},
	{"programista python", "software_engineering"},
	// Italian.
	{"sviluppatore software", "software_engineering"},
	{"ingegnere del software", "software_engineering"},
	{"ingegnere software", "software_engineering"},
	// The IT tail: titles the catalogue carries in volume that this dictionary had no
	// word for at all. Measured on prod 2026-09-02, 47.5% of open postings reached the
	// search index with no role, and EVERY one of them had an empty category — so the
	// gap is here, not in roletag.
	//
	// The whole block is declared late on purpose. Nothing in it resolves to anything
	// today, so a late declaration cannot take a row from a category that already
	// works; the ordering that matters is only WITHIN the block.
	//
	// The four industrial namesakes come first and are BLIND. "Systems Engineer" is the
	// single largest unresolved IT title here (1440 open for the exact spelling), so the
	// bare alias has to exist — and without these four above it, every control, power,
	// electrical and quality engineer in the catalogue would be swept into software.
	// The sentinel keeps them resolving to nothing, which is what they do today; they
	// belong to an industrial taxonomy this change does not introduce.
	{"control systems engineer", categoryNone},
	{"power systems engineer", categoryNone},
	{"electrical systems engineer", categoryNone},
	{"quality systems engineer", categoryNone},
	// The same four under `developer`, and for the same reason: the bare
	// "systems developer" below would otherwise sweep every one of them into software,
	// exactly as the bare "systems engineer" would have. They are listed even though no
	// such title is common, because the blindness has to be declared where the sweep
	// happens — the twin block above is the only thing that makes this one obvious.
	{"control systems developer", categoryNone},
	{"power systems developer", categoryNone},
	{"electrical systems developer", categoryNone},
	{"quality systems developer", categoryNone},
	// Then the qualified IT spellings, each naming its own discipline.
	{"linux systems engineer", "devops"},
	{"cyber systems engineer", "security"},
	{"software systems engineer", "software_engineering"},
	{"it systems engineer", "software_engineering"},
	{"software systems developer", "software_engineering"},
	{"it systems developer", "software_engineering"},
	// The generic technical analyst titles, ~2.2k live postings between them. They were
	// reached only through the bare "analyst" fall-through, which called them data
	// analysts; the software_engineering bucket is the same answer this file gives every
	// title it confirms is IT work without naming a sub-discipline.
	//
	// "it analyst" is why the boundary match matters rather than a substring one: 982
	// live "Credit Analyst" postings CONTAIN it, and a substring match would file every
	// one of them as IT staff.
	{"it analyst", "software_engineering"},
	{"technology analyst", "software_engineering"},
	{"technical analyst", "software_engineering"},
	{"software analyst", "software_engineering"},
	// Both surface forms: the plural does not contain the singular on a word boundary,
	// and prod carries far more of the plural (healthcare's Epic/Cerner analysts).
	{"application analyst", "software_engineering"},
	{"applications analyst", "software_engineering"},
	// The bare form closes the family. `software_engineering` and not `devops`: the
	// population left after the four blind spellings is mixed between infrastructure
	// and generalist software work, and the generic bucket is the honest answer where
	// devops would be a guess.
	{"systems engineer", "software_engineering"},
	{"system engineer", "software_engineering"},
	{"systems developer", "software_engineering"},
	{"system developer", "software_engineering"},

	// Vendor platforms. Naming an enterprise product states the discipline as surely
	// as naming a language does. "Salesforce Developer" and "SAP Developer" already
	// resolved through the bare "developer" alias; the administrator and consultant
	// spellings did not.
	{"servicenow developer", "software_engineering"},
	{"servicenow engineer", "software_engineering"},
	{"servicenow administrator", "devops"},
	{"salesforce administrator", "software_engineering"},
	{"salesforce engineer", "software_engineering"},
	{"salesforce consultant", "software_engineering"},
	{"mainframe developer", "software_engineering"},
	{"oracle dba", "devops"},
	{"sharepoint administrator", "devops"},
	{"tableau developer", "data_analytics"},

	// Infrastructure and end-user IT. "IT Specialist"/"IT Technician" go to `support`
	// rather than `devops`: "IT Support Specialist" already resolves there, and they
	// name the same desk — splitting one job across two facets on a dropped word is
	// the defect the design split existed to fix.
	{"data center technician", "devops"},
	{"data center engineer", "devops"},
	{"release engineer", "devops"},
	{"cloud operations engineer", "devops"},
	{"cloud migration engineer", "devops"},
	{"network operations engineer", "devops"},
	{"network specialist", "network_engineering"},
	{"network technician", "network_engineering"},
	// 256 live postings, previously read as data analysts.
	{"network analyst", "network_engineering"},
	{"it specialist", "it_support"},
	{"it technician", "it_support"},
	// The integration family. None of these contains "systems engineer" as consecutive
	// words, so their order against that alias does not matter.
	{"integration engineer", "software_engineering"},
}
