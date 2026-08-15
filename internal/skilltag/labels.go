package skilltag

import (
	"maps"
	"slices"
	"strings"
	"unicode"
)

// Display labels for the canonical skill slugs.
//
// The canonical vocabulary is a slug set — lowercase, hyphenated, matchable ("aws",
// "nodejs", "ci-cd"). That is the right shape for a facet value and the wrong shape for
// a heading: a page that says "javascript" and "postgresql" reads as raw data, not as a
// product. Labels are that vocabulary's display side, owned here because the canonical
// set is owned here — a skill added to the dictionary with no label is a test failure,
// not a slug leaking into the interface.
//
// A label is NOT a spelling the parser accepts. Aliases resolve text to a canonical
// (dictionaries.go); this maps a canonical to the one way it is written for a reader.
// The interchangeable-surface table is a third thing again — the spellings a CV may be
// rewritten between — and stays lowercase prose on purpose.

// displayNames overrides the mechanical title-casing below. An entry earns its place by
// being something the machine cannot derive: an acronym ("aws"), a vendor's own
// stylization ("dbt", "pandas", "PostgreSQL"), punctuation the slug had to drop ("C++",
// "CI/CD", "Node.js"), or a word that is not two words ("Pre-sales").
//
// Everything else is deliberately absent: "data-engineering" → "Data Engineering" needs
// no curation, and listing it here would only be one more line to keep true.
var displayNames = map[string]string{
	// Numerals and punctuation the slug form cannot carry.
	"1c":          "1C",
	"ab-testing":  "A/B Testing",
	"as400":       "AS/400",
	"ci-cd":       "CI/CD",
	"civil-3d":    "Civil 3D",
	"cpp":         "C++",
	"csharp":      "C#",
	"customer-io": "Customer.io",
	"dotnet":      ".NET",
	"iso-27001":   "ISO 27001",
	"monday-com":  "Monday.com",
	"plsql":       "PL/SQL",
	"ros2":        "ROS 2",
	"sap-mm":      "SAP MM",
	"sap-s4hana":  "SAP S/4HANA",
	"sd-wan":      "SD-WAN",
	"soc-2":       "SOC 2",
	"tcp-ip":      "TCP/IP",
	"vbnet":       "VB.NET",
	"wifi":        "Wi-Fi",

	// Acronyms and initialisms — written in caps, never as words.
	"abap":   "ABAP",
	"ai":     "AI",
	"ajax":   "AJAX",
	"aks":    "AKS",
	"api":    "API",
	"aws":    "AWS",
	"bdd":    "BDD",
	"bgp":    "BGP",
	"bpmn":   "BPMN",
	"cad":    "CAD",
	"catia":  "CATIA",
	"cdk":    "CDK",
	"cli":    "CLI",
	"cmdb":   "CMDB",
	"cobol":  "COBOL",
	"cpq":    "CPQ",
	"crm":    "CRM",
	"css":    "CSS",
	"cuda":   "CUDA",
	"dast":   "DAST",
	"dhcp":   "DHCP",
	"dlp":    "DLP",
	"dns":    "DNS",
	"ec2":    "EC2",
	"ecs":    "ECS",
	"edi":    "EDI",
	"edr":    "EDR",
	"eks":    "EKS",
	"elk":    "ELK",
	"elt":    "ELT",
	"erp":    "ERP",
	"etl":    "ETL",
	"evm":    "EVM",
	"faiss":  "FAISS",
	"fhir":   "FHIR",
	"fpga":   "FPGA",
	"gcp":    "GCP",
	"gis":    "GIS",
	"gke":    "GKE",
	"hl7":    "HL7",
	"hpc":    "HPC",
	"html":   "HTML",
	"iam":    "IAM",
	"ipsec":  "IPsec",
	"itil":   "ITIL",
	"j2ee":   "J2EE",
	"jax":    "JAX",
	"jpa":    "JPA",
	"json":   "JSON",
	"jtag":   "JTAG",
	"jvm":    "JVM",
	"jwt":    "JWT",
	"ldap":   "LDAP",
	"llm":    "LLM",
	"lwc":    "LWC",
	"matlab": "MATLAB",
	"mcp":    "MCP",
	"mdm":    "MDM",
	"mpls":   "MPLS",
	"mqtt":   "MQTT",
	"mvc":    "MVC",
	"mvvm":   "MVVM",
	"nlp":    "NLP",
	"onnx":   "ONNX",
	"oop":    "OOP",
	"ospf":   "OSPF",
	"owasp":  "OWASP",
	"pcie":   "PCIe",
	"peft":   "PEFT",
	"php":    "PHP",
	"pki":    "PKI",
	"pmp":    "PMP",
	"ppc":    "PPC",
	"qos":    "QoS",
	"rag":    "RAG",
	"rbac":   "RBAC",
	"rdbms":  "RDBMS",
	"rds":    "RDS",
	"rest":   "REST",
	"rhel":   "RHEL",
	"rlhf":   "RLHF",
	"rpa":    "RPA",
	"rtos":   "RTOS",
	"saml":   "SAML",
	"sap":    "SAP",
	"sas":    "SAS",
	"sast":   "SAST",
	"scada":  "SCADA",
	"sccm":   "SCCM",
	"scim":   "SCIM",
	"sdlc":   "SDLC",
	"seo":    "SEO",
	"sftp":   "SFTP",
	"siem":   "SIEM",
	"snmp":   "SNMP",
	"sns":    "SNS",
	"soa":    "SOA",
	"soql":   "SOQL",
	"spss":   "SPSS",
	"sql":    "SQL",
	"sqs":    "SQS",
	"ssh":    "SSH",
	"ssis":   "SSIS",
	"ssl":    "SSL",
	"sso":    "SSO",
	"ssrs":   "SSRS",
	"svn":    "SVN",
	"tdd":    "TDD",
	"tls":    "TLS",
	"uart":   "UART",
	"udp":    "UDP",
	"uml":    "UML",
	"vba":    "VBA",
	"vhdl":   "VHDL",
	"vlan":   "VLAN",
	"voip":   "VoIP",
	"vpc":    "VPC",
	"vpn":    "VPN",
	"waf":    "WAF",
	"wan":    "WAN",
	"wcag":   "WCAG",
	"xdr":    "XDR",
	"xml":    "XML",
	"yaml":   "YAML",
	"yolo":   "YOLO",

	// Compounds that carry a capital inside rather than in front.
	"defi":      "DeFi",
	"devops":    "DevOps",
	"devsecops": "DevSecOps",
	"finops":    "FinOps",
	"gitops":    "GitOps",
	"llmops":    "LLMOps",
	"mlops":     "MLOps",
	"nosql":     "NoSQL",
	"saas":      "SaaS",

	// Vendor and project stylizations — how the thing writes its own name.
	"activemq":              "ActiveMQ",
	"adobe-xd":              "Adobe XD",
	"agentic-ai":            "Agentic AI",
	"arcgis":                "ArcGIS",
	"argocd":                "Argo CD",
	"autocad":               "AutoCAD",
	"autogen":               "AutoGen",
	"aws-bedrock":           "AWS Bedrock",
	"azure-ad":              "Azure AD",
	"azure-devops":          "Azure DevOps",
	"backbone":              "Backbone.js",
	"bamboohr":              "BambooHR",
	"bentoml":               "BentoML",
	"bigquery":              "BigQuery",
	"certified-scrummaster": "Certified ScrumMaster",
	"chatgpt":               "ChatGPT",
	"chromadb":              "ChromaDB",
	"churnzero":             "ChurnZero",
	"circleci":              "CircleCI",
	"clickhouse":            "ClickHouse",
	"cloudformation":        "CloudFormation",
	"cloudwatch":            "CloudWatch",
	"cmake":                 "CMake",
	"cockroachdb":           "CockroachDB",
	"conversational-ai":     "Conversational AI",
	"couchdb":               "CouchDB",
	"crewai":                "CrewAI",
	"dbt":                   "dbt",
	"deepspeed":             "DeepSpeed",
	"dynamodb":              "DynamoDB",
	"ecommerce":             "E-commerce",
	"ember":                 "Ember.js",
	"entra-id":              "Entra ID",
	"ethersjs":              "ethers.js",
	"express":               "Express.js",
	"fastapi":               "FastAPI",
	"figjam":                "FigJam",
	"generative-ai":         "Generative AI",
	"github":                "GitHub",
	"github-actions":        "GitHub Actions",
	"gitlab":                "GitLab",
	"graphql":               "GraphQL",
	"grpc":                  "gRPC",
	"hapi":                  "hapi",
	"hbase":                 "HBase",
	"hubspot":               "HubSpot",
	"huggingface":           "Hugging Face",
	"hyper-v":               "Hyper-V",
	"indesign":              "InDesign",
	"influxdb":              "InfluxDB",
	"invision":              "InVision",
	"ios":                   "iOS",
	"javascript":            "JavaScript",
	"jmeter":                "JMeter",
	"jquery":                "jQuery",
	"junit":                 "JUnit",
	"k6":                    "k6",
	"kicad":                 "KiCad",
	"labview":               "LabVIEW",
	"langchain":             "LangChain",
	"langgraph":             "LangGraph",
	"langsmith":             "LangSmith",
	"lidar":                 "LiDAR",
	"linkedin-ads":          "LinkedIn Ads",
	"linkedin-recruiter":    "LinkedIn Recruiter",
	"llamaindex":            "LlamaIndex",
	"macos":                 "macOS",
	"madcap-flare":          "MadCap Flare",
	"mariadb":               "MariaDB",
	"mitre-attack":          "MITRE ATT&CK",
	"mlflow":                "MLflow",
	"mobx":                  "MobX",
	"mongodb":               "MongoDB",
	"mysql":                 "MySQL",
	"n8n":                   "n8n",
	"nats":                  "NATS",
	"nestjs":                "NestJS",
	"netsuite":              "NetSuite",
	"newrelic":              "New Relic",
	"nextjs":                "Next.js",
	"nifi":                  "NiFi",
	"nodejs":                "Node.js",
	"numpy":                 "NumPy",
	"oauth":                 "OAuth",
	"objective-c":           "Objective-C",
	"ocaml":                 "OCaml",
	"openai":                "OpenAI",
	"openapi":               "OpenAPI",
	"opencv":                "OpenCV",
	"openid":                "OpenID",
	"opensearch":            "OpenSearch",
	"openshift":             "OpenShift",
	"openstack":             "OpenStack",
	"opentelemetry":         "OpenTelemetry",
	"pagerduty":             "PagerDuty",
	"pandas":                "pandas",
	"peoplesoft":            "PeopleSoft",
	"pgvector":              "pgvector",
	"postgresql":            "PostgreSQL",
	"powerapps":             "Power Apps",
	"powerbi":               "Power BI",
	"powershell":            "PowerShell",
	"protopie":              "ProtoPie",
	"pyspark":               "PySpark",
	"pytest":                "pytest",
	"pytorch":               "PyTorch",
	"qgis":                  "QGIS",
	"quickbooks":            "QuickBooks",
	"rabbitmq":              "RabbitMQ",
	"rspec":                 "RSpec",
	"rxjs":                  "RxJS",
	"safe-agile":            "SAFe Agile",
	"sagemaker":             "SageMaker",
	"scikit-learn":          "scikit-learn",
	"scipy":                 "SciPy",
	"scylladb":              "ScyllaDB",
	"servicenow":            "ServiceNow",
	"sharepoint":            "SharePoint",
	"siemens-nx":            "Siemens NX",
	"sketchup":              "SketchUp",
	"smartrecruiters":       "SmartRecruiters",
	"soapui":                "SoapUI",
	"solidjs":               "SolidJS",
	"solidworks":            "SolidWorks",
	"sonarqube":             "SonarQube",
	"spacy":                 "spaCy",
	"sql-server":            "SQL Server",
	"sqlite":                "SQLite",
	"successfactors":        "SuccessFactors",
	"swiftui":               "SwiftUI",
	"sysml":                 "SysML",
	"systemverilog":         "SystemVerilog",
	"technical-seo":         "Technical SEO",
	"tensorflow":            "TensorFlow",
	"tensorrt":              "TensorRT",
	"testng":                "TestNG",
	"threejs":               "Three.js",
	"tiktok-ads":            "TikTok Ads",
	"timescaledb":           "TimescaleDB",
	"travis":                "Travis CI",
	"typescript":            "TypeScript",
	"uipath":                "UiPath",
	"vertex-ai":             "Vertex AI",
	"vllm":                  "vLLM",
	"vmware":                "VMware",
	"wasm":                  "WebAssembly",
	"webgl":                 "WebGL",
	"wordpress":             "WordPress",
	"xgboost":               "XGBoost",
	"zeromq":                "ZeroMQ",

	// Phrases that are one idea, not a run of capitalised words.
	"docs-as-code":           "Docs as Code",
	"go-to-market":           "Go-to-Market",
	"infrastructure-as-code": "Infrastructure as Code",
	"pre-sales":              "Pre-sales",
	"proof-of-concept":       "Proof of Concept",
}

// Label is how a canonical skill slug is written for a reader: the curated name when
// there is one, else the slug title-cased on its hyphens. An unknown slug gets the same
// mechanical treatment rather than an error — a facet value that outlives a dictionary
// edit still renders as words.
func Label(canonical string) string {
	if name, ok := displayNames[canonical]; ok {
		return name
	}
	return titleCase(canonical)
}

// Labels is the whole canonical vocabulary with its display names — the catalog
// cmd/gen-contracts emits for the SPA, so the frontend labels skills off the same
// dictionary that decides them.
func Labels() map[string]string {
	canonicals := Canonicals()
	out := make(map[string]string, len(canonicals))
	for _, c := range canonicals {
		out[c] = Label(c)
	}
	return out
}

// Canonicals is every skill this package can emit, sorted. Drawn from every alias tier
// so a canonical reachable only through an acronym is still in the catalog.
func Canonicals() []string {
	set := map[string]struct{}{}
	for _, c := range wordAliases {
		set[c] = struct{}{}
	}
	for _, p := range phraseAliases {
		set[p.canonical] = struct{}{}
	}
	for _, c := range sharedAcronyms {
		set[c] = struct{}{}
	}
	for _, c := range resumeAcronyms {
		set[c] = struct{}{}
	}
	for _, a := range categoryScopedAcronyms {
		set[a.canonical] = struct{}{}
	}
	return slices.Sorted(maps.Keys(set))
}

// titleCase capitalises each hyphen-separated word: "data-engineering" → "Data
// Engineering". Only the first rune is touched, so a slug that already carries an inner
// capital (none do today) would keep it.
func titleCase(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		if w == "" {
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}
