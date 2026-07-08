package skilltag

import (
	"reflect"
	"testing"
)

func TestNormalizeStripsHTMLAndLowercases(t *testing.T) {
	got := normalize("<div><p>Senior <b>Go</b> Engineer</p></div>")
	// Two spaces between words: each surrounding tag is replaced by one space.
	// Separators are NOT collapsed here — the phrase matcher handles that.
	want := "senior  go  engineer"
	if got != want {
		t.Fatalf("normalize = %q, want %q", got, want)
	}
}

func TestWordTokens(t *testing.T) {
	got := wordTokens("go, node.js & c++17")
	want := []string{"go", "node", "js", "c", "17"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wordTokens = %#v, want %#v", got, want)
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"plain alias", "We use Golang and PostgreSQL.", []string{"go", "postgresql"}},
		{"dedup + sort", "react, React.js, REACT", []string{"react"}},
		{"punctuated", "Strong C++ and C# with .NET.", []string{"cpp", "csharp", "dotnet"}},
		{"node variants", "node.js / nodejs / node js", []string{"nodejs"}},
		{"multiword", "React Native and CI/CD pipelines", []string{"ci-cd", "react", "react-native"}},
		{"ambiguous word rejected", "Please go to the careers page in C.", nil},
		{"ambiguous via qualifier", "5y as a C developer", []string{"c"}},
		{"objective-c does not leak bare c", "Objective-C developer", []string{"objective-c"}},
		{"objective-c alongside real C still tags both", "Objective-C and C developer", []string{"c", "objective-c"}},
		{"word boundary", "a reaction to going rusty", nil},
		{"html stripped", "<p>Kubernetes</p><a href='k8s'>k8s</a>", []string{"kubernetes"}},
		{"empty", "", nil},
		{"dotted domain not matched", "see docs at foo.asp.net for details", nil},
		{"sentence-end periods ok", "We use C#. Also ASP.NET.", []string{"csharp", "dotnet"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// Separator-insensitive matching (skilltag-matching-fixes): '-'/'_' between
// alphanumerics are equivalent to a space, so hyphenated/underscored multi-word
// terms resolve like their space form — without touching punctuated canonicals.
func TestParse_SeparatorInsensitive(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"hyphen multiword", "Experience with distributed-systems at scale.", []string{"distributed-systems"}},
		{"underscore multiword", "distributed_systems everywhere", []string{"distributed-systems"}},
		{"space still resolves", "distributed systems everywhere", []string{"distributed-systems"}},
		{"hyphen and space dedup to one", "distributed-systems and distributed systems", []string{"distributed-systems"}},
		{"machine-learning hyphen", "machine-learning pipelines", []string{"machine-learning"}},
		{"ci-cd hyphen", "CI-CD pipeline", []string{"ci-cd"}},
		{"react-native hyphen", "React-Native apps", []string{"react", "react-native"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// Case-preserving acronym pass (skilltag-matching-fixes): a curated shared tier
// (ML) resolves everywhere; a résumé-scoped tier (RAG) resolves only under the
// résumé option so it never tags job facets ("RAG status").
func TestParse_Acronyms(t *testing.T) {
	has := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}

	// Shared acronym ML → machine-learning, applied by the default Parse.
	if got := Parse("Senior ML Engineer with Python"); !has(got, "machine-learning") {
		t.Errorf("Parse ML = %v, want machine-learning", got)
	}
	// Whole-word only: ML embedded in HTML/HTMl must not fire; lowercase ml must not.
	if got := Parse("We write HTML and CSS"); has(got, "machine-learning") {
		t.Errorf("Parse HTML = %v, must not emit machine-learning", got)
	}
	if got := Parse("Mix 500 ml of solution"); has(got, "machine-learning") {
		t.Errorf("Parse 'ml' lowercase = %v, must not emit machine-learning", got)
	}

	// Résumé-scoped acronym RAG → rag, ONLY under WithResumeAcronyms.
	if got := Parse("Built RAG pipelines over pgvector", WithResumeAcronyms()); !has(got, "rag") {
		t.Errorf("Parse RAG (resume) = %v, want rag", got)
	}
	// Default (job) parsing must NOT tag RAG — this is what protects the existing
	// "RAG status" job guard and keeps job facets unchanged.
	if got := Parse("Built RAG pipelines over pgvector"); has(got, "rag") {
		t.Errorf("Parse RAG (default) = %v, must not emit rag", got)
	}
	if got := Parse("We report RAG status weekly"); has(got, "rag") {
		t.Errorf("Parse 'RAG status' (default) = %v, must not emit rag", got)
	}
}

// New tech/methodology vocabulary (skills-vocab-gaps): each resolves from a
// realistic description; bare "rest" never tags (only "REST API"/"RESTful").
func TestParse_NewTechVocab(t *testing.T) {
	contains := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	cases := []struct {
		name   string
		in     string
		want   []string // all must be present
		absent []string // none may be present
	}{
		{"methodologies", "We work in an Agile environment using Scrum and Kanban.", []string{"agile", "scrum", "kanban"}, nil},
		{"platforms", "Experience with Salesforce, SAP and Oracle is required.", []string{"salesforce", "sap", "oracle"}, nil},
		{"practices", "You will own our DevOps and observability across microservices.", []string{"devops", "observability", "microservices"}, nil},
		{"microservice singular", "Designing a microservice architecture.", []string{"microservices"}, nil},
		{"rest via restful", "Build RESTful APIs in Go.", []string{"rest"}, nil},
		{"rest via rest api", "You will design a REST API.", []string{"rest"}, nil},
		{"power bi", "Dashboards in Power BI.", []string{"powerbi"}, nil},
		{"rest trap", "You will support the rest of the team and ship features.", nil, []string{"rest"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

// LLM-mined vocabulary (skilltag-llm-mined): high-frequency gaps found by mining
// the enrichment discovery signal (jobs.enrichment->skills). Each resolves from
// realistic text; the ambiguity guards confirm the tokens we deliberately did NOT
// add (plc, temporal, embedded, nomad) never tag on their common non-tech uses.
func TestParse_LLMMinedVocab(t *testing.T) {
	contains := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"data tooling", "ETL into NoSQL stores; dashboards in Qlik with SAS and SPSS.",
			[]string{"etl", "nosql", "qlik", "sas", "spss"}, nil},
		{"saas platforms", "Administer ServiceNow, SharePoint, HubSpot and GitHub.",
			[]string{"servicenow", "sharepoint", "hubspot", "github"}, nil},
		{"search engines", "Full-text search over OpenSearch and Solr.", []string{"opensearch", "solr"}, nil},
		{"cad + hardware", "AutoCAD and Revit for CAD; FPGA design in Verilog.",
			[]string{"autocad", "revit", "cad", "fpga", "verilog"}, nil},
		{"python viz/ml", "Plotting with Matplotlib, Seaborn and Plotly in Jupyter; XGBoost models; VBA macros.",
			[]string{"matplotlib", "seaborn", "plotly", "jupyter", "xgboost", "vba"}, nil},
		{"web3", "Smart contracts on Ethereum with a modern Web3 stack.", []string{"ethereum", "web3"}, nil},
		{"phrases", "Azure DevOps pipelines, distributed systems, deep learning and prompt engineering.",
			[]string{"azure-devops", "distributed-systems", "deep-learning", "prompt-engineering"}, nil},
		{"spring boot via word", "Spring Boot microservices.", []string{"spring"}, nil},
		{"ms low-code", "Automate with Power Apps and Power Automate.", []string{"powerapps", "power-automate"}, nil},
		{"data phrases", "Strong data modeling and data visualization skills.", []string{"data-modeling", "data-visualization"}, nil},
		{"generative ai", "Experience with Generative AI tooling.", []string{"generative-ai"}, nil},
		{"itil + six sigma", "ITIL service management and Six Sigma.", []string{"itil", "six-sigma"}, nil},
		{"security", "Cybersecurity and incident response.", []string{"cybersecurity"}, nil},
		// ambiguity guards: deliberately-omitted tokens must not tag on common non-tech uses.
		{"plc company suffix trap", "Join Barclays PLC, a FTSE-100 firm.", nil, []string{"plc"}},
		{"temporal adjective trap", "Analyze temporal trends in the data.", nil, []string{"temporal"}},
		{"embedded phrase trap", "You will be embedded in a cross-functional team.", nil, []string{"embedded"}},
		{"nomad trap", "A remote-first company hiring digital nomads.", nil, []string{"nomad"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

// AI/LLM engineering vocabulary (ai-skills-gaps): vector DBs, LLM frameworks,
// model providers, and serving/inference tools resolve from realistic text.
// Ambiguous bare words are deliberately NOT tagged (RAG project status, the word
// "bedrock", the "needle in a haystack" idiom) — those collide with non-AI jobs.
func TestParse_AIVocab(t *testing.T) {
	contains := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"vector dbs", "Vector search over Pinecone, Weaviate, Qdrant, Milvus, pgvector, FAISS and ChromaDB.",
			[]string{"pinecone", "weaviate", "qdrant", "milvus", "pgvector", "faiss", "chromadb"}, nil},
		{"chroma db spaced", "We store embeddings in Chroma DB.", []string{"chromadb"}, nil},
		{"llm frameworks", "Orchestrate agents with LangGraph, LangSmith, LlamaIndex, CrewAI and AutoGen.",
			[]string{"langgraph", "langsmith", "llamaindex", "crewai", "autogen"}, nil},
		{"framework spacing", "Built on Llama Index and Crew AI.", []string{"llamaindex", "crewai"}, nil},
		{"semantic kernel", "Microsoft Semantic Kernel experience.", []string{"semantic-kernel"}, nil},
		{"providers", "Integrate Anthropic, Cohere, Mistral and Ollama models.",
			[]string{"anthropic", "cohere", "mistral", "ollama"}, nil},
		{"clouds", "Deploy on Databricks, AWS SageMaker, Vertex AI and AWS Bedrock.",
			[]string{"databricks", "sagemaker", "vertex-ai", "aws-bedrock"}, nil},
		{"amazon bedrock", "Using Amazon Bedrock for inference.", []string{"aws-bedrock"}, nil},
		{"serving", "Serve models with vLLM, Triton, TensorRT, ONNX and BentoML.",
			[]string{"vllm", "triton", "tensorrt", "onnx", "bentoml"}, nil},
		{"training", "Fine-tuning with DeepSpeed and PEFT; sentence-transformers for embeddings.",
			[]string{"deepspeed", "peft", "sentence-transformers"}, nil},
		{"cv model", "Object detection with YOLO.", []string{"yolo"}, nil},
		{"rag phrase", "Build retrieval augmented generation pipelines.", []string{"rag"}, nil},
		// ambiguity guards: these must NOT tag (skilltag runs on ALL jobs).
		{"rag status trap", "We report project health as RAG status weekly.", nil, []string{"rag"}},
		{"bedrock word trap", "Trust is the bedrock of our company culture.", nil, []string{"aws-bedrock"}},
		{"haystack idiom trap", "Finding signal is a needle in a haystack.", nil, []string{"haystack"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

// TestParse_ExpansionBatch1 covers the security/qa/data skill additions (diffed
// against the Lightcast ATS taxonomy). Positives assert the new canonicals tag;
// negatives assert the ambiguous-word skills route ONLY via a qualifying phrase,
// so a bare English use ("a druid", "a burp") never misfires — the precision-first
// doctrine the dictionary already applies to c/r/go.
func TestParse_ExpansionBatch1(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		// security
		{"scanners", "We run Metasploit, Nmap and Wireshark.", []string{"metasploit", "nmap", "wireshark"}},
		{"appsec tools", "OWASP guidelines, Nessus scans, Burp Suite pentest.", []string{"burp-suite", "nessus", "owasp", "penetration-testing"}},
		{"auth stack", "Auth: OAuth2, SAML, JWT, Keycloak, OpenID Connect.", []string{"jwt", "keycloak", "oauth", "openid", "saml"}},
		// qa
		{"load + browser + frameworks", "Load: k6, Gatling, JMeter. Browser: Puppeteer, Appium. Frameworks: TestNG, Robot Framework, SoapUI.",
			[]string{"appium", "gatling", "jmeter", "k6", "puppeteer", "robot-framework", "soapui", "testng"}},
		// data — ELT/warehouse
		{"elt pipeline", "Airbyte, Fivetran, Dagster, Debezium into Metabase; HBase, Apache NiFi.",
			[]string{"airbyte", "dagster", "debezium", "fivetran", "hbase", "metabase", "nifi"}},
		// data — query engines routed via phrase (bare token is ambiguous)
		{"query engines via phrase", "Apache Druid, Apache Pinot, PrestoDB, Delta Lake, Apache Iceberg.",
			[]string{"delta-lake", "druid", "iceberg", "pinot", "presto"}},

		// precision negatives — ambiguous words must NOT tag without their qualifier
		{"bare ambiguous words do not tag", "The druid drank pinot noir, said presto, and admired the iceberg.", nil},
		{"burp is not a skill", "He let out a loud burp.", nil},
		{"jasmine and karma are not added", "Jasmine watered the plants with good karma.", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParse_ExpansionBatch2 covers the LLM-mined batch (jobs.enrichment->skills,
// freq >= 1500): infra/network/security tokens, data & AI concepts, and multi-word
// routes. Distinctive lowercase tokens tag directly; multi-word terms route via
// phrases. Negatives assert the deliberately-omitted ambiguous tokens (windows,
// http-in-URLs, s3) never misfire — skilltag runs on ALL jobs, IT and non-IT.
func TestParse_ExpansionBatch2(t *testing.T) {
	contains := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"network + virt", "Managed Active Directory, DNS, DHCP and VPN across a VMware estate.",
			[]string{"active-directory", "dns", "dhcp", "vpn", "vmware"}, nil},
		{"security stack", "SIEM and EDR feed our cloud security; we enforce zero trust and RBAC with SSO via IAM.",
			[]string{"siem", "edr", "cloud-security", "zero-trust", "rbac", "sso", "iam"}, nil},
		{"data platform", "Built ETL data pipelines feeding a data warehouse; strong data engineering and data science.",
			[]string{"data-pipelines", "data-warehousing", "data-engineering", "data-science"}, nil},
		{"iac ops", "Infrastructure as Code with GitOps and MLOps on a Unix host.",
			[]string{"infrastructure-as-code", "gitops", "mlops", "unix"}, nil},
		{"iac acronym", "We practice IaC and DevSecOps.", []string{"infrastructure-as-code", "devsecops"}, nil},
		{"protocols + formats", "A REST API returning JSON and XML over TCP/IP and Ethernet.",
			[]string{"json", "xml", "tcp-ip", "ethernet"}, nil},
		{"testing practices", "Unit testing and TDD; test automation, automated testing and A/B testing.",
			[]string{"unit-testing", "tdd", "test-automation", "ab-testing"}, nil},
		{"databases", "SQL Server with T-SQL, and PL/SQL on legacy systems.",
			[]string{"sql-server", "plsql"}, nil},
		{"mssql token", "Deep MSSQL experience.", []string{"sql-server"}, nil},
		{"google cloud + analytics", "Google Cloud Platform, Google Analytics (GA4) and a cloud native architecture.",
			[]string{"gcp", "google-analytics", "cloud-native"}, nil},
		{"ai concepts", "Computer vision and agentic AI backed by vector databases.",
			[]string{"computer-vision", "agentic-ai", "vector-databases"}, nil},
		{"agentic word", "Building agentic workflows.", []string{"agentic-ai"}, nil},
		{"llms plural to llm", "Fine-tuning LLMs for production.", []string{"llm"}, nil},
		{"code craft", "OOP and design patterns; version control with Bitbucket.",
			[]string{"oop", "design-patterns", "version-control", "bitbucket"}, nil},
		{"pentest word", "Pentesting web apps.", []string{"penetration-testing"}, nil},
		{"covered variants html/css", "HTML5 and CSS3 responsive layout.", []string{"html", "css"}, nil},
		{"covered nlp phrase", "Natural language processing pipelines.", []string{"nlp"}, nil},
		{"covered ci-cd phrase", "Continuous integration and continuous delivery.", []string{"ci-cd"}, nil},
		{"covered gcp phrase", "Deploy to Google Cloud.", []string{"gcp"}, nil},
		// web3 / crypto
		{"web3 stack", "Solidity smart contracts on Ethereum and Solana; DeFi and tokenomics.",
			[]string{"solidity", "smart-contracts", "ethereum", "solana", "defi", "tokenomics"}, nil},
		{"evm + ethers", "EVM chains with ethers.js and a cryptocurrency wallet backend.",
			[]string{"evm", "ethersjs", "cryptocurrency"}, nil},
		// web3 false-friend guards: these tokens are NOT crypto in context and must NOT tag.
		{"cosmos db not crypto", "Store documents in Azure Cosmos DB.", nil, []string{"cosmos"}},
		{"foundry not crypto", "Analytics on Palantir Foundry and Cloud Foundry.", nil, []string{"foundry"}},
		{"rollup bundler not crypto", "Bundle the app with Rollup and Vite.", nil, []string{"rollup"}},
		{"nftables not nft", "Configure the firewall with nftables.", nil, []string{"nft", "nftables"}},
		{"defi does not match definition", "Gather the requirements definition first.", nil, []string{"defi"}},
		// ambiguity guards: deliberately-omitted tokens must NOT tag on common uses.
		{"windows office trap", "A bright office with big windows and a great team.", nil, []string{"windows"}},
		{"http url trap", "Apply via http://careers.example.com by Friday.", nil, []string{"http"}},
		{"s3 trap", "We finished the S3 phase of the roadmap.", nil, []string{"s3"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

// TestParse_ExpansionBatch3 covers the LLM-mined batch 2 (freq 500-1500): named
// network/security/infra tools, SaaS/eng products, and multi-word ml/security
// concepts. Negatives lock in the ultra-generic words we deliberately left OUT
// (caching, routing, concurrency, load-balancing, https-in-URLs) so a later editor
// doesn't quietly add them.
func TestParse_ExpansionBatch3(t *testing.T) {
	contains := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"routing protocols", "Networking with OSPF, BGP, VLAN, MPLS and IPsec over a VPC.",
			[]string{"ospf", "bgp", "vlan", "mpls", "ipsec", "vpc"}, nil},
		{"security tooling", "Auth via LDAP and Okta; WAF, DLP and SAST in the pipeline.",
			[]string{"ldap", "okta", "waf", "dlp", "sast"}, nil},
		{"linux ops", "Admin RHEL and Ubuntu; monitoring with Zabbix and Dynatrace.",
			[]string{"rhel", "ubuntu", "zabbix", "dynatrace"}, nil},
		{"ml concepts", "Reinforcement learning and neural networks for recommendation systems.",
			[]string{"reinforcement-learning", "neural-networks", "recommendation-systems"}, nil},
		{"data science phrases", "Time series feature engineering and anomaly detection.",
			[]string{"time-series", "feature-engineering", "anomaly-detection"}, nil},
		{"retrieval", "Vector search and semantic search over embeddings.",
			[]string{"vector-search", "semantic-search", "embeddings"}, nil},
		{"data eng", "A data lake with data lineage, built in Alteryx.",
			[]string{"data-lake", "data-lineage", "alteryx"}, nil},
		{"automation saas", "Build on Firebase; automate with Zapier and n8n.",
			[]string{"firebase", "zapier", "n8n"}, nil},
		{"ms products", "Visual Studio, Microsoft Access and the Power Platform.",
			[]string{"visual-studio", "microsoft-access", "power-platform"}, nil},
		{"azure identity", "Azure AD (Entra ID), Azure Functions and Azure Data Factory.",
			[]string{"azure-ad", "entra-id", "azure-functions", "azure-data-factory"}, nil},
		{"sap", "SAP S/4HANA and SAP MM migration.", []string{"sap-s4hana", "sap-mm"}, nil},
		{"security concepts", "Threat hunting, intrusion detection and secure coding.",
			[]string{"threat-hunting", "intrusion-detection", "secure-coding"}, nil},
		{"compliance frameworks", "ISO 27001 and SOC 2 compliance; MITRE ATT&CK mapping.",
			[]string{"iso-27001", "soc-2", "mitre-attack"}, nil},
		{"embedded", "Embedded Linux on microcontrollers with an RTOS.",
			[]string{"embedded-linux", "microcontrollers", "rtos"}, nil},
		{"transport security", "Traffic over TLS/SSL with SFTP and SSH.",
			[]string{"tls", "ssl", "sftp", "ssh"}, nil},
		// negatives: ultra-generic words we deliberately did NOT add must never tag.
		{"generic words omitted", "A focus on caching, routing, concurrency and load balancing.",
			nil, []string{"caching", "routing", "concurrency", "load-balancing"}},
		{"https url not tagged", "Read the docs at https://example.com/guide.", nil, []string{"https"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}
