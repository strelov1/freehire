package skilltag

// engineeringPhraseAliases2 is part 2 of 4 of engineeringPhraseAliases, in order (see dictionaries.go).
var engineeringPhraseAliases2 = []phraseAlias{
	{"substance designer", "substance-designer"},
	{"color grading", "color-grading"}, {"colour grading", "color-grading"},
	{"video editing", "video-editing"},
	// Declared as phrases, not words, so nonCorroboratingPhrases can reach them — that
	// map is consulted on the phrase pass only, and the word pass has no equivalent
	// tier. A single-token phrase alias matches exactly as the word pass would.
	{"storyboarding", "storyboarding"}, {"storyboards", "storyboarding"},
	// mechanical CAD phrases (the single-token products live in wordAliases).
	{"3ds max", "3ds-max"},
	{"fusion 360", "fusion-360"},
	{"civil 3d", "civil-3d"},
	// No "solid edge": "a solid edge over the competition" is sales prose, and the
	// CAD product is rare enough here that the phrase costs more than it recalls.
	{"autodesk inventor", "autodesk-inventor"},
	{"ptc creo", "creo"}, {"creo parametric", "creo"},
	{"cadence virtuoso", "cadence-virtuoso"},

	// LLM-mined batch 4 — the phrase half. Every term here was mined as a bare token
	// too, and every one is routed through a phrase instead because the bare form has
	// a reading this catalogue carries in bulk.
	// .net server stack — the word pass cannot see a dotted or two-word product name.
	{".net core", "dotnet"}, {"asp.net core", "dotnet"},
	{"entity framework", "dotnet"},
	// AWS/Azure/SAP services named after ordinary words. "glue" is adhesive on a
	// manufacturing posting, "synapse" is neuroscience on a clinical one, and bare
	// "hana" is a given name — the vendor prefix is what makes them products.
	{"aws glue", "aws-glue"},
	{"azure synapse", "azure-synapse"}, {"synapse analytics", "azure-synapse"},
	// HANA is the database and S/4HANA the ERP suite on top of it, so they are two
	// canonicals rather than one. The qualified spellings of S/4HANA are declared with
	// the other SAP products above; only the unseparated form was missing.
	{"sap hana", "sap-hana"}, {"s4hana", "sap-s4hana"},
	// Network security vendors named after places and plants. "Palo Alto" is the city
	// in every Bay Area address block and "juniper" is a shrub; the qualified product
	// name is the only safe route.
	{"palo alto networks", "palo-alto-networks"},
	{"juniper networks", "juniper"}, {"junos", "juniper"},
	// Compliance frameworks and security certifications. The acronyms are coined, so
	// tagging them was never the risk — VOUCHING was. A strong match lifts the
	// corroboration gate off every weak word in the same text, and these appear in the
	// prose of postings that are not technical at all: "HIPAA training required" is on
	// most of a healthcare catalogue, ISO 9001 on most of a manufacturing one. As word
	// aliases they turned a nurse's "pick up the slack" into `slack`. They are phrases
	// now, and every canonical below is listed in nonCorroboratingPhrases.
	//
	// The privacy and trade-control regimes sit in the PROFESSIONAL list further down
	// instead: a posting whose only match is HIPAA or GDPR is a compliance role.
	{"nist", "nist"}, {"fedramp", "fedramp"}, {"cmmc", "cmmc"},
	{"cissp", "cissp"}, {"cism", "cism"}, {"ccna", "ccna"}, {"ccnp", "ccnp"},
	// "PCI" alone is the bus, "SOC" a system-on-chip or a security operations centre,
	// "ISO" the organisation (and a camera setting) — the versioned spelling is what
	// disambiguates each of these.
	{"pci dss", "pci-dss"},
	{"soc2", "soc-2"},
	{"iso 9001", "iso-9001"}, {"iso9001", "iso-9001"},
	{"iso 13485", "iso-13485"},
	{"iso 26262", "iso-26262"},
	{"as9100", "as9100"},
	// Silicon and simulation terms whose bare acronym belongs to another industry
	// this catalogue carries in bulk: ASIC is the Australian Securities and Investments
	// Commission, RTL is right-to-left text (and React Testing Library), CFD is a
	// contract for difference, and KVM is the switch in an IT-support closet.
	{"asic design", "asic"}, {"asic verification", "asic"}, {"asic development", "asic"},
	{"rtl design", "rtl"}, {"rtl verification", "rtl"}, {"rtl coding", "rtl"},
	{"computational fluid dynamics", "cfd"}, {"cfd simulation", "cfd"}, {"cfd analysis", "cfd"},
	{"kvm hypervisor", "kvm"}, {"kvm virtualization", "kvm"}, {"kvm virtualisation", "kvm"},
	// Industrial automation. "PLC" is the British company suffix — "… PLC" closes a
	// legal entity name in a third of UK postings — so the controller returns only
	// through what an engineer actually writes beside it. "CAN" as a bare token is
	// hopeless for the obvious reason.
	{"plc programming", "plc"}, {"ladder logic", "plc"}, {"allen bradley", "plc"},
	{"can bus", "can-bus"},
	// "PCB" is also polychlorinated biphenyl, which this catalogue meets on every
	// environmental-remediation posting.
	{"pcb design", "pcb"}, {"pcb layout", "pcb"},
	{"gd&t", "gd-t"}, {"geometric dimensioning and tolerancing", "gd-t"},
	// Shop-floor vocabulary. These ARE the skill on a machinist, technician or
	// assembler posting, so they must tag on their own — but they are declared as
	// phrases and listed in nonCorroboratingPhrases so they never vouch for the gated
	// design and software words beside them. A CNC operator who "reads wireframes and
	// 2D drawings" is not doing UX, and as a strong word alias `cnc` said he was.
	{"cnc", "cnc"},
	{"soldering", "soldering"},
	{"oscilloscope", "oscilloscope"}, {"oscilloscopes", "oscilloscope"},
	// Engineering software named after an animal, a season, a superlative and two
	// letters: rhino, primavera (PT/ES "spring"), maximo ("máximo"). Siemens NX is
	// already declared with the other vendor products above.
	{"rhino 3d", "rhino"}, {"rhinoceros 3d", "rhino"},
	{"primavera p6", "primavera-p6"},
	{"ibm maximo", "maximo"},
	{"lean six sigma", "six-sigma"},
	// AI tooling. The model names are all ordinary nouns or given names — Claude,
	// Gemini, Copilot (an aviator) — so each is reachable only as a qualified product.
	// The unseparated "huggingface" is already a word alias, so only the spaced form
	// needs a phrase, and it resolves to that same canonical: one facet value, not two.
	// "claude" is the model and "claude-code" the CLI, declared with the other vendor
	// products above — different things, so the phrases here deliberately skip it.
	{"hugging face", "huggingface"},
	{"fine tuning", "fine-tuning"},
	{"large language models", "llm"}, {"large language model", "llm"},
	{"github copilot", "github-copilot"},
	{"microsoft copilot", "microsoft-copilot"}, {"copilot studio", "microsoft-copilot"},
	{"google gemini", "gemini"}, {"gemini api", "gemini"},
	{"anthropic claude", "claude"}, {"claude api", "claude"},
	{"deep seek", "deepseek"},
	{"lite llm", "litellm"},
	{"sg lang", "sglang"},
	{"duck db", "duckdb"},
	// batch 5 (2026-10 enrichment mining). Multi-word product and practice names.
	// Each phrase emits its canonical BESIDE whatever word its parts already tag
	// ("api gateway" still tags api) — phrases do not consume text.
	{"api gateway", "api-gateway"}, {"api testing", "api-testing"},
	{"integration testing", "integration-testing"},
	{"react testing library", "react-testing-library"},
	{"threat modeling", "threat-modeling"}, {"threat modelling", "threat-modeling"},
	{"threat intelligence", "threat-intelligence"},
	{"vulnerability management", "vulnerability-management"},
	{"domain-driven design", "domain-driven-design"}, {"domain driven design", "domain-driven-design"},
	{"clean architecture", "clean-architecture"},
	{"hexagonal architecture", "hexagonal-architecture"},
	{"medallion architecture", "medallion-architecture"},
	{"data mesh", "data-mesh"},
	{"big data", "big-data"},
	{"unity catalog", "unity-catalog"},
	{"control-m", "control-m"},
	{"signal integrity", "signal-integrity"},
	{"model-based systems engineering", "mbse"}, {"model based systems engineering", "mbse"},
	{"finite element analysis", "fea"},
	// cloud-vendor services, reachable only qualified: "cloud run", "logic apps" and
	// "service bus" are generic words on their own.
	{"azure monitor", "azure-monitor"},
	{"application insights", "azure-application-insights"},
	{"azure service bus", "azure-service-bus"},
	{"azure logic apps", "azure-logic-apps"}, {"logic apps", "azure-logic-apps"},
	{"arm templates", "arm-templates"},
	{"cosmos db", "cosmos-db"}, {"cosmosdb", "cosmos-db"},
	{"data factory", "azure-data-factory"},
	{"microsoft purview", "microsoft-purview"},
	{"microsoft sentinel", "microsoft-sentinel"},
	{"microsoft defender", "microsoft-defender"}, {"defender for endpoint", "microsoft-defender"},
	// Bare "autopilot" is Tesla's too, and "workspace one" reads "a shared workspace one day a week".
	{"windows autopilot", "windows-autopilot"},
	{"vmware workspace one", "workspace-one"}, {"omnissa workspace one", "workspace-one"}, {"workspace one uem", "workspace-one"},
	{"exchange online", "microsoft-exchange"},
	{"dynamics 365", "microsoft-dynamics"},
	{"google cloud run", "cloud-run"}, {"cloud run", "cloud-run"},
	{"cloud composer", "cloud-composer"},
	{"aws step functions", "step-functions"},
	{"ibm mq", "ibm-mq"},
	{"sap fiori", "sap-fiori"}, {"sap btp", "sap-btp"},
	{"oracle hyperion", "oracle-hyperion"},
	{"adobe experience manager", "adobe-experience-manager"},
	{"cisco meraki", "cisco-meraki"},
	{"rest assured", "rest-assured"},
	{"lightning web components", "lwc"},
	{"argo cd", "argocd"},
	{"vb.net", "vbnet"},
	{"ms sql", "sql-server"},
	{"java ee", "j2ee"},
	{"relational database", "rdbms"}, {"relational databases", "rdbms"},
	{"data lakes", "data-lake"},
	{"cyber security", "cybersecurity"},
	{"identity and access management", "iam"}, {"identity management", "iam"},
	// "PCB" is phrase-routed for polychlorinated biphenyl; the assembly form has no
	// such reading.
	{"pcba", "pcb"},
	// Discipline and office-tool vocabulary that a non-technical posting names in
	// passing — a PM's "MS Project", a planner's FMEA, a designer's Creative Suite.
	// Real enough to tag, never trustworthy enough to vouch; every canonical here is in
	// nonCorroboratingPhrases.
	{"microsoft project", "microsoft-project"}, {"ms project", "microsoft-project"},
	{"business intelligence", "business-intelligence"},
	{"fmea", "fmea"},
	{"design of experiments", "design-of-experiments"},
	{"design for manufacturing", "design-for-manufacturing"}, {"design for manufacturability", "design-for-manufacturing"},
	{"adobe creative suite", "adobe-creative-suite"},
	{"clickup", "clickup"},
	{"lucidchart", "lucidchart"},
	{"midjourney", "midjourney"},
	{"helpdesk", "help-desk"},
	{"dmaic", "six-sigma"},
	// batch 6 (2026-10-25, German market). "SPS" is German for PLC; the bare acronym is
	// left out (it is also a German abbreviation in care and social-work postings), so
	// the controller comes back through what an automation technician writes. TIA Portal
	// and STEP 7 are Siemens' PLC environment and SIMATIC its PLC line. Sinumerik and a
	// Heidenhain control are CNC controls; bare "Heidenhain" is an employer in its own
	// right, so only the control is read.
	{"sps programmierung", "plc"}, {"sps steuerung", "plc"}, {"sps steuerungen", "plc"},
	{"speicherprogrammierbare steuerung", "plc"}, {"speicherprogrammierbare steuerungen", "plc"},
	{"tia portal", "plc"}, {"siemens s7", "plc"}, {"simatic s7", "plc"}, {"simatic", "plc"},
	{"sinumerik", "cnc"}, {"siemens 840d", "cnc"},
	{"heidenhain steuerung", "cnc"}, {"heidenhain steuerungen", "cnc"},
	// Tools an electrician, a site manager or a draughtsperson is asked for: real and
	// specific, so they tag on their own, but listed in nonCorroboratingPhrases so they
	// never vouch for a gated word. Measured on the crawl: as strong word aliases, "iTWO"
	// on a site manager's posting vouched for "julia" (the contact, "Frau Julia …").
	{"eplan", "eplan"}, {"knx", "knx"}, {"itwo", "itwo"},
	{"archicad", "archicad"}, {"allplan", "allplan"},
	// batch 7 (2026-10-25, Dutch market). Bare "pega" is a Spanish and Portuguese verb,
	// "tekton" a hand-tool brand and "business central" could be prose, so each comes back
	// only bound to its context. HASTUS is a transit planner's scheduling tool and does not
	// vouch, nor does TOPdesk. NIS2 and NEN 7510 are regimes a posting is subject to: they tag, and never
	// vouch (nonCorroboratingPhrases), as the compliance batch before them.
	{"pega platform", "pega"}, {"pega developer", "pega"}, {"pega ontwikkelaar", "pega"},
	{"pega csa", "pega"}, {"pega cssa", "pega"}, {"pega lsa", "pega"}, {"pega bpm", "pega"},
	{"tekton pipelines", "tekton"}, {"tekton pipeline", "tekton"},
	{"business central", "microsoft-dynamics"},
	{"data vault", "data-vault"}, {"tabular editor", "tabular-editor"},
	{"apache camel", "apache-camel"},
	{"hastus", "hastus"}, {"topdesk", "topdesk"},
	{"nis2", "nis2"}, {"nis 2", "nis2"}, {"nen 7510", "nen-7510"}, {"nen7510", "nen-7510"},
	// batch 8 (2026-10-27, Spanish market). The Spanish name of an English canonical, each
	// written with and without its accents, as postings write both. Every one is a
	// multi-word term of the craft; the plain words in them ("datos", "pruebas",
	// "calidad") stay out.
	{"api rest", "rest"}, {"apis rest", "rest"}, {"api restful", "rest"},
	{"apis restful", "rest"}, {"servicios rest", "rest"},
	{"integración continua", "ci-cd"}, {"integracion continua", "ci-cd"},
	// Not "entrega continua": a delivery driver's posting promises it too.
	{"despliegue continuo", "ci-cd"},
	{"infraestructura como código", "infrastructure-as-code"},
	{"infraestructura como codigo", "infrastructure-as-code"},
	{"pruebas unitarias", "unit-testing"}, {"tests unitarios", "unit-testing"},
	{"test unitarios", "unit-testing"},
	{"pruebas automatizadas", "test-automation"}, {"automatización de pruebas", "test-automation"},
	{"automatizacion de pruebas", "test-automation"},
	{"control de versiones", "version-control"},
	{"patrones de diseño", "design-patterns"}, {"patrones de diseno", "design-patterns"},
	{"arquitectura hexagonal", "hexagonal-architecture"},
	{"arquitectura limpia", "clean-architecture"},
	{"diseño dirigido por el dominio", "domain-driven-design"},
	{"diseno dirigido por el dominio", "domain-driven-design"},
	{"desarrollo dirigido por pruebas", "tdd"},
	{"sistemas distribuidos", "distributed-systems"},
}
