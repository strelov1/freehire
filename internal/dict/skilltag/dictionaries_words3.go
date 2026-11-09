package skilltag

// wordAliases3 is part 3 of 3 of wordAliases (see dictionaries.go).
var wordAliases3 = map[string]string{
	"jboss":    "jboss",
	"weblogic": "weblogic",
	"jsp":      "jsp",
	"jms":      "jms",
	"jdbc":     "jdbc",
	"jcl":      "jcl",
	"xslt":     "xslt",
	"wpf":      "wpf",
	"iis":      "iis",
	// apple / frontend build
	"uikit":      "uikit",
	"xcode":      "xcode",
	"scss":       "scss",
	"npm":        "npm",
	"websocket":  "websockets",
	"websockets": "websockets",
	// data platforms and pipelines
	"teradata":    "teradata",
	"talend":      "talend",
	"mulesoft":    "mulesoft",
	"streamlit":   "streamlit",
	"dataverse":   "dataverse",
	"onedrive":    "onedrive",
	"eventbridge": "eventbridge",
	// devops / observability / virtualisation
	"teamcity":   "teamcity",
	"nagios":     "nagios",
	"solarwinds": "solarwinds",
	"citrix":     "citrix",
	// networking
	"vlans": "vlan",
	"vxlan": "vxlan",
	"sdn":   "sdn",
	"sase":  "sase",
	// security vendors. The certifications and the regulatory frameworks that shipped
	// here beside them have MOVED to the phrase lists and nonCorroboratingPhrases: each
	// spelling is coined, so tagging was never the problem, but a strong match also
	// vouches for every gated word in the same text — and "HIPAA training required"
	// sits on most of a healthcare catalogue. See the compliance block below.
	"crowdstrike": "crowdstrike",
	"fortinet":    "fortinet",
	"cyberark":    "cyberark",
	"osint":       "osint",
	// embedded, electronics and industrial automation. "asic" (the Australian
	// Securities and Investments Commission, on a large share of the AU finance
	// postings SEEK brings in), "rtl" (right-to-left, in any i18n posting) and "cfd"
	// (contract for difference) were strong here and are now phrase-routed.
	"firmware": "firmware",
	"cmos":     "cmos",
	"uvm":      "uvm",
	"hmi":      "hmi",
	"modbus":   "modbus",
	"i2c":      "i2c",
	"simulink": "simulink",
	// engineering analysis
	"fea": "fea",
	// AEC / construction software
	"bluebeam":     "bluebeam",
	"microstation": "microstation",
	"navisworks":   "navisworks",
	"procore":      "procore",
	// engineering-side vendor SaaS
	"trello":    "trello",
	"atlassian": "atlassian",
	"testrail":  "testrail",

	// batch 5 (2026-10 enrichment mining, freq ≥ 500) — coined product and protocol
	// names with no common English reading. Each was named by the LLM on 500+ postings
	// the dictionary tagged with nothing for it.
	// languages, frameworks, libraries
	"blazor":      "blazor",
	"sqlalchemy":  "sqlalchemy",
	"pydantic":    "pydantic",
	"asyncio":     "asyncio",
	"polars":      "polars",
	"linq":        "linq",
	"wcf":         "wcf",
	"ngrx":        "ngrx",
	"es6":         "javascript",
	"tailwindcss": "tailwind",
	// testing
	"nunit":       "nunit",
	"xunit":       "xunit",
	"phpunit":     "phpunit",
	"restassured": "rest-assured",
	// cloud, platform and data services
	"cloudflare":    "cloudflare",
	"cloudfront":    "cloudfront",
	"cloudtrail":    "cloudtrail",
	"route53":       "route53",
	"fargate":       "fargate",
	"dataproc":      "dataproc",
	"dataform":      "dataform",
	"supabase":      "supabase",
	"vercel":        "vercel",
	"cloudera":      "cloudera",
	"hdfs":          "hadoop",
	"dataops":       "dataops",
	"olap":          "olap",
	"ssas":          "ssas",
	"kql":           "kql",
	"odata":         "odata",
	"avro":          "avro",
	"ceph":          "ceph",
	"collibra":      "collibra",
	"cognos":        "cognos",
	"microstrategy": "microstrategy",
	"sybase":        "sybase",
	"dwh":           "data-warehousing",
	// integration, workflow and mainframe
	"workato":    "workato",
	"boomi":      "boomi",
	"apigee":     "apigee",
	"camunda":    "camunda",
	"websphere":  "websphere",
	"cics":       "cics",
	"agentforce": "agentforce",
	"langfuse":   "langfuse",
	// devops, build and infrastructure
	"terragrunt":  "terragrunt",
	"opentofu":    "opentofu",
	"kustomize":   "kustomize",
	"bazel":       "bazel",
	"gitflow":     "gitflow",
	"liquibase":   "liquibase",
	"slurm":       "slurm",
	"proxmox":     "proxmox",
	"nutanix":     "nutanix",
	"veeam":       "veeam",
	"vsphere":     "vmware",
	"esxi":        "vmware",
	"centos":      "centos",
	"debian":      "debian",
	"redhat":      "rhel",
	"intellij":    "intellij",
	"gdb":         "gdb",
	"cicd":        "ci-cd",
	"appdynamics": "appdynamics",
	"sre":         "sre",
	"itsm":        "itsm",
	// networking
	"evpn":       "evpn",
	"eigrp":      "eigrp",
	"ipv6":       "ipv6",
	"rdma":       "rdma",
	"infiniband": "infiniband",
	"webrtc":     "webrtc",
	"smtp":       "smtp",
	"wlan":       "wifi",
	"meraki":     "cisco-meraki",
	// security
	"snyk":      "snyk",
	"qualys":    "qualys",
	"zscaler":   "zscaler",
	"sailpoint": "sailpoint",
	"fortigate": "fortinet",
	"cspm":      "cspm",
	"abac":      "abac",
	"sbom":      "sbom",
	"appsec":    "application-security",
	"mtls":      "tls",
	// embedded, automotive and industrial
	"yocto":    "yocto",
	"qnx":      "qnx",
	"freertos": "freertos",
	"autosar":  "autosar",
	"serdes":   "serdes",
	"profinet": "profinet",
	"iot":      "iot",
	"mbse":     "mbse",
	// AEC software
	"bim":     "bim",
	"enscape": "enscape",
	// architecture patterns
	"cqrs": "cqrs",
	// CMS
	"drupal": "drupal",

	// batch 4, gated half — declared here because ambiguousWords marks wordAliases
	// keys, not canonicals. Each is a real product whose bare token is also ordinary
	// English, so it tags only with a concrete technology beside it.
	//
	// "bedrock" and "ros" were here and are gone: each named a product the dictionary
	// ALREADY carried under a different canonical ("aws bedrock" → aws-bedrock, "ros2"),
	// so a posting matched both and the facet split its count between them. Gating does
	// not help — a corroborated weak match still emits. The qualified forms cover them.
	"bootstrap": "bootstrap",
	"hibernate": "hibernate",
	"unity":     "unity",
	"eclipse":   "eclipse",
	"slack":     "slack",
	"zoom":      "zoom",
	"notion":    "notion",
	"asana":     "asana",
	"puppet":    "puppet",
	"bicep":     "bicep",
	"athena":    "athena",
	"aurora":    "aurora",
	"parquet":   "parquet",
	"loki":      "loki",
	"prefect":   "prefect",
	"flux":      "flux",
	// batch 5, gated half. Real products and system classes whose bare token either is
	// ordinary English (stripe, fastlane, perforce) or rides on non-technical postings
	// that merely name the system they use: an HR generalist's HRIS, a facilities
	// planner's CMMS, a retail associate's Bluetooth headset, a gamer-store GPU.
	"stripe":    "stripe",
	"fastlane":  "fastlane",
	"perforce":  "perforce",
	"arista":    "arista",
	"aem":       "adobe-experience-manager",
	"hris":      "hris",
	"cmms":      "cmms",
	"plm":       "plm",
	"lims":      "lims",
	"iaas":      "iaas",
	"gpu":       "gpu",
	"bluetooth": "bluetooth",
	"adas":      "adas",

	// batch 6 (2026-10-25, German market) — protocol and SCADA names the German ATS crawl
	// asks for in its requirement clauses, with no common German or English reading, and
	// only an automation engineer's posting carries them. The batch's TOOL names (EPLAN,
	// KNX, iTWO, ArchiCAD, Allplan) are phrases instead — see engineeringPhraseAliases.
	"wincc":    "scada",
	"profibus": "profibus",
	"ethercat": "ethercat",
	"canopen":  "canopen",

	// batch 7 (2026-10-25, Dutch market) — mined from the LLM skills of the open Dutch
	// postings (eures, recruitee, whatjobs-nl) and counted in their descriptions. Coined
	// product names with no Dutch or English reading, which a developer's or an architect's
	// posting asks for. Pega is a phrase (Spanish/Portuguese "pega"), and TOPdesk too: a
	// facility clerk is asked for it as often as a service-desk engineer.
	"mendix":      "mendix",
	"outsystems":  "outsystems",
	"thinkwise":   "thinkwise",
	"matillion":   "matillion",
	"archimate":   "archimate",
	"togaf":       "togaf",
	"auth0":       "auth0",
	"firestore":   "firestore",
	"onelake":     "microsoft-fabric",
	"pegasystems": "pega",

	// batch 8 (2026-10-27, Spanish market) — the Spanish names of practices already
	// canonical in English, mined from the requirement clauses of the open
	// Spanish-language technical postings. See engineeringPhraseAliases for the phrases.
	"microservicios": "microservices",
	"microservicio":  "microservices",
	"observabilidad": "observability",

	// batch 9 (2026-10-27, Brazilian and Portuguese market) — the same, in Portuguese,
	// with and without the accents. See engineeringPhraseAliases for the phrases.
	"microsserviços":  "microservices",
	"microsservicos":  "microservices",
	"microsserviço":   "microservices",
	"microsservico":   "microservices",
	"observabilidade": "observability",

	// batch 10 (2026-10-28, French market) — the same, in French, with and without the
	// accents. See engineeringPhraseAliases for the phrases.
	"observabilité":     "observability",
	"observabilite":     "observability",
	"cybersécurité":     "cybersecurity",
	"cybersecurite":     "cybersecurity",
	"conteneurisation":  "containerization",
	"datavisualisation": "data-visualization",
	"sgbdr":             "rdbms",

	// batch 11 (2026-10-29, Polish market) — the Polish single words, in the cases the
	// crawl carries. See engineeringPhraseAliases for the phrases.
	"mikroserwis": "microservices", "mikroserwisy": "microservices",
	"mikroserwisów": "microservices", "mikroserwisach": "microservices",
	"mikroserwisami": "microservices", "mikroserwisowa": "microservices",
	"mikroserwisowej": "microservices", "mikroserwisową": "microservices",
	"mikroserwisowym": "microservices", "mikroserwisowych": "microservices",
	"mikrousługi": "microservices", "mikrousług": "microservices",
	"mikrousługach": "microservices", "mikrousługowej": "microservices",
	"mikrousługową":  "microservices",
	"konteneryzacja": "containerization", "konteneryzacji": "containerization",
	"konteneryzacją": "containerization", "konteneryzację": "containerization",
	"obserwowalność": "observability", "obserwowalności": "observability",
	"obserwowalnością": "observability",

	// batch 12 (2026-10-29, Croatian, Serbian and Bosnian market), in the cases a posting
	// writes them. See engineeringPhraseAliases for the phrases.
	"mikroservisi":      "microservices",
	"mikroservisa":      "microservices",
	"mikroservisima":    "microservices",
	"mikroservise":      "microservices",
	"kontejnerizacija":  "containerization",
	"kontejnerizacije":  "containerization",
	"kontejnerizaciju":  "containerization",
	"kontejnerizacijom": "containerization",
	"kontejnerizaciji":  "containerization",
	"virtualizacija":    "virtualization",
	"virtualizacije":    "virtualization",
	"virtualizaciju":    "virtualization",
	"virtualizacijom":   "virtualization",
	"virtualizaciji":    "virtualization",
}
