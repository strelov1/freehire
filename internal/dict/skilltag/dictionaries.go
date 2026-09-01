package skilltag

import (
	"slices"
	"strings"
)

// trimLower is the canonical-slug normal form used by the invariant test: lowercase
// with no surrounding or internal spaces. A canonical equal to its trimLower form is
// a valid slug.
func trimLower(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, " ", "")))
}

// wordAliases maps a bare alphanumeric token to its canonical skill slug. Matched
// on whole word tokens (the word pass), so an entry never matches inside a larger
// word. Ambiguous English words (go, c, r) are deliberately ABSENT here — they are
// emitted only via an unambiguous alias (golang) or a phrase ("c developer"), so
// "please go home" or "plan c" never tags. Seed list; expand toward ~200 from
// MIND-tech-ontology and github/linguist languages.yml — pure data, no engine change.
var wordAliases = map[string]string{
	// languages (unambiguous)
	"golang":     "go",
	"python":     "python",
	"java":       "java",
	"javascript": "javascript",
	"typescript": "typescript",
	"ts":         "typescript",
	"rust":       "rust",
	"kotlin":     "kotlin",
	"swift":      "swift",
	"scala":      "scala",
	"ruby":       "ruby",
	"php":        "php",
	"elixir":     "elixir",
	"erlang":     "erlang",
	"clojure":    "clojure",
	"haskell":    "haskell",
	"perl":       "perl",
	"lua":        "lua",
	"dart":       "dart",
	"groovy":     "groovy",
	"solidity":   "solidity",
	// more languages
	"julia":   "julia",
	"ocaml":   "ocaml",
	"fsharp":  "fsharp",
	"vbnet":   "vbnet",
	"cobol":   "cobol",
	"fortran": "fortran",
	"matlab":  "matlab",
	"abap":    "abap",
	"apex":    "apex",
	// 1c is the RU enterprise platform's own language. Gated (ambiguousWords) because a bare
	// "1c" also occurs as a figure/list label in English prose; the Cyrillic "1с" form the RU
	// market uses is a strong phrase alias instead.
	"1c":         "1c",
	"nim":        "nim",
	"zig":        "zig",
	"assembly":   "assembly",
	"bash":       "bash",
	"powershell": "powershell",
	"sql":        "sql",
	"html":       "html",
	"css":        "css",
	"sass":       "sass",
	"shell":      "bash",
	// frontend
	"react":     "react",
	"reactjs":   "react",
	"angular":   "angular",
	"angularjs": "angular",
	"vue":       "vue",
	"vuejs":     "vue",
	"svelte":    "svelte",
	"nextjs":    "nextjs",
	"nodejs":    "nodejs",
	"nuxt":      "nuxt",
	"redux":     "redux",
	"tailwind":  "tailwind",
	"webpack":   "webpack",
	"vite":      "vite",
	"preact":    "preact",
	"solidjs":   "solidjs",
	"remix":     "remix",
	"astro":     "astro",
	"ember":     "ember",
	"backbone":  "backbone",
	"jquery":    "jquery",
	"rxjs":      "rxjs",
	"mobx":      "mobx",
	"zustand":   "zustand",
	"storybook": "storybook",
	"figma":     "figma",
	"threejs":   "threejs",
	"webgl":     "webgl",
	"wasm":      "wasm",
	// backend frameworks
	"django":     "django",
	"flask":      "flask",
	"fastapi":    "fastapi",
	"spring":     "spring",
	"rails":      "rails",
	"laravel":    "laravel",
	"symfony":    "symfony",
	"express":    "express",
	"nestjs":     "nestjs",
	"gin":        "gin",
	"fiber":      "fiber",
	"dotnet":     "dotnet",
	"gorilla":    "gorilla",
	"koa":        "koa",
	"hapi":       "hapi",
	"actix":      "actix",
	"quarkus":    "quarkus",
	"micronaut":  "micronaut",
	"ktor":       "ktor",
	"fastify":    "fastify",
	"sinatra":    "sinatra",
	"dropwizard": "dropwizard",
	"grails":     "grails",
	// datastores
	"postgres":      "postgresql",
	"postgresql":    "postgresql",
	"psql":          "postgresql",
	"mysql":         "mysql",
	"mariadb":       "mariadb",
	"sqlite":        "sqlite",
	"redis":         "redis",
	"memcached":     "memcached",
	"mongodb":       "mongodb",
	"mongo":         "mongodb",
	"cassandra":     "cassandra",
	"dynamodb":      "dynamodb",
	"elasticsearch": "elasticsearch",
	"clickhouse":    "clickhouse",
	"snowflake":     "snowflake",
	"cockroachdb":   "cockroachdb",
	"scylladb":      "scylladb",
	"influxdb":      "influxdb",
	"timescaledb":   "timescaledb",
	"neo4j":         "neo4j",
	"couchdb":       "couchdb",
	"bigquery":      "bigquery",
	"redshift":      "redshift",
	"databricks":    "databricks",
	"hadoop":        "hadoop",
	"hive":          "hive",
	"trino":         "trino",
	"flink":         "flink",
	// messaging / streaming
	"kafka":    "kafka",
	"rabbitmq": "rabbitmq",
	"nats":     "nats",
	"pulsar":   "pulsar",
	"sqs":      "sqs",
	"sns":      "sns",
	"kinesis":  "kinesis",
	"activemq": "activemq",
	"zeromq":   "zeromq",
	"mqtt":     "mqtt",
	// infra / cloud / devops
	"kubernetes":    "kubernetes",
	"k8s":           "kubernetes",
	"docker":        "docker",
	"terraform":     "terraform",
	"ansible":       "ansible",
	"pulumi":        "pulumi",
	"helm":          "helm",
	"aws":           "aws",
	"gcp":           "gcp",
	"azure":         "azure",
	"linux":         "linux",
	"nginx":         "nginx",
	"prometheus":    "prometheus",
	"grafana":       "grafana",
	"jenkins":       "jenkins",
	"gitlab":        "gitlab",
	"circleci":      "circleci",
	"travis":        "travis",
	"argocd":        "argocd",
	"istio":         "istio",
	"envoy":         "envoy",
	"vault":         "vault",
	"cdk":           "cdk",
	"serverless":    "serverless",
	"lambda":        "lambda",
	"eks":           "eks",
	"ecs":           "ecs",
	"gke":           "gke",
	"aks":           "aks",
	"openshift":     "openshift",
	"rancher":       "rancher",
	"containerd":    "containerd",
	"podman":        "podman",
	"datadog":       "datadog",
	"sentry":        "sentry",
	"opentelemetry": "opentelemetry",
	"kibana":        "kibana",
	"logstash":      "logstash",
	"splunk":        "splunk",
	"newrelic":      "newrelic",
	"pagerduty":     "pagerduty",
	// api / data
	"graphql":    "graphql",
	"grpc":       "grpc",
	"pytorch":    "pytorch",
	"tensorflow": "tensorflow",
	"pandas":     "pandas",
	"numpy":      "numpy",
	"spark":      "spark",
	"airflow":    "airflow",
	"dbt":        "dbt",
	// ml / ai / data-eng
	"scikit":       "scikit-learn",
	"keras":        "keras",
	"jax":          "jax",
	"huggingface":  "huggingface",
	"transformers": "transformers",
	"langchain":    "langchain",
	"openai":       "openai",
	"llm":          "llm",
	"nlp":          "nlp",
	"opencv":       "opencv",
	"spacy":        "spacy",
	"mlflow":       "mlflow",
	"kubeflow":     "kubeflow",
	"dask":         "dask",
	"snowpark":     "snowpark",
	"looker":       "looker",
	"tableau":      "tableau",
	"powerbi":      "powerbi",
	"superset":     "superset",
	// llm / genai tooling — vector DBs, orchestration frameworks, model providers,
	// serving/inference. Single distinctive tokens only; English-word collisions
	// (bedrock, whisper, the "rag" project-status / "haystack" idiom) are matched
	// via phrases below or left out, since this dictionary runs on ALL jobs.
	"pinecone":   "pinecone",
	"weaviate":   "weaviate",
	"qdrant":     "qdrant",
	"milvus":     "milvus",
	"pgvector":   "pgvector",
	"faiss":      "faiss",
	"chromadb":   "chromadb",
	"langgraph":  "langgraph",
	"langsmith":  "langsmith",
	"llamaindex": "llamaindex",
	// "Microsoft Certified Professional" is the only real-world collision, and it's a
	// legacy-2000s abbreviation essentially absent from current job postings — unlike the
	// live English-word collisions ambiguousWords exists for. Strong, ungated alias.
	"mcp":       "mcp",
	"crewai":    "crewai",
	"autogen":   "autogen",
	"anthropic": "anthropic",
	"cohere":    "cohere",
	"mistral":   "mistral",
	"ollama":    "ollama",
	"vllm":      "vllm",
	"tensorrt":  "tensorrt",
	"triton":    "triton",
	"onnx":      "onnx",
	"deepspeed": "deepspeed",
	"bentoml":   "bentoml",
	"peft":      "peft",
	"yolo":      "yolo",
	"sagemaker": "sagemaker",
	// mobile
	"android": "android",
	"ios":     "ios",
	"flutter": "flutter",
	"xamarin": "xamarin",
	"ionic":   "ionic",
	"cordova": "cordova",
	"swiftui": "swiftui",
	"jetpack": "jetpack",
	// testing / tools
	"jest":       "jest",
	"mocha":      "mocha",
	"cypress":    "cypress",
	"playwright": "playwright",
	"selenium":   "selenium",
	"junit":      "junit",
	"pytest":     "pytest",
	"rspec":      "rspec",
	"vitest":     "vitest",
	"testify":    "testify",
	"cucumber":   "cucumber",
	"postman":    "postman",
	"swagger":    "swagger",
	"openapi":    "openapi",
	"protobuf":   "protobuf",
	"thrift":     "thrift",
	"git":        "git",
	"jira":       "jira",
	"confluence": "confluence",
	"elk":        "elk",

	// methodologies / practices / architecture
	"agile":         "agile",
	"scrum":         "scrum",
	"kanban":        "kanban",
	"devops":        "devops",
	"microservices": "microservices",
	"microservice":  "microservices",
	"observability": "observability",
	"restful":       "rest",

	// platforms (Salesforce/SAP/Oracle read unambiguously in IT job text)
	"salesforce": "salesforce",
	"sap":        "sap",
	"oracle":     "oracle",

	// LLM-mined gaps: high-frequency tokens the enrichment discovery signal emitted
	// (jobs.enrichment->skills) that the seed list lacked. Only distinctive tokens are
	// bare aliases here; ambiguous ones (plc↔"… plc", temporal↔the adjective,
	// embedded↔"embedded in", dax↔the index) are deliberately omitted, and R stays a
	// phrase-only route (see below).
	"github":        "github",
	"servicenow":    "servicenow",
	"sharepoint":    "sharepoint",
	"hubspot":       "hubspot",
	"nosql":         "nosql",
	"etl":           "etl",
	"itil":          "itil",
	"cybersecurity": "cybersecurity",
	"opensearch":    "opensearch",
	"solr":          "solr",
	// BI / analytics / statistics tooling
	"qlik":       "qlik",
	"sas":        "sas",
	"spss":       "spss",
	"stata":      "stata",
	"vba":        "vba",
	"xgboost":    "xgboost",
	"matplotlib": "matplotlib",
	"seaborn":    "seaborn",
	"plotly":     "plotly",
	"jupyter":    "jupyter",
	// hardware / CAD
	"autocad": "autocad",
	"revit":   "revit",
	"cad":     "cad",
	"fpga":    "fpga",
	"verilog": "verilog",
	// mechanical CAD and EDA — the stack the engineering-design category states.
	// "nx" (Siemens NX) is NOT here: it collides with the Nx JS monorepo tool, and
	// "eagle" (Autodesk EAGLE) collides with the bird and with "eagle-eyed" boilerplate.
	"solidworks": "solidworks",
	"catia":      "catia",
	"sketchup":   "sketchup",
	"altium":     "altium",
	"kicad":      "kicad",
	"ansys":      "ansys",
	// design tools — brand tokens, unambiguous on their own. "sketch", "maya" and
	// "blender" are listed too but gated by ambiguousWords (ordinary English, a person's
	// name, a kitchen appliance).
	//
	// Two products are absent because their token belongs to someone else: "framer" is
	// the carpentry trade AND the Framer Motion React library, and "spline" is the
	// splined shaft of the very mechanical population this dictionary now describes —
	// both would tag a posting with a design tool it never mentions. "creo" is Spanish
	// for "I think", so it resolves only through the "ptc creo"/"creo parametric" phrase.
	"illustrator": "illustrator",
	"indesign":    "indesign",
	"webflow":     "webflow",
	"invision":    "invision",
	"zeplin":      "zeplin",
	"protopie":    "protopie",
	"canva":       "canva",
	"figjam":      "figjam",
	"lottie":      "lottie",
	"blender":     "blender",
	"sketch":      "sketch",
	"maya":        "maya",
	// design practices — the craft, not the tool. The singular "wireframe" is left out:
	// it is also CAD prose ("wireframe model", "wireframe view"), and the plural and
	// gerund carry the intent a posting actually states.
	"prototyping":   "prototyping",
	"wireframing":   "wireframing",
	"wireframes":    "wireframing",
	"typography":    "typography",
	"accessibility": "accessibility",
	"a11y":          "accessibility",
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
	"mssql":      "sql-server",
	"tsql":       "sql-server",
	"ga4":        "google-analytics",

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
	"jboss":      "jboss",
	"weblogic":   "weblogic",
	"jsp":        "jsp",
	"jms":        "jms",
	"jdbc":       "jdbc",
	"jcl":        "jcl",
	"xslt":       "xslt",
	"wpf":        "wpf",
	"iis":        "iis",
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
}

// ambiguousWords marks the wordAliases keys whose word-pass match is "weak": Parse
// tags it ONLY when the same text carries at least one strong tech token
// (corroboration). Three groups qualify:
//
//   - English-word collisions — a common word that doubles as a tech name
//     (react/swift/spring/rust/ruby/…). Alone it is noise: a cook's "must react to
//     changes", a maintenance post's "inspect for rust".
//   - Broad concepts — the batch-3 generic tags (ai/automation/cloud/api/crm/erp/
//     saas/analytics/seo/networking/…). They appear on non-tech posts too ("AI-powered
//     marketing", a salesperson's "CRM experience", "strong networking skills"), so
//     they are too low-precision to stand alone AND too low-precision to corroborate
//     one another — only a concrete named technology (python, kubernetes, typescript,
//     a phrase, an acronym) is a trustworthy corroborator.
//   - Job-posting boilerplate — words that recur in the prose of NON-technical
//     postings in particular, listed below.
//
// The unambiguous alias forms of the collision canonicals stay strong (reactjs,
// "react native", "spring boot", expressjs, "ruby on rails"), so a genuinely-named
// stack is never gated. Membership is per-alias, not per-canonical; value is unused.
var ambiguousWords = map[string]bool{
	// English-word collisions
	"react":     true,
	"swift":     true,
	"rust":      true,
	"spring":    true,
	"express":   true,
	"ruby":      true,
	"dart":      true,
	"gin":       true,
	"fiber":     true,
	"ember":     true,
	"elixir":    true,
	"rails":     true,
	"groovy":    true,
	"gorilla":   true,
	"koa":       true,
	"remix":     true,
	"astro":     true,
	"shell":     true,
	"apex":      true,
	"julia":     true,
	"maven":     true,
	"1c":        true,
	"amplitude": true,
	// broad concepts (batch 3) — tag only in a concrete tech context
	"ai":         true,
	"automation": true,
	"crm":        true,
	"erp":        true,
	"saas":       true,
	"analytics":  true,
	"api":        true,
	"apis":       true,
	"cloud":      true,
	"networking": true,
	"statistics": true,
	"seo":        true,
	"ecommerce":  true,
	"fintech":    true,
	// job-posting boilerplate — the word belongs to the prose of NON-tech postings
	// (an "agile" supervisor, the drivers who are "the backbone", a "restful" night in
	// a care home, the "assembly" line, the rugby "scrum", tree "sap", a "sentry" post,
	// a bank "vault", the fire-code "firewall", a "rancher", a "postman", "braze"d
	// copper). As strong aliases they did double damage: each tagged its posting on its
	// own, AND lifted the gate off every weak word beside it — one "sap" on a road-
	// maintenance description turned "guard rails" into Ruby on Rails. The real roles
	// name their stack (SAP→ABAP, REST→the "rest api" phrase), so nothing is lost.
	"agile":     true,
	"backbone":  true,
	"restful":   true,
	"assembly":  true,
	"scrum":     true,
	"sap":       true,
	"sentry":    true,
	"vault":     true,
	"firewall":  true,
	"firewalls": true,
	"rancher":   true,
	"postman":   true,
	"braze":     true,
	// design words whose lowercase form is ordinary English, a person's name, an
	// occupation or a kitchen appliance: a retail posting "sketches out ideas", managers
	// called Maya and Lottie, a line cook's "industrial blender", a building's
	// "accessibility ramp", the "typography of the shelf labels", a book imprint hiring
	// an "Illustrator", a store using "Canva" for posters, a CNC operator reading
	// "wireframes". A real design posting names Figma, Photoshop or a CAD product beside
	// them, so corroboration costs nothing and drops the false ones.
	//
	// The last five matter for a second reason: a STRONG match lifts the gate off every
	// weak word in the same text, so leaving them strong would have made the gate on
	// sketch/maya/blender decorative — "Typography of the shelf labels … sketch out
	// ideas" tagged both.
	"sketch":  true,
	"maya":    true,
	"blender": true,
	// "invision" is how half the postings misspell "envision"; "prototyping" is what a
	// line cook does with a menu.
	"invision":      true,
	"prototyping":   true,
	"accessibility": true,
	"typography":    true,
	"illustrator":   true,
	"canva":         true,
	"lottie":        true,
	"wireframes":    true,
	// batch 4 — products whose bare token is ordinary English, a mythological name or
	// a body part, in exactly the postings this dictionary runs over: a startup that
	// "bootstraps" itself, a laptop that "hibernates", a culture blurb about "unity",
	// a solar "eclipse", "cut me some slack", "zoom in on the detail", "the notion
	// that", the yoga "asana", a "puppet" show, the "bicep" a trainer works, Athena
	// and Aurora and Loki as names, "parquet flooring" on a construction posting, a
	// school "prefect" and welding "flux core".
	// Every one of them is unmistakable next to a named technology and noise without
	// one, which is precisely what the corroboration gate decides.
	"bootstrap": true,
	"hibernate": true,
	"unity":     true,
	"eclipse":   true,
	"slack":     true,
	"zoom":      true,
	"notion":    true,
	"asana":     true,
	"puppet":    true,
	"bicep":     true,
	"athena":    true,
	"aurora":    true,
	"parquet":   true,
	"loki":      true,
	"prefect":   true,
	"flux":      true,
}

// phraseAlias is a punctuated or multi-word term matched against the normalized
// text with whole-term boundaries (the phrase pass). Canonicals are facet-safe
// slugs (cpp, csharp, ci-cd) rather than the raw punctuated form.
type phraseAlias struct {
	alias     string
	canonical string
}

// phraseAliases is every phrase the engine matches — the engineering vocabulary plus
// the non-engineering professional one. The two are declared separately only so
// HasEngineering can tell them apart; Parse treats them identically.
var phraseAliases = slices.Concat(engineeringPhraseAliases, professionalPhraseAliases)

// engineeringPhraseAliases covers terms the word pass cannot see because they contain
// non-alphanumeric characters or spaces. Includes the ONLY routes by which an
// ambiguous canonical (c) may be emitted.
var engineeringPhraseAliases = []phraseAlias{
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
	// writing does: design is craft a technical employer posts, and HasEngineering must
	// not read a design-only board as "never posted anything technical".
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
	// No "visual design" either: "visual design of the store" is retail prose, and a
	// phrase always matches strong, so it would corroborate the gated words beside it.
	{"motion design", "motion-design"},
	{"motion graphics", "motion-graphics"},
	{"user flows", "user-flows"},
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
	// instead: a posting whose only match is HIPAA or GDPR is a compliance role, and
	// HasEngineering must not read it as an engineering one.
	{"nist", "nist"}, {"fedramp", "fedramp"}, {"cmmc", "cmmc"},
	{"cissp", "cissp"}, {"cism", "cism"}, {"ccna", "ccna"},
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
}

// professionalPhraseAliases is the non-engineering half of the IT-company role
// vocabulary: the recruiting, HR, finance, legal, operations, sales, customer-success
// and support craft a technical company hires for without hiring an engineer. Parse
// matches these exactly like any other phrase — a skills facet describes every posting,
// not only the technical ones — but they are declared apart so a caller can ask the
// narrower question HasEngineering answers.
//
// What is NOT here is the point: developer relations, technical writing, business
// analysis and pre-sales stay in engineeringPhraseAliases. They are tech-industry
// craft, posted by technical employers, and the conservative error for every caller of
// HasEngineering is to keep a board rather than retire a live one.
var professionalPhraseAliases = []phraseAlias{
	// AI enablement — the adoption side of AI, and a skill rather than a role.
	// These postings are Program Managers, Architects, Change Managers and even
	// Interns whose SUBJECT is getting an organisation to use AI; tagging the
	// subject lets it filter alongside the role instead of displacing it, which
	// is what a named role did when this first shipped.
	//
	// "ai trainer" is deliberately absent: it names the opposite job — labelling
	// data so a model learns, not teaching people to use one.
	{"ai enablement", "ai-enablement"},
	{"ai adoption", "ai-enablement"},
	{"ai transformation", "ai-enablement"},
	// recruiting
	{"boolean search", "boolean-search"},
	{"talent sourcing", "talent-sourcing"},
	{"technical screening", "technical-screening"},
	{"candidate experience", "candidate-experience"},
	{"employer branding", "employer-branding"},
	{"full-cycle recruiting", "full-cycle-recruiting"}, {"full cycle recruiting", "full-cycle-recruiting"},
	{"linkedin recruiter", "linkedin-recruiter"},
	{"smartrecruiters", "smartrecruiters"},
	// hr / people
	{"employee relations", "employee-relations"},
	{"performance management", "performance-management"},
	{"talent management", "talent-management"},
	{"succession planning", "succession-planning"},
	{"people analytics", "people-analytics"},
	{"bamboohr", "bamboohr"}, {"successfactors", "successfactors"},
	// finance
	{"financial modeling", "financial-modeling"},
	{"revenue recognition", "revenue-recognition"},
	{"accounts payable", "accounts-payable"},
	{"accounts receivable", "accounts-receivable"},
	{"general ledger", "general-ledger"},
	{"financial reporting", "financial-reporting"},
	{"quickbooks", "quickbooks"}, {"netsuite", "netsuite"}, {"xero", "xero"},
	// legal
	{"contract negotiation", "contract-negotiation"},
	{"contract drafting", "contract-drafting"},
	{"contract lifecycle management", "contract-lifecycle-management"},
	{"legal research", "legal-research"},
	{"regulatory compliance", "regulatory-compliance"},
	// operations
	{"process improvement", "process-improvement"},
	{"vendor management", "vendor-management"},
	{"program management", "program-management"},
	{"strategic planning", "strategic-planning"},
	{"stakeholder management", "stakeholder-management"},
	// agile/PM certifications — spelled-out forms are unambiguous strong matches;
	// the acronym forms (CSM/PSM/PMP/SAFe) collide with other meanings in general
	// job text and resolve only via categoryScopedAcronyms below.
	{"certified scrummaster", "certified-scrummaster"}, {"certified scrum master", "certified-scrummaster"},
	{"professional scrum master", "professional-scrum-master"},
	{"project management professional", "pmp"},
	{"scaled agile framework", "safe-agile"},
	// sales
	{"account executive", "account-executive"},
	{"business development", "business-development"},
	{"pipeline management", "pipeline-management"},
	{"cold outreach", "cold-outreach"},
	{"sales enablement", "sales-enablement"},
	{"lead generation", "lead-generation"},
	// customer success
	{"customer onboarding", "customer-onboarding"},
	{"customer retention", "customer-retention"},
	{"churn prevention", "churn-prevention"},
	{"customer health score", "customer-health-score"},
	{"renewal management", "renewal-management"},
	{"gainsight", "gainsight"}, {"churnzero", "churnzero"},
	// support
	{"help desk", "help-desk"},
	{"service desk", "service-desk"},
	{"ticket resolution", "ticket-resolution"},
	// marketing — search optimization tooling. These are unambiguous product names,
	// so they are strong matches, which also makes them corroborators for the gated
	// `seo` canonical: before this block a posting could name its whole toolchain and
	// still lose the `seo` tag for want of an engineering token to corroborate it.
	{"semrush", "semrush"}, {"ahrefs", "ahrefs"},
	{"screaming frog", "screaming-frog"},
	{"google search console", "google-search-console"}, {"search console", "google-search-console"},
	// marketing — lifecycle, email and advertising platforms. The ad platforms are
	// phrases rather than bare vendor names on purpose: "meta", "google" and
	// "linkedin" alone name the company, not the ad product.
	{"klaviyo", "klaviyo"}, {"mailchimp", "mailchimp"}, {"customer.io", "customer-io"},
	{"google ads", "google-ads"}, {"google adwords", "google-ads"}, {"adwords", "google-ads"},
	{"meta ads", "meta-ads"}, {"facebook ads", "meta-ads"}, {"meta ads manager", "meta-ads"},
	{"tiktok ads", "tiktok-ads"}, {"linkedin ads", "linkedin-ads"},
	// marketing — measurement and social tooling. Segment, Buffer and Later are
	// missing on purpose: each is an ordinary English word in exactly the postings
	// this block serves ("the customer segment", "a content buffer", "apply later"),
	// and a strong alias would tag the corpus rather than describe the job.
	{"looker studio", "looker-studio"}, {"data studio", "looker-studio"},
	{"hootsuite", "hootsuite"}, {"sprout social", "sprout-social"},
	{"contentful", "contentful"},
	// marketing — the disciplines themselves, so a search can combine "does technical
	// SEO" with "uses Ahrefs". All multi-word, hence strong without gating. "SEM" is
	// absent by design: it is a Portuguese and Spanish preposition ("sem experiência")
	// and this catalogue carries that population in bulk.
	{"technical seo", "technical-seo"},
	{"link building", "link-building"}, {"linkbuilding", "link-building"},
	{"paid social", "paid-social"},
	{"paid search", "ppc"}, {"ppc campaigns", "ppc"}, {"ppc management", "ppc"},
	{"ppc specialist", "ppc"}, {"ppc manager", "ppc"}, {"pay-per-click", "ppc"},
	{"demand generation", "demand-generation"}, {"demand gen", "demand-generation"},
	{"lifecycle marketing", "lifecycle-marketing"}, {"crm marketing", "lifecycle-marketing"},
	{"marketing automation", "marketing-automation"},
	{"generative engine optimization", "generative-engine-optimization"},
	{"answer engine optimization", "generative-engine-optimization"},
	{"generative search optimization", "generative-engine-optimization"},
	{"content marketing", "content-marketing"},
	{"email marketing", "email-marketing"},
	{"influencer marketing", "influencer-marketing"},
	{"copywriting", "copywriting"},
	// GTM is go-to-market in a posting far more often than it is Google Tag Manager:
	// "GTM strategy" and "GTM motion" are the common forms, while the tag manager is
	// spelled out or named as a container. So the abbreviation belongs to this
	// canonical, and the product keeps its full name (declared with the other Google
	// products above) plus the container form.
	{"go-to-market", "go-to-market"}, {"go to market", "go-to-market"},
	{"gtm strategy", "go-to-market"}, {"gtm motion", "go-to-market"},
	{"gtm plan", "go-to-market"}, {"gtm execution", "go-to-market"},
	{"gtm container", "google-tag-manager"},

	// LLM-mined batch 4 — the business-software half. These sit here rather than with
	// the engineering vocabulary for the reason the list exists: a board that only ever
	// posts recruiters, AEs and payroll admins names Workday and Gong all day, and
	// HasEngineering must not read that as a technical board.
	//
	// Each is phrase-routed because the vendor took an ordinary word: a "flexible
	// workday", "I concur", a "gong", the god Apollo, the building "intercom", a
	// "greenhouse". Bare "adp" is three letters, so it carries its product suffix too.
	{"workday hcm", "workday"}, {"workday financials", "workday"},
	{"workday adaptive", "workday"}, {"workday integrations", "workday"},
	{"sap concur", "concur"},
	{"adp workforce now", "adp"}, {"adp payroll", "adp"},
	{"apollo.io", "apollo-io"},
	{"gong.io", "gong"},
	{"linkedin sales navigator", "linkedin-sales-navigator"},
	{"sales navigator", "linkedin-sales-navigator"},
	// Unambiguous single-token vendor names, same shape as quickbooks/bamboohr above.
	{"zoominfo", "zoominfo"}, {"salesloft", "salesloft"},
	{"veeva", "veeva"}, {"yardi", "yardi"},
	// Privacy and trade-control regimes. They belong to this list rather than the
	// engineering one for the reason the list exists: a posting whose only match is
	// HIPAA is a healthcare compliance role, and HasEngineering reads an unrecognised
	// vocabulary as engineering. All are non-corroborating — see below.
	{"hipaa", "hipaa"}, {"gdpr", "gdpr"}, {"ccpa", "ccpa"}, {"lgpd", "lgpd"},
	{"itar", "itar"}, {"dfars", "dfars"},
}

// nonCorroboratingPhrases are the phrase canonicals that tag on their own but do NOT
// vouch for the gated single-word canonicals in ambiguousWords. Every phrase match is
// otherwise a strong match, and a strong match rescues a weak one — which is right for
// a named product ("Ahrefs" genuinely evidences an SEO role) and wrong for a
// discipline. "AI-powered marketing automation" is marketing prose, not an AI
// requirement, and marketing postings carry that phrasing at scale.
var nonCorroboratingPhrases = map[string]bool{
	"technical-seo":                  true,
	"link-building":                  true,
	"paid-social":                    true,
	"ppc":                            true,
	"demand-generation":              true,
	"lifecycle-marketing":            true,
	"marketing-automation":           true,
	"generative-engine-optimization": true,
	"content-marketing":              true,
	"email-marketing":                true,
	"influencer-marketing":           true,
	"copywriting":                    true,
	"go-to-market":                   true,
	// sales / support — same doctrine as the marketing disciplines above: "manage our
	// CRM as an account executive" names the gated word "crm" without evidencing any
	// technical involvement with it, and that phrasing recurs at scale in this category.
	"account-executive":    true,
	"business-development": true,
	"pipeline-management":  true,
	"cold-outreach":        true,
	"sales-enablement":     true,
	"lead-generation":      true,
	"help-desk":            true,
	"service-desk":         true,
	"ticket-resolution":    true,
	// batch 4 — shop-floor craft. Same doctrine, opposite direction: the term is real
	// and specific, but the posting carrying it is a machinist's, not an engineer's,
	// and the design vocabulary (wireframes, sketch, prototyping) is ordinary prose
	// there. Tagging the craft is right; letting it lift the gate is how a CNC
	// operator came back tagged with wireframing.
	"cnc":          true,
	"soldering":    true,
	"oscilloscope": true,
	// batch 4 — compliance frameworks and certifications. Naming a regime is evidence
	// that the posting is SUBJECT to it, never that whoever fills it is technical: a
	// nurse's posting carries HIPAA, an injection-moulding plant's carries ISO 9001,
	// and a defence administrator's carries ITAR. As strong matches they lifted the
	// gate off the words beside them and tagged `slack` on a nursing post.
	"nist":      true,
	"fedramp":   true,
	"cmmc":      true,
	"cissp":     true,
	"cism":      true,
	"ccna":      true,
	"pci-dss":   true,
	"soc-2":     true,
	"iso-9001":  true,
	"iso-13485": true,
	"iso-26262": true,
	"as9100":    true,
	"hipaa":     true,
	"gdpr":      true,
	"ccpa":      true,
	"lgpd":      true,
	"itar":      true,
	"dfars":     true,
}

// nonEngineeringBareCanonicals are non-engineering disciplines whose canonical also has a
// BARE, single-word alias in wordAliases ("seo", "ecommerce") rather than only a multi-word
// phrase. professionalPhraseAliases covers the multi-word marketing/business phrases
// (technical-seo, link-building, content-marketing, …); these two canonicals fall through
// that derivation entirely and, unlisted, would read as unrecognised — which HasEngineering
// treats as engineering — for a discipline the dictionary in fact recognises.
var nonEngineeringBareCanonicals = map[string]bool{
	"seo":       true,
	"ecommerce": true,
}

// nonEngineeringCanonicals is derived from professionalPhraseAliases plus
// nonEngineeringBareCanonicals, never written by hand beyond those two sources: a term added
// to either cannot drift out of sync with this set.
var nonEngineeringCanonicals = func() map[string]bool {
	out := make(map[string]bool, len(professionalPhraseAliases)+len(nonEngineeringBareCanonicals))
	for _, p := range professionalPhraseAliases {
		out[p.canonical] = true
	}
	for c := range nonEngineeringBareCanonicals {
		out[c] = true
	}
	return out
}()

// sharedAcronyms resolve in ALL text (jobs and résumés). They are matched by their
// exact UPPERCASE surface form as a whole word (case-preserved pass), because their
// lowercase form is deliberately excluded as ambiguous (`ml` = millilitre). The
// uppercase form is unambiguous even in job descriptions. Each value is a canonical
// that already exists in the vocabulary — an acronym is another alias, never a new
// facet value.
var sharedAcronyms = map[string]string{
	"ML": "machine-learning",
}

// resumeAcronyms resolve ONLY when parsing résumés (Parse with WithResumeAcronyms).
// Their uppercase form collides with a non-tech meaning in JOB text — "RAG status"
// (red/amber/green project health) — so tagging them on jobs would corrupt facets;
// in résumés that collision is near-absent. Each value is an existing canonical.
var resumeAcronyms = map[string]string{
	"RAG": "rag",
}

// categoryScopedAcronym pairs a canonical with the job categories it may resolve
// for (see WithAcronymCategory) — a third acronym tier, for a term ambiguous in
// job text generally but unambiguous within a specific category. The category
// already evidences an AI-flavored posting (it required an AI-flavored title to
// classify into it), so it substitutes for corroboration without reopening the
// collision catalogue-wide.
type categoryScopedAcronym struct {
	canonical         string
	allowedCategories map[string]bool
}

// categoryScopedAcronyms resolve on JOB text only when the caller supplies a
// category on the acronym's own allow-list. RAG collides with "RAG status"
// (red/amber/green project health) in general job text — hence resumeAcronyms
// above — but within ai_engineering/ml_ai postings that collision is negligible
// and the acronym is the dominant real-world spelling (vs. the spelled-out
// "retrieval augmented generation" phrase).
var categoryScopedAcronyms = map[string]categoryScopedAcronym{
	"RAG": {canonical: "rag", allowedCategories: map[string]bool{"ai_engineering": true, "ml_ai": true}},
	// Each collides with another meaning in general job text — CSM with Customer
	// Success Manager, PSM/PMP are short enough to be noise, bare SAFe is the common
	// English word — but within a posting already classified project_management the
	// collision is negligible and the acronym is the dominant real-world spelling.
	"CSM": {canonical: "certified-scrummaster", allowedCategories: map[string]bool{"project_management": true}},
	"PSM": {canonical: "professional-scrum-master", allowedCategories: map[string]bool{"project_management": true}},
	"PMP": {canonical: "pmp", allowedCategories: map[string]bool{"project_management": true}},
	// "SAFe" is the framework's own stylization and the dominant real-world spelling;
	// the case-sensitive acronym pass matches this exact form. "SAFE" (all-caps) is
	// also listed: ATS feeds that render whole titles in caps would otherwise miss it.
	"SAFe": {canonical: "safe-agile", allowedCategories: map[string]bool{"project_management": true}},
	"SAFE": {canonical: "safe-agile", allowedCategories: map[string]bool{"project_management": true}},
}
