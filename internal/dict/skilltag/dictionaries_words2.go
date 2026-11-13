package skilltag

// wordAliases2 is part 2 of 3 of wordAliases (see dictionaries.go).
var wordAliases2 = map[string]string{
	"indesign": "indesign",
	"webflow":  "webflow",
	"invision": "invision",
	"zeplin":   "zeplin",
	"protopie": "protopie",
	"canva":    "canva",
	"figjam":   "figjam",
	"lottie":   "lottie",
	"blender":  "blender",
	"sketch":   "sketch",
	"maya":     "maya",
	// design practices — the craft, not the tool. The singular "wireframe" is left out:
	// it is also CAD prose ("wireframe model", "wireframe view"), and the plural and
	// gerund carry the intent a posting actually states.
	"prototyping":   "prototyping",
	"wireframing":   "wireframing",
	"wireframes":    "wireframing",
	"typography":    "typography",
	"accessibility": "accessibility",
	"a11y":          "accessibility",
	// media production — the coined product names, plus the one practice whose gerund
	// is unambiguous. "houdini" is listed but gated by ambiguousWords (the escapologist).
	//
	// Two are absent for the same reason "design systems" is: the corroboration gate is
	// lifted by ANY strong skill, and both collide with prose that sits beside one.
	// "animation" is the CSS property of every frontend posting that also names React,
	// and "nuke" is what a platform posting does to a cache next to Terraform — gating
	// them would tag exactly the postings they are wrong for. The Foundry product is
	// reachable through the artists' other tools.
	"godot":   "godot",
	"zbrush":  "zbrush",
	"capcut":  "capcut",
	"houdini": "houdini",
	// "c4d" is here rather than with the "cinema 4d" phrase for a reason worth stating:
	// ambiguousWords gates the WORD pass only, and a phrase match is always strong. As
	// a phrase this token tagged a transplant pathologist's posting ("interpret C4d
	// staining"), C4d being a standard complement-fragment biomarker — and lifted the
	// gate off every weak word beside it. Only the word pass can gate a single token.
	"c4d": "cinema-4d",
	// web3 / crypto (mined from enrichment->skills; the long tail sits below the
	// freq-500 floor). Distinctive tokens only — the ambiguous ones are NOT added:
	// "cosmos" (↔ Azure Cosmos DB), "foundry" (↔ Palantir/Cloud Foundry),
	// "rollup" (↔ rollup.js bundler), "staking"/"layer 2"/"wallet" (↔ non-crypto uses).
	"web3":             "web3",
	"ethereum":         "ethereum",
	"solana":           "solana",
	"defi":             "defi",
	"evm":              "evm",
	"evms":             "evm",
	"tokenomics":       "tokenomics",
	"cryptocurrency":   "cryptocurrency",
	"cryptocurrencies": "cryptocurrency",
	// security (unambiguous tokens; ambiguous "burp" routes via a phrase below)
	"nmap":       "nmap",
	"wireshark":  "wireshark",
	"metasploit": "metasploit",
	"owasp":      "owasp",
	"nessus":     "nessus",
	"keycloak":   "keycloak",
	"oauth":      "oauth",
	"oauth2":     "oauth",
	"saml":       "saml",
	"openid":     "openid",
	"oidc":       "openid",
	"jwt":        "jwt",
	// qa / testing
	"puppeteer": "puppeteer",
	"appium":    "appium",
	"k6":        "k6",
	"gatling":   "gatling",
	"jmeter":    "jmeter",
	"testng":    "testng",
	"soapui":    "soapui",
	// data — ELT / warehouse (unambiguous; ambiguous engines route via phrases below)
	"metabase": "metabase",
	"dagster":  "dagster",
	"airbyte":  "airbyte",
	"fivetran": "fivetran",
	"debezium": "debezium",
	"hbase":    "hbase",
	"nifi":     "nifi",

	// LLM-mined batch (jobs.enrichment->skills, freq >= 1500). Distinctive single
	// tokens whose lowercase form is unambiguous, so the word pass tags both the
	// upper- and lowercase surface ("DNS"/"dns") without an acronym tier. Ambiguous
	// tokens (windows, s3, http-in-URLs, plc) are deliberately omitted or phrase-routed.
	"unix":           "unix",
	"json":           "json",
	"xml":            "xml",
	"dns":            "dns",
	"dhcp":           "dhcp",
	"ethernet":       "ethernet",
	"vpn":            "vpn",
	"bgp":            "bgp",
	"cisco":          "cisco",
	"vmware":         "vmware",
	"virtualization": "virtualization",
	"virtualisation": "virtualization",
	"macos":          "macos",
	"firewall":       "firewall",
	"firewalls":      "firewall",
	"siem":           "siem",
	"edr":            "edr",
	"sso":            "sso",
	"iam":            "iam",
	"rbac":           "rbac",
	"scada":          "scada",
	"gis":            "gis",
	"mlops":          "mlops",
	"devsecops":      "devsecops",
	"gitops":         "gitops",
	"pyspark":        "pyspark",
	"multithreading": "multithreading",
	"blockchain":     "blockchain",
	"rpa":            "rpa",
	"tdd":            "tdd",
	"oop":            "oop",
	"plsql":          "plsql",
	"iac":            "infrastructure-as-code",
	"llms":           "llm",
	"pentest":        "penetration-testing",
	"pentesting":     "penetration-testing",
	"agentic":        "agentic-ai",
	"photoshop":      "photoshop",
	// vendor products / SaaS / cloud resources (unambiguous brand tokens)
	"netsuite":   "netsuite",
	"zendesk":    "zendesk",
	"smartsheet": "smartsheet",
	"visio":      "visio",
	"bitbucket":  "bitbucket",
	"cloudwatch": "cloudwatch",
	"ec2":        "ec2",
	"rds":        "rds",
	// s3 is gated (ambiguousWords), unlike its ec2/rds siblings: a bare "S3" also
	// names a project-roadmap phase/stage label ("the S3 phase of the roadmap"),
	// so it tags only alongside a strong tech token — see the "s3 trap" test.
	"s3":    "s3",
	"mssql": "sql-server",
	"tsql":  "sql-server",
	"ga4":   "google-analytics",
	// support / scheduling / legal-practice tooling
	"calendly":  "calendly",
	"clio":      "clio",
	"freshdesk": "freshdesk",
	"uscis":     "uscis",

	// LLM-mined batch 2 (jobs.enrichment->skills, freq 500-1500). Distinctive single
	// tokens. Ultra-generic concept words (caching, routing, concurrency,
	// web-development, software-testing, memory-management, load-balancing) are
	// deliberately omitted as low-signal / over-matching; multi-word concepts are
	// phrase-routed below.
	// low-level / hardware / embedded
	"systemverilog":    "systemverilog",
	"vhdl":             "vhdl",
	"sysml":            "sysml",
	"tcl":              "tcl",
	"cuda":             "cuda",
	"rtos":             "rtos",
	"uart":             "uart",
	"jtag":             "jtag",
	"pcie":             "pcie",
	"microcontrollers": "microcontrollers",
	"lidar":            "lidar",
	"ros2":             "ros2",
	// jvm ecosystem
	"j2ee":    "j2ee",
	"jpa":     "jpa",
	"jvm":     "jvm",
	"tomcat":  "tomcat",
	"gradle":  "gradle",
	"mockito": "mockito",
	// networking / protocols
	"vpc":   "vpc",
	"vlan":  "vlan",
	"wan":   "wan",
	"ospf":  "ospf",
	"mpls":  "mpls",
	"udp":   "udp",
	"ipsec": "ipsec",
	"voip":  "voip",
	"qos":   "qos",
	"snmp":  "snmp",
	"ldap":  "ldap",
	"scim":  "scim",
	"tls":   "tls",
	"ssl":   "ssl",
	"ssh":   "ssh",
	"sftp":  "sftp",
	"wifi":  "wifi",
	"edi":   "edi",
	"hl7":   "hl7",
	"fhir":  "fhir",
	// security
	"sast":         "sast",
	"dast":         "dast",
	"waf":          "waf",
	"dlp":          "dlp",
	"xdr":          "xdr",
	"pki":          "pki",
	"cryptography": "cryptography",
	// databases / data platforms
	"db2":        "db2",
	"as400":      "as400",
	"rdbms":      "rdbms",
	"soql":       "soql",
	"lakehouse":  "lakehouse",
	"embeddings": "embeddings",
	// devops / infra / observability / management
	"sonarqube":   "sonarqube",
	"artifactory": "artifactory",
	"zabbix":      "zabbix",
	"dynatrace":   "dynatrace",
	"jamf":        "jamf",
	"intune":      "intune",
	"sccm":        "sccm",
	"kandji":      "kandji",
	"tanium":      "tanium",
	"ninjaone":    "ninjaone",
	"airwatch":    "workspace-one",
	"mdm":         "mdm",
	"cmdb":        "cmdb",
	"cmake":       "cmake",
	"svn":         "svn",
	"openstack":   "openstack",
	"ubuntu":      "ubuntu",
	"rhel":        "rhel",
	"hpc":         "hpc",
	"cli":         "cli",
	"finops":      "finops",
	"n8n":         "n8n",
	"zapier":      "zapier",
	"webhooks":    "webhooks",
	"yaml":        "yaml",
	// saas / business apps / eng tools
	"okta":      "okta",
	"wordpress": "wordpress",
	"shopify":   "shopify",
	"marketo":   "marketo",
	"firebase":  "firebase",
	"mixpanel":  "mixpanel",
	// Product-analytics platform whose name is also a physics noun ("the amplitude of
	// demand"). Gated in ambiguousWords, so it tags only in a concrete tech context.
	"amplitude":  "amplitude",
	"braze":      "braze",
	"miro":       "miro",
	"alteryx":    "alteryx",
	"anaplan":    "anaplan",
	"coupa":      "coupa",
	"ariba":      "ariba",
	"uipath":     "uipath",
	"peoplesoft": "peoplesoft",
	"windchill":  "windchill",
	"teamcenter": "teamcenter",
	"labview":    "labview",
	"arcgis":     "arcgis",
	"qgis":       "qgis",
	"minitab":    "minitab",
	"scipy":      "scipy",
	"chatgpt":    "chatgpt",
	"ssis":       "ssis",
	"ssrs":       "ssrs",
	// architecture / methodology
	"mvc":  "mvc",
	"mvvm": "mvvm",
	"soa":  "soa",
	"bpmn": "bpmn",
	"bdd":  "bdd",
	"uml":  "uml",
	// ai / ml
	"llmops":       "llmops",
	"quantization": "quantization",
	"rlhf":         "rlhf",
	// salesforce
	"lwc":         "lwc",
	"visualforce": "visualforce",
	// misc
	"bioinformatics": "bioinformatics",
	"cpq":            "cpq",
	"mainframe":      "mainframe",
	"airtable":       "airtable",
	"ajax":           "ajax",
	"wcag":           "wcag",

	// LLM-mined batch 3 — broad tech CONCEPTS (jobs.enrichment->skills, freq >= 1500).
	// A deliberate widening past the "distinctive product token" rule to the high-signal
	// generic concepts the discovery signal surfaced most. Tokens with a clear non-tech
	// collision are still withheld (security↔guard, testing↔drug/QC, architecture↔building,
	// monitoring/storage/logging, http-in-URLs, "workday"↔"a work day") — the
	// never-corrupt-a-facet invariant outranks recall.
	"ai":               "ai",
	"automation":       "automation",
	"crm":              "crm",
	"erp":              "erp",
	"saas":             "saas",
	"fintech":          "fintech",
	"ecommerce":        "ecommerce",
	"api":              "api",
	"apis":             "api",
	"cloud":            "cloud",
	"networking":       "networking",
	"analytics":        "analytics",
	"statistics":       "statistics",
	"authentication":   "authentication",
	"containerization": "containerization",
	"containerisation": "containerization",
	"seo":              "seo",
	"sdlc":             "sdlc",
	"elt":              "elt",
	"maven":            "maven",
	"genai":            "generative-ai",
	"expressjs":        "express",

	// LLM-mined batch 4 — jobs.enrichment->skills, freq >= 500 (2026-08 run: 2660
	// distinct values, 692 already resolved). The gap this batch closes is not another
	// web framework. It is the vocabulary of the NON-web technical catalogue the crawl
	// has grown into: industrial automation, electronics and test equipment, enterprise
	// vendor software, and the compliance frameworks a technical posting names.
	//
	// Everything here is a product name or a coined acronym with no ordinary reading.
	// Three groups are deliberately NOT here:
	//   - Office and business vocabulary (excel 68k, powerpoint 22k, customer_service
	//     150k). skilltag is tech-only and the catalogue is two-thirds non-IT, so the
	//     head of the mined list is correct to miss.
	//   - Tokens whose bare form is an ordinary English word (bootstrap, hibernate,
	//     unity, eclipse, slack, zoom, notion, asana, puppet, bicep, athena, aurora,
	//     bedrock, parquet, loki, prefect, flux, ros) — those are gated in
	//     ambiguousWords below.
	//   - Tokens whose bare form is a place, a person, or another language's everyday
	//     word: informatica (PT/ES "IT"), primavera ("spring"), maximo ("máximo"),
	//     palo alto, juniper, apollo, intercom, greenhouse, glue, soar, ray, node, mfa
	//     ("Master of Fine Arts"), kms ("kilometres" in a driving posting), spi
	//     (PMI's Schedule Performance Index). Each returns only through a qualifying
	//     phrase, or not at all.

	// jvm / .net server ecosystem
	"springboot": "spring",
}
