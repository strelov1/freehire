package skilltag

// engineeringPhraseAliases1 is part 1 of 4 of engineeringPhraseAliases, in order (see dictionaries.go).
var engineeringPhraseAliases1 = []phraseAlias{
	{"c++", "cpp"}, {"c/c++", "cpp"},
	{"c#", "csharp"},
	{".net", "dotnet"}, {"asp.net", "dotnet"},
	{"node.js", "nodejs"}, {"node js", "nodejs"},
	{"next.js", "nextjs"},
	{"vue.js", "vue"},
	{"react native", "react-native"},
	{"react.js", "react"},
	{"objective-c", "objective-c"},
	{"ci/cd", "ci-cd"}, {"ci cd", "ci-cd"},
	{"c developer", "c"}, {"c programming", "c"}, {"ansi c", "c"},
	// Cyrillic 1С (the RU market's spelling) — the word pass can't see it ([a-z0-9] only), so it
	// resolves here as a strong phrase; the Latin "1c" is the gated word alias.
	{"1с", "1c"},
	{"machine learning", "machine-learning"},
	// additional phrases
	{"rest api", "rest"}, {"rest apis", "rest"},
	// The bare word "restful" is gated (a care home offers a restful night), so the
	// spelled-out form carries the real postings that say "RESTful API" rather than
	// "REST API" — it is the phrase, not the adjective, that names the style.
	{"restful api", "rest"}, {"restful apis", "rest"},
	{"soap api", "soap"}, {"soap apis", "soap"},
	{"aws s3", "s3"}, {"amazon s3", "s3"},
	{"github actions", "github-actions"},
	{"cloudformation", "cloudformation"},
	{"scikit-learn", "scikit-learn"},
	{"power bi", "powerbi"},
	{"new relic", "newrelic"},
	{"open telemetry", "opentelemetry"},
	{"f#", "fsharp"},
	{"visual basic", "vbnet"},
	{"three.js", "threejs"},
	{"web assembly", "wasm"},
	{"r language", "r"},
	// llm / genai multi-word + disambiguated phrases (the bare word would collide
	// with non-AI jobs, so it is only matched as a phrase).
	{"chroma db", "chromadb"},
	{"llama index", "llamaindex"},
	{"crew ai", "crewai"},
	{"semantic kernel", "semantic-kernel"},
	{"vertex ai", "vertex-ai"},
	{"aws bedrock", "aws-bedrock"}, {"amazon bedrock", "aws-bedrock"},
	// Hyphen/space variants collapse to one matcher (see phraseMatchers), so only the
	// space form is listed — "sentence-transformers"/"retrieval-augmented generation"
	// resolve via the separator-insensitive phrase match.
	{"sentence transformers", "sentence-transformers"},
	{"retrieval augmented generation", "rag"},
	// LLM-mined multi-word gaps (jobs.enrichment->skills). Multi-word or ambiguous
	// terms routed as phrases so the bare token can't misfire on non-tech jobs.
	// Unambiguous names for the frameworks whose bare word is now corroboration-gated
	// (see ambiguousWords) — these strong forms tag without needing a co-token.
	{"spring boot", "spring"}, {"spring framework", "spring"}, {"spring mvc", "spring"},
	{"ruby on rails", "ruby"}, {"ruby on rails", "rails"},
	{"express.js", "express"},
	{"azure devops", "azure-devops"},
	{"power apps", "powerapps"}, {"powerapps", "powerapps"},
	{"power automate", "power-automate"},
	{"distributed systems", "distributed-systems"},
	{"data modeling", "data-modeling"}, {"data modelling", "data-modeling"},
	{"data visualization", "data-visualization"}, {"data visualisation", "data-visualization"},
	{"prompt engineering", "prompt-engineering"},
	{"generative ai", "generative-ai"}, {"gen ai", "generative-ai"},
	{"deep learning", "deep-learning"},
	{"six sigma", "six-sigma"},
	// Lightcast-diffed gaps (security/qa/data). Ambiguous English words (burp, druid,
	// pinot, presto, iceberg) are emitted ONLY via a qualifying phrase, so a bare
	// non-tech use ("a druid", "pinot noir", "a loud burp") never tags — same doctrine
	// as c/r above.
	{"burp suite", "burp-suite"},
	{"robot framework", "robot-framework"},
	{"apache druid", "druid"},
	{"apache pinot", "pinot"},
	{"prestodb", "presto"}, {"presto db", "presto"},
	{"delta lake", "delta-lake"},
	{"apache iceberg", "iceberg"},
	// LLM-mined batch (jobs.enrichment->skills, freq >= 1500). Multi-word or
	// punctuated concepts the word pass can't see. Separator-insensitive, so only the
	// space form is listed; distinct word forms (warehouse/warehousing) each need an
	// entry. Variant spellings collapse to one canonical to keep the facet un-fragmented.
	{"data pipeline", "data-pipelines"}, {"data pipelines", "data-pipelines"},
	{"infrastructure as code", "infrastructure-as-code"},
	{"sql server", "sql-server"}, {"t-sql", "sql-server"},
	{"data science", "data-science"},
	{"data engineering", "data-engineering"},
	{"data warehouse", "data-warehousing"}, {"data warehousing", "data-warehousing"},
	{"cloud security", "cloud-security"},
	{"network security", "network-security"},
	{"active directory", "active-directory"},
	{"windows server", "windows-server"},
	{"google analytics", "google-analytics"},
	{"google cloud", "gcp"}, {"google cloud platform", "gcp"},
	{"cloud native", "cloud-native"},
	{"zero trust", "zero-trust"},
	{"penetration testing", "penetration-testing"},
	{"unit testing", "unit-testing"}, {"unit test", "unit-testing"},
	{"test automation", "test-automation"}, {"automated testing", "test-automation"},
	{"test driven development", "tdd"},
	{"a/b testing", "ab-testing"}, {"ab testing", "ab-testing"},
	{"vector database", "vector-databases"}, {"vector databases", "vector-databases"},
	{"computer vision", "computer-vision"},
	{"api design", "api-design"},
	{"embedded systems", "embedded-systems"},
	{"event driven architecture", "event-driven-architecture"}, {"event driven", "event-driven-architecture"},
	{"design patterns", "design-patterns"}, {"design pattern", "design-patterns"},
	{"version control", "version-control"},
	{"object oriented programming", "oop"}, {"object oriented design", "oop"},
	{"agentic ai", "agentic-ai"}, {"ai agents", "agentic-ai"},
	{"tcp/ip", "tcp-ip"}, {"tcp ip", "tcp-ip"},
	{"pl/sql", "plsql"},
	{"continuous integration", "ci-cd"}, {"continuous delivery", "ci-cd"},
	{"natural language processing", "nlp"},
	{"html5", "html"},
	{"css3", "css"},
	// web3 / crypto multi-word + punctuated
	{"smart contract", "smart-contracts"}, {"smart contracts", "smart-contracts"},
	{"ethers.js", "ethersjs"},
	// LLM-mined batch 2 (freq 500-1500) — multi-word concepts + products the word
	// pass can't see. Separator-insensitive, so only the space form is listed.
	// data / ml concepts
	{"data lake", "data-lake"},
	{"data mining", "data-mining"},
	{"data lineage", "data-lineage"},
	{"data ingestion", "data-ingestion"},
	{"metadata management", "metadata-management"},
	{"master data management", "master-data-management"},
	{"dimensional modeling", "dimensional-modeling"}, {"dimensional modelling", "dimensional-modeling"},
	{"time series", "time-series"},
	{"feature engineering", "feature-engineering"},
	{"reinforcement learning", "reinforcement-learning"},
	{"neural network", "neural-networks"}, {"neural networks", "neural-networks"},
	{"predictive modeling", "predictive-modeling"}, {"predictive modelling", "predictive-modeling"},
	{"predictive analytics", "predictive-analytics"},
	{"anomaly detection", "anomaly-detection"},
	{"recommendation systems", "recommendation-systems"}, {"recommendation system", "recommendation-systems"},
	{"conversational ai", "conversational-ai"},
	{"causal inference", "causal-inference"},
	{"semantic search", "semantic-search"},
	{"vector search", "vector-search"},
	{"model deployment", "model-deployment"},
	{"model evaluation", "model-evaluation"},
	{"distributed training", "distributed-training"},
	{"distributed computing", "distributed-computing"},
	{"image processing", "image-processing"},
	{"signal processing", "signal-processing"},
	{"workflow orchestration", "workflow-orchestration"},
	// security concepts
	{"threat hunting", "threat-hunting"},
	{"intrusion detection", "intrusion-detection"},
	{"vulnerability scanning", "vulnerability-scanning"},
	{"secure coding", "secure-coding"},
	{"secrets management", "secrets-management"},
	{"api security", "api-security"},
	{"container security", "container-security"},
	{"reverse engineering", "reverse-engineering"},
	{"iso 27001", "iso-27001"}, {"iso27001", "iso-27001"},
	{"soc 2", "soc-2"},
	{"mitre att&ck", "mitre-attack"}, {"mitre attack", "mitre-attack"},
	// embedded / systems
	{"embedded linux", "embedded-linux"},
	{"embedded software", "embedded-software"},
	{"sensor fusion", "sensor-fusion"},
	{"game development", "game-development"},
	// products / platforms (head word is generic, so route the full phrase)
	{"visual studio", "visual-studio"},
	{"microsoft access", "microsoft-access"}, {"ms access", "microsoft-access"},
	{"microsoft dynamics", "microsoft-dynamics"},
	{"microsoft fabric", "microsoft-fabric"},
	{"power platform", "power-platform"},
	{"power query", "power-query"},
	{"azure ad", "azure-ad"}, {"entra id", "entra-id"},
	{"azure functions", "azure-functions"},
	{"azure data factory", "azure-data-factory"},
	{"sales cloud", "sales-cloud"},
	{"salesforce marketing cloud", "salesforce-marketing-cloud"},
	// No {"sap s4hana"}: the bare "s4hana" alias declared with the mined batch below
	// matches inside it, so the two-word form is dead weight.
	{"s/4hana", "sap-s4hana"},
	{"sap mm", "sap-mm"},
	{"siemens nx", "siemens-nx"},
	{"premiere pro", "premiere-pro"},
	{"adobe analytics", "adobe-analytics"},
	{"google tag manager", "google-tag-manager"},
	{"monday.com", "monday-com"},
	{"claude code", "claude-code"},
	{"unreal engine", "unreal-engine"},
	{"hyper-v", "hyper-v"}, {"hyper v", "hyper-v"},
	{"sd-wan", "sd-wan"},
	// LLM-mined batch 3 — multi-word broad concepts (see the wordAliases batch 3 note).
	{"data analytics", "data-analytics"},
	{"data governance", "data-governance"},
	{"data quality", "data-quality"},
	{"e commerce", "ecommerce"},
	// IT-company role skills (expand-role-taxonomy) — multi-word phrases only, so they
	// stay precision-safe (no false match inside a larger word). Seed set; expand later.
	//
	// Two kinds of term are deliberately absent, because a job description is not only
	// its requirements:
	//   - Boilerplate sections. The pay-transparency block ("Compensation and
	//     Benefits"), the GDPR notice ("Data Privacy Notice"), the ATS footer
	//     ("Powered by Greenhouse", "Apply via Lever") and the culture blurb
	//     ("employee engagement", "due diligence") appear for EVERY role, so matching
	//     them tags the whole corpus instead of describing the job.
	//   - ATS product names whose bare token is an English word (lever, workday) —
	//     "a key lever", "a flexible workday". Same doctrine as burp/druid above: such
	//     a name may only return through a qualifying phrase, never as a bare token.
	//
	// The recruiting/HR/finance/legal/operations/customer-success/sales/support half of
	// this seed set lives in professionalPhraseAliases below — same matching, separate
	// list.
	// business analysis
	{"requirements gathering", "requirements-gathering"},
	{"requirements elicitation", "requirements-elicitation"},
	{"process modeling", "process-modeling"},
	{"gap analysis", "gap-analysis"},
	{"user stories", "user-stories"},
	{"acceptance criteria", "acceptance-criteria"},
	// solutions / pre-sales
	{"pre-sales", "pre-sales"}, {"presales", "pre-sales"},
	{"sales engineering", "sales-engineering"},
	{"proof of concept", "proof-of-concept"},
	{"technical discovery", "technical-discovery"},
	{"solution design", "solution-design"},
	// developer relations
	{"developer advocacy", "developer-advocacy"},
	{"technical evangelism", "technical-evangelism"},
	{"community management", "community-management"},
	{"developer experience", "developer-experience"},
	// technical writing
	{"technical writing", "technical-writing"},
	{"api documentation", "api-documentation"},
	{"docs-as-code", "docs-as-code"}, {"docs as code", "docs-as-code"},
	{"structured authoring", "structured-authoring"},
	{"information architecture", "information-architecture"},
	{"madcap flare", "madcap-flare"},
	// design — the multi-word tools and the named practices. They sit with the
	// engineering vocabulary (not the professional one) for the same reason technical
	// writing does: design is craft a technical employer posts.
	// "adobe illustrator"/"adobe indesign" need no phrase: the word pass already sees
	// the product name inside them. "xd" alone is far too short to be safe.
	{"adobe xd", "adobe-xd"},
	// "adobe after effects" only: bare "after effects" is clinical prose, and
	// healthcare is the largest non-technical mass this dictionary runs over.
	{"adobe after effects", "after-effects"},
	// No "design system"/"design systems": that is the ordinary verb phrase of every
	// backend, embedded and architecture posting ("you will design systems that scale",
	// "design system architecture"). A phrase match is always strong, so tagging it
	// would not merely mislabel those postings — it would lift the corroboration gate
	// off every weak word beside it, which is how a maintenance posting once came back
	// tagged {react, sap}. The practice is still reachable through the job title
	// ("Design Systems Engineer" → the design category and the design_engineer role).
	{"design thinking", "design-thinking"},
	{"user research", "user-research"}, {"ux research", "user-research"},
	{"usability testing", "usability-testing"},
	{"interaction design", "interaction-design"},
	{"ux design", "ux-design"}, {"user experience design", "ux-design"},
	// No "visual design" either: "visual design of the store" is retail prose, and a
	// phrase always matches strong, so it would corroborate the gated words beside it.
	{"motion design", "motion-design"},
	{"motion graphics", "motion-graphics"},
	{"user flows", "user-flows"},
	// media production — the multi-word products and the two practices. Both British
	// and American spellings of the grade are listed: neither is the odd one out in a
	// catalogue this international. "final cut" without "pro" is left out — it is
	// ordinary prose ("the final cut of the budget").
	{"davinci resolve", "davinci-resolve"}, {"da vinci resolve", "davinci-resolve"},
	{"final cut pro", "final-cut-pro"},
	{"cinema 4d", "cinema-4d"},
	// Substance 3D Painter and Substance 3D Designer are two products with two job
	// descriptions — texture painting and procedural material authoring. Folding the
	// second into the first would have the glossary render "substance designer" as a
	// spelling of Painter, which is the dictionary guessing.
	{"substance painter", "substance-painter"},
}
