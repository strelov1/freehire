package skilltag

import (
	"reflect"
	"slices"
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
func TestParse_ITCompanyRoleSkills(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"recruiting", "Sourcing with boolean search and employer branding", []string{"boolean-search", "employer-branding"}},
		{"finance", "Own financial modeling and revenue recognition in NetSuite", []string{"financial-modeling", "netsuite", "revenue-recognition"}},
		{"business analysis", "Requirements gathering, process modeling and user stories", []string{"process-modeling", "requirements-gathering", "user-stories"}},
		{"customer success", "Drive customer onboarding and churn prevention via Gainsight", []string{"churn-prevention", "customer-onboarding", "gainsight"}},
		{"technical writing", "API documentation with a docs-as-code workflow", []string{"api", "api-documentation", "docs-as-code"}},
		{"unknown emits nothing", "great communicator and team player", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// Boilerplate sections — the pay-transparency block, the GDPR notice, the ATS
// footer — appear in postings for EVERY role, so a dictionary term that matches
// them tags the whole corpus instead of describing the job (an ML Platform
// Engineer came back with "compensation-and-benefits"). Such a term is not a
// requirement signal at all, so it is out of the vocabulary entirely; the same
// applies to an ATS name whose bare token doubles as an English word
// ("a key lever", "a flexible workday").
func TestParse_BoilerplateSectionsAreNotSkills(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"pay transparency block", "Compensation and Benefits: we offer a competitive salary and equity.", nil},
		{"privacy notice", "Data Privacy Notice: we process your application data as described here.", nil},
		{"ats footer", "Powered by Greenhouse. Apply via Lever.", nil},
		{"culture blurb", "We invest in employee engagement and do our due diligence.", nil},
		{"boilerplate does not pollute a real stack", "We use Python and Kafka. A flexible workday, applications via Greenhouse.", []string{"kafka", "python"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParse_AccentedWordsAreNotSkills pins the two places a word boundary is
// decided — the phrase pass (wordmatch) and the word pass (wordTokens) — against
// text whose letters are not ASCII. Both read a curated alias out of the middle of
// an inflected foreign word before this: the Hungarian "elkészítése" ("preparing
// it") yielded elk, on 18 of 110 postings sampled from a live Hungarian IT crawl.
// The failure is not Hungarian-specific — any diacritic or non-Latin script ends an
// ASCII-only token the same way.
func TestParse_AccentedWordsAreNotSkills(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"hungarian inflection yields no elk", "A dokumentáció elkészítése a feladatod.", nil},
		{"hungarian inflection beside a real stack", "Rendszertervek elkészítése Python nyelven.", []string{"python"}},
		{"a real ELK mention still tags", "ELK stack üzemeltetése", []string{"elk"}},
		{"lowercase elk as a whole word still tags", "we run elk for logging", []string{"elk"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

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

// Category-scoped acronym pass (refine-ai-role-classification): RAG resolves on
// job text when the caller supplies a job category on the acronym's own
// allow-list (ai_engineering, ml_ai), and does not resolve otherwise — the
// category already evidences an AI-flavored posting, so it substitutes for
// corroboration without reopening the "RAG status" collision catalogue-wide.
func TestParse_CategoryScopedAcronyms(t *testing.T) {
	has := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}

	if got := Parse("Built RAG pipelines over pgvector", WithAcronymCategory("ai_engineering")); !has(got, "rag") {
		t.Errorf("Parse RAG (category ai_engineering) = %v, want rag", got)
	}
	if got := Parse("Built RAG pipelines over pgvector", WithAcronymCategory("ml_ai")); !has(got, "rag") {
		t.Errorf("Parse RAG (category ml_ai) = %v, want rag", got)
	}
	if got := Parse("We report RAG status weekly", WithAcronymCategory("backend")); has(got, "rag") {
		t.Errorf("Parse 'RAG status' (category backend) = %v, must not emit rag", got)
	}
	if got := Parse("Built RAG pipelines over pgvector"); has(got, "rag") {
		t.Errorf("Parse RAG (no category option) = %v, must not emit rag", got)
	}
}

// Agile/PM certification acronyms (role-taxonomy-alias-gaps): CSM, PSM, PMP and
// SAFe each collide with another meaning in general job text (Customer Success
// Manager, common English word, other short-string noise), so they resolve only
// on job text already categorized project_management — the same category-scoped
// shape as RAG. The spelled-out phrase forms are unambiguous and resolve
// regardless of category.
func TestParse_AgilePMCertificationAcronyms(t *testing.T) {
	has := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}

	cases := []struct {
		text      string
		canonical string
	}{
		{"CSM certification required", "certified-scrummaster"},
		{"PSM I certified", "professional-scrum-master"},
		{"PMP holders preferred", "pmp"},
		{"SAFe experience a plus", "safe-agile"},
		{"SAFE experience a plus", "safe-agile"}, // ATS all-caps title rendering
	}
	for _, c := range cases {
		if got := Parse(c.text, WithAcronymCategory("project_management")); !has(got, c.canonical) {
			t.Errorf("Parse(%q, project_management) = %v, want %s", c.text, got, c.canonical)
		}
		if got := Parse(c.text, WithAcronymCategory("backend")); has(got, c.canonical) {
			t.Errorf("Parse(%q, backend) = %v, must not emit %s", c.text, got, c.canonical)
		}
		if got := Parse(c.text); has(got, c.canonical) {
			t.Errorf("Parse(%q, no category) = %v, must not emit %s", c.text, got, c.canonical)
		}
	}

	// Spelled-out phrase forms are unambiguous strong matches, no category needed.
	phraseCases := []struct{ text, canonical string }{
		{"Certified ScrumMaster (CSM)", "certified-scrummaster"},
		{"Certified Scrum Master (CSM)", "certified-scrummaster"},
		{"Holds a Professional Scrum Master certification", "professional-scrum-master"},
		{"Project Management Professional preferred", "pmp"},
		{"Scaled Agile Framework experience", "safe-agile"},
	}
	for _, c := range phraseCases {
		if got := Parse(c.text); !has(got, c.canonical) {
			t.Errorf("Parse(%q) = %v, want %s", c.text, got, c.canonical)
		}
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
		{"mcp", "Experience building tools with MCP and LangGraph.", []string{"mcp"}, nil},
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
		// URL path extensions must not leak as skills on non-tech jobs: a retail post
		// linking to "about-us.html" (or a ".php" page) tokenizes to html/php otherwise.
		{"html extension in link text", "Learn more at www.dollargeneral.com/about-us.html today.", nil, []string{"html"}},
		{"php extension in url", "See https://jobs.example.com/apply.php for details.", nil, []string{"php"}},
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

// Ambiguous English words that double as tech names (react/swift/rust/spring/…)
// must only tag when the same text carries an unambiguous tech token
// (corroboration). This kills the dominant false-positive class — a non-tech post
// that merely says "must react to changes" or "strong networking skills" — while a
// real posting that also names its stack keeps the tag. Unambiguous alias forms
// (reactjs, "react native", "spring boot") always tag, no corroboration needed.
func TestParse_AmbiguousCorroboration(t *testing.T) {
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
		// bare ambiguous word, NO unambiguous tech token → dropped
		{"react verb (cook)", "Must be able to react to changes in schedules.", nil, []string{"react"}},
		{"networking soft skill", "Strong networking and negotiation skills.", nil, []string{"networking"}},
		{"spring season", "Spring and summer seasonal cleaning crew.", nil, []string{"spring"}},
		{"rust corrosion", "Inspect the railings for rust and repaint.", nil, []string{"rust"}},
		{"swift adjective", "We need a swift and friendly barista.", nil, []string{"swift"}},
		{"ruby name", "Report directly to Ruby, the floor manager.", nil, []string{"ruby"}},
		{"cloud weather", "Outdoor role, rain or cloud, year round.", nil, []string{"cloud"}},
		// broad concepts alone (non-tech context) → dropped
		{"ai marketing", "AI-powered marketing automation for our sales team.", nil, []string{"ai", "automation"}},
		{"crm sales role", "Manage our CRM as an account executive.", nil, []string{"crm"}},
		{"broad concepts don't self-corroborate", "AI, automation and analytics for retail.", nil, []string{"ai", "automation", "analytics"}},
		// broad concept + concrete tech → kept
		{"ai + pytorch", "AI models with PyTorch and Python.", []string{"ai", "pytorch", "python"}, nil},
		// corroborated by an unambiguous tech token → kept
		{"react + typescript", "React and TypeScript for our UI.", []string{"react", "typescript"}, nil},
		{"networking + linux", "Networking with Linux, BGP and firewalls.", []string{"networking", "linux", "bgp"}, nil},
		{"swift + ios", "Swift developer building for iOS.", []string{"swift", "ios"}, nil},
		{"cloud + aws", "Cloud infrastructure on AWS and Kubernetes.", []string{"cloud", "aws", "kubernetes"}, nil},
		// unambiguous alias forms tag WITHOUT corroboration
		{"reactjs standalone", "We build with reactjs.", []string{"react"}, nil},
		{"react native phrase", "React Native mobile apps.", []string{"react-native", "react"}, nil},
		{"spring boot phrase", "Spring Boot services.", []string{"spring"}, nil},
		{"ruby on rails phrase", "A Ruby on Rails shop.", []string{"ruby", "rails"}, nil},
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

// Job-posting boilerplate that happens to spell a tech name. Unlike the collisions
// above — which read as ordinary English anywhere — these are the words that recur in
// the prose of NON-TECHNICAL postings specifically: an "agile" retail supervisor, the
// drivers who are "the backbone" of a haulier, a "restful" night in a care home, the
// factory "assembly" line, the rugby "scrum", tree "sap" on a guard rail, a "sentry"
// post, a bank "vault", the fire-code "firewall", a "rancher", a "postman", "braze"d
// copper joints.
//
// Each was a strong alias, so one of them alone tagged the posting — and worse, a
// strong match lifts the corroboration gate off EVERY weak word in the same text, so a
// single boilerplate hit also legitimises the react/rust/spring collisions beside it.
// That is how a Highway Maintenance Supervisor came out of the pipeline tagged
// {react, sap}. Measured on prod, 20855 boards holding 475625 postings were kept off
// the retirement report by the skills dictionary alone, with no technical posting
// anywhere.
//
// Gating them costs nothing a real posting needs: a genuine SAP role names ABAP, a
// REST role says "REST API" (a strong phrase), a security role names its stack.
func TestParse_BoilerplateWordsNeedCorroboration(t *testing.T) {
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
		// Boilerplate in non-technical prose, no other tech token → dropped.
		{"agile as a temperament", "We want an agile, hands-on retail supervisor.", nil, []string{"agile"}},
		{"backbone of the team", "Our drivers are the backbone of this haulage firm.", nil, []string{"backbone"}},
		{"restful care home", "Help residents enjoy a restful night in our care home.", nil, []string{"rest"}},
		{"assembly line", "Work the assembly line fitting door panels.", nil, []string{"assembly"}},
		{"rugby scrum", "Coach the scrum and play tight-head prop.", nil, []string{"scrum"}},
		{"tree sap", "Clear vegetation and wash tree sap off the guard rails.", nil, []string{"sap"}},
		{"sentry duty", "Stand sentry at the north gate on night shifts.", nil, []string{"sentry"}},
		{"bank vault", "Count the drawer and lock the vault at close.", nil, []string{"vault"}},
		{"fire-code firewall", "Inspect the firewall between the garage and the dwelling.", nil, []string{"firewall"}},
		{"ranch hand", "Assist the rancher with calving and fence repair.", nil, []string{"rancher"}},
		{"postal round", "Cover the postman's round while they are on leave.", nil, []string{"postman"}},
		{"brazing copper", "Braze copper joints on refrigeration units.", nil, []string{"braze"}},
		// Corroborated by an unambiguous tech token → kept, exactly as before.
		{"agile + jira", "Agile delivery tracked in Jira and Kubernetes.", []string{"agile", "jira", "kubernetes"}, nil},
		{"sap + abap", "SAP ABAP developer for our finance modules.", []string{"sap", "abap"}, nil},
		{"assembly + c", "Assembly and C programming for embedded targets.", []string{"assembly", "c"}, nil},
		{"firewall + linux", "Configure firewalls and BGP on Linux.", []string{"firewall", "bgp", "linux"}, nil},
		{"sentry + typescript", "Error tracking with Sentry in a TypeScript app.", []string{"sentry", "typescript"}, nil},
		{"vault + kubernetes", "HashiCorp Vault secrets on Kubernetes.", []string{"vault", "kubernetes"}, nil},
		{"scrum + jira", "Scrum ceremonies tracked in Jira.", []string{"scrum", "jira"}, nil},
		{"rancher + kubernetes", "Rancher-managed Kubernetes clusters.", []string{"rancher", "kubernetes"}, nil},
		{"backbone + jquery", "A legacy Backbone and jQuery front end.", []string{"backbone", "jquery"}, nil},
		{"braze + typescript", "Braze lifecycle campaigns instrumented in TypeScript.", []string{"braze", "typescript"}, nil},
		// The unambiguous phrase form still tags on its own — "REST API" is not prose.
		{"rest api phrase", "Design REST APIs in Python.", []string{"rest", "python"}, nil},
		{"postman + rest api", "API testing with Postman against our REST APIs.", []string{"postman", "rest"}, nil},
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

// Every ambiguousWords key must be a real wordAliases entry — an ambiguous marker
// on a token the word pass never emits would be dead config.
func TestAmbiguousWordsSubsetOfAliases(t *testing.T) {
	for w := range ambiguousWords {
		if _, ok := wordAliases[w]; !ok {
			t.Errorf("ambiguousWords[%q] has no wordAliases entry", w)
		}
	}
}

// The design craft's toolchain and the CAD/EDA stack the engineering-design side
// states. Both were nearly absent: the dictionary knew figma, photoshop and autocad
// and little else, so a designer's description came back with one tag or none.
//
// Three aliases stay OUT deliberately — "principle" (ordinary English), "eagle"
// (the bird, and "eagle-eyed" is posting boilerplate) and "nx" (Siemens NX vs. the
// Nx JS monorepo tool). Two go in behind the corroboration gate instead of being
// dropped: "sketch" ("sketch out ideas") and "maya" (a person's name).
func TestParse_DesignAndCADVocab(t *testing.T) {
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
		// design tools
		{"adobe suite", "You will work in Figma and Adobe Illustrator, with InDesign for print.",
			[]string{"figma", "illustrator", "indesign"}, nil},
		{"bare illustrator", "Strong Illustrator and Photoshop skills.", []string{"illustrator", "photoshop"}, nil},
		{"after effects", "Motion work in Adobe After Effects and Premiere Pro.", []string{"after-effects", "premiere-pro"}, nil},
		// "after effects" unqualified is clinical prose, and healthcare is the largest
		// non-technical mass this catalogue filters. A false STRONG token would also
		// have lifted the gate off the weak words beside it — and HasEngineering would
		// have read a nursing board as having posted engineering work.
		{"after effects of anaesthesia",
			"Registered Nurse. Monitor patients for the after effects of anaesthesia; sketch out care plans.",
			nil, []string{"after-effects", "sketch"}},
		{"a solid edge over the competition",
			"Account Executive. Our platform gives clients a solid edge over the competition. Sketch out the territory plan.",
			nil, []string{"solid-edge", "sketch"}},
		{"invision as a misspelling of envision",
			"We invision a workplace where everyone belongs. Sketch out your growth plan.",
			nil, []string{"invision", "sketch"}},
		{"menu prototyping in a kitchen",
			"Line Cook. Menu prototyping with the chef; sketch out plating ideas.",
			nil, []string{"prototyping", "sketch"}},
		{"adobe xd", "Wireframes in Adobe XD.", []string{"adobe-xd", "wireframing"}, nil},
		{"no-code design", "Ship marketing pages in Webflow.", []string{"webflow"}, nil},
		{"handoff tools", "Design handoff through InVision and Zeplin, source in Figma.",
			[]string{"invision", "zeplin", "figma"}, nil},
		{"3d design", "3D assets in Blender, animations with Lottie built in Figma.",
			[]string{"blender", "lottie", "figma"}, nil},
		// design practices
		{"practices", "You will own prototyping and wireframing in Figma.",
			[]string{"prototyping", "wireframing", "figma"}, nil},
		// "design system(s)" is NOT a canonical: it is also the ordinary verb phrase
		// every backend and embedded posting writes ("you will design systems that
		// scale"). A phrase match is always strong, so tagging it would ALSO lift the
		// corroboration gate off the weak words beside it.
		{"design systems as a verb", "You will design systems that scale to millions of requests.",
			nil, []string{"design-systems"}},
		{"design systems verb does not lift the gate",
			"Backend engineer: you will design systems and sketch out solutions with the team.",
			nil, []string{"design-systems", "sketch"}},
		{"design systems architecture is still not it",
			"You will design system architecture for our microservices.", nil, []string{"design-systems"}},
		{"research practices", "Run user research and usability testing with our PMs.",
			[]string{"user-research", "usability-testing"}, nil},
		{"craft practices", "Interaction design and typography matter here, alongside Figma.",
			[]string{"interaction-design", "typography", "figma"}, nil},
		{"design thinking", "We practise design thinking end to end.", []string{"design-thinking"}, nil},
		{"motion", "Motion design and motion graphics for product launches.",
			[]string{"motion-design", "motion-graphics"}, nil},
		// CAD / EDA
		{"mechanical cad", "3D modelling in SolidWorks and PTC Creo, drawings in AutoCAD.",
			[]string{"solidworks", "creo", "autocad"}, nil},
		{"more cad", "Experience with CATIA, SketchUp and Autodesk Inventor.",
			[]string{"catia", "sketchup", "autodesk-inventor"}, nil},
		{"eda", "PCB layout in Altium and KiCad; simulation in ANSYS.",
			[]string{"altium", "kicad", "ansys"}, nil},
		{"cad phrases", "Drafting in 3ds Max, Fusion 360 and Civil 3D.",
			[]string{"3ds-max", "fusion-360", "civil-3d"}, nil},
		// Trade and prose collisions that keep whole products out of the dictionary.
		// A carpentry "framer", the Spanish "creo que", and the splined shafts of the
		// very mechanical population this change splits out would each have tagged a
		// posting with a design tool it never mentions — and a false STRONG token also
		// lifts the corroboration gate off every weak word beside it.
		{"framer the carpenter", "Framer needed for residential construction crews.", nil, []string{"framer"}},
		{"framer motion is not the design tool", "Frontend dev: animate with Framer Motion.", nil, []string{"framer"}},
		{"creo in spanish prose", "Creo que este puesto es para ti.", nil, []string{"creo"}},
		{"splined shafts", "Design splined shafts and spline broaching fixtures for gearboxes.", nil, []string{"spline"}},
		// homonyms: uncorroborated prose must stay silent
		{"blender in a kitchen", "Line cook: operate the industrial blender and mixer.", nil, []string{"blender"}},
		{"sketch verb", "You will sketch out ideas with the team each morning.", nil, []string{"sketch"}},
		{"principle noun", "Our guiding principle is respect for the customer.", nil, []string{"principle"}},
		{"eagle-eyed", "We need an eagle-eyed proofreader for our brochures.", nil, []string{"eagle"}},
		{"maya the person", "You will report to Maya, our store manager.", nil, []string{"maya"}},
		// homonyms: corroborated by a strong design token → tagged
		{"sketch corroborated", "Our team designs in Figma and Sketch.", []string{"figma", "sketch"}, nil},
		// Two gated words do NOT corroborate each other — a strong token has to be
		// present, exactly as for the broad concepts.
		{"maya and blender corroborated", "Character rigging in Maya and Blender, textures in Photoshop.",
			[]string{"maya", "blender", "photoshop"}, nil},
		{"gated words alone stay silent", "Sculpting in Maya and Blender.", nil, []string{"maya", "blender"}},
		// A design word that is ALSO ordinary prose cannot be a strong token either —
		// otherwise it corroborates the gated words and the gate is decorative. These
		// four join the gate for that reason, each probed in its real false context.
		{"typography in retail prose", "Retail assistant. Typography of the shelf labels matters; sketch out ideas for displays.",
			nil, []string{"typography", "sketch"}},
		{"illustrator the occupation", "Hiring an Illustrator for our children's book imprint.",
			nil, []string{"illustrator"}},
		{"canva in a store posting", "Store manager. Use Canva for posters. Accessibility of the entrance is your duty.",
			nil, []string{"canva", "accessibility"}},
		{"lottie the given name", "You will report to Lottie, our head of retail operations. Sketch out weekly rotas.",
			nil, []string{"lottie", "sketch"}},
		{"wireframes in cad prose", "CNC operator: read wireframes and 2D drawings before fabrication.",
			nil, []string{"wireframing"}},
		// accessibility is a broad word, so it needs corroboration too
		{"accessibility alone", "An accessibility ramp is available at the entrance.", nil, []string{"accessibility"}},
		{"accessibility corroborated", "Accessibility work in Figma, meeting WCAG 2.2.",
			[]string{"accessibility", "figma", "wcag"}, nil},
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

// The tail of the design/CAD vocabulary: the tools and phrases the first batch of
// cases did not reach, kept as their own case list so each alias has a witness.
func TestParse_DesignAndCADVocabTail(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Interactive prototypes in ProtoPie.", "protopie"},
		{"Social assets in Canva, source files in Figma.", "canva"},
		{"Workshops run in FigJam.", "figjam"},
		{"We hold ourselves to a11y standards with Figma.", "accessibility"},
		{"Assemblies modelled in Siemens NX.", "siemens-nx"},
		{"Analog layout in Cadence Virtuoso.", "cadence-virtuoso"},
		{"Parametric modelling in Creo Parametric.", "creo"},
		{"You will run ux research sessions with Figma prototypes.", "user-research"},
		{"You will map user flows before building.", "user-flows"},
		{"Rapid prototyping of new concepts in Figma.", "prototyping"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			got := Parse(c.in)
			for _, g := range got {
				if g == c.want {
					return
				}
			}
			t.Errorf("Parse(%q) = %v, missing %q", c.in, got, c.want)
		})
	}
}

// TestParse_SEOTooling covers the search-optimization toolchain. The corroboration
// case is the point of the block: `seo` is an ambiguousWords member, so it is
// dropped unless the same text carries a strong token. Before these entries a
// posting that named its whole toolchain but no engineering technology corroborated
// nothing and lost even the `seo` tag it obviously deserved.
func TestParse_SEOTooling(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string // must be present
	}{
		{"ahrefs", "Experience with Ahrefs required.", []string{"ahrefs"}},
		{"semrush", "You will own reporting in Semrush.", []string{"semrush"}},
		{"screaming frog", "Run crawls in Screaming Frog.", []string{"screaming-frog"}},
		{"google search console", "Monitor Google Search Console daily.", []string{"google-search-console"}},
		{
			"tooling corroborates the ambiguous seo canonical",
			"SEO Specialist. You will own keyword research in Ahrefs and Semrush.",
			[]string{"ahrefs", "semrush", "seo"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.text)
			for _, w := range tc.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, w)
				}
			}
		})
	}
}

// TestParse_SEOAloneStillDropped pins the other half of the corroboration rule: a
// bare mention of SEO in otherwise non-technical prose must still resolve to
// nothing. The new tooling entries must not weaken that gate.
func TestParse_SEOAloneStillDropped(t *testing.T) {
	got := Parse("A retail assistant with an interest in SEO and social media.")
	if slices.Contains(got, "seo") {
		t.Errorf("Parse(...) = %v, want no bare seo without corroboration", got)
	}
}

// TestParse_MarketingPlatforms covers the lifecycle, email and advertising
// platforms. "Google Ads" is the interesting one: the phrase pass must claim it
// before the word pass can read "google" as some other Google product.
func TestParse_MarketingPlatforms(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"klaviyo", "Own flows in Klaviyo.", []string{"klaviyo"}},
		{"mailchimp", "Campaigns in Mailchimp.", []string{"mailchimp"}},
		{"customer.io", "Journeys built in Customer.io.", []string{"customer-io"}},
		{"google ads", "Manage Google Ads budgets.", []string{"google-ads"}},
		{"meta ads", "Scale Meta Ads creative.", []string{"meta-ads"}},
		{"facebook ads is the same canonical", "Scale Facebook Ads creative.", []string{"meta-ads"}},
		{"tiktok ads", "Launch TikTok Ads campaigns.", []string{"tiktok-ads"}},
		{"linkedin ads", "Run LinkedIn Ads for ABM.", []string{"linkedin-ads"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.text)
			for _, w := range tc.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, w)
				}
			}
		})
	}
}

// TestParse_GoogleAdsDoesNotLeakAnotherGoogleProduct pins the vendor-collision
// guard: naming one Google product must not emit a different one.
func TestParse_GoogleAdsDoesNotLeakAnotherGoogleProduct(t *testing.T) {
	got := Parse("Manage Google Ads budgets across accounts.")
	for _, unwanted := range []string{"google-analytics", "gcp"} {
		if slices.Contains(got, unwanted) {
			t.Errorf("Parse(...) = %v, want no %q", got, unwanted)
		}
	}
}

// TestParse_MarketingMeasurementAndSocialTooling covers the measurement and social
// stack. Several products in this space are named after ordinary English words —
// Segment, Buffer, Later — and are deliberately absent from the vocabulary: a bare
// "segment" is a customer segment in the very postings this block serves. Amplitude
// takes the middle route, resolving only when a concrete technology corroborates it.
func TestParse_MarketingMeasurementAndSocialTooling(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"looker studio", "Dashboards in Looker Studio.", []string{"looker-studio"}},
		{"google tag manager spelled out", "Tagging via Google Tag Manager.", []string{"google-tag-manager"}},
		{"hootsuite", "Scheduling in Hootsuite.", []string{"hootsuite"}},
		{"sprout social", "Reporting from Sprout Social.", []string{"sprout-social"}},
		{"contentful", "Publish through Contentful.", []string{"contentful"}},
		{"amplitude with corroboration", "Product analytics in Amplitude, events piped via Python.", []string{"amplitude"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.text)
			for _, w := range tc.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, w)
				}
			}
		})
	}
}

// TestParse_ToolNamesThatAreOrdinaryWordsDoNotTag pins the collisions this block
// must never introduce. Each sentence is realistic marketing prose.
func TestParse_ToolNamesThatAreOrdinaryWordsDoNotTag(t *testing.T) {
	cases := []struct{ text, unwanted string }{
		{"Define the customer segment and its jobs to be done.", "segment"},
		{"Maintain a healthy content buffer of two weeks.", "buffer"},
		{"Applications reviewed now or later in the quarter.", "later"},
		{"The amplitude of seasonal demand shifts each quarter.", "amplitude"},
	}
	for _, tc := range cases {
		if got := Parse(tc.text); slices.Contains(got, tc.unwanted) {
			t.Errorf("Parse(%q) = %v, must not emit %q", tc.text, got, tc.unwanted)
		}
	}
}

// TestParse_MarketingDisciplines covers the disciplines themselves, so a search can
// combine "does technical SEO" with "uses Ahrefs". They are multi-word phrases,
// which the matcher already treats as strong — no gating needed. "SEM" is absent by
// design: it is a Portuguese and Spanish preposition ("sem experiência"), and this
// catalogue carries a large PT/ES population.
func TestParse_MarketingDisciplines(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"technical seo", "You will lead technical SEO audits.", []string{"technical-seo"}},
		{"link building", "Own link building and digital PR.", []string{"link-building"}},
		{"paid social", "Scale our paid social programme.", []string{"paid-social"}},
		{"demand generation", "Run demand generation for EMEA.", []string{"demand-generation"}},
		{"lifecycle marketing", "Own lifecycle marketing end to end.", []string{"lifecycle-marketing"}},
		{"marketing automation", "Build marketing automation flows.", []string{"marketing-automation"}},
		{"generative engine optimization", "Lead generative engine optimization efforts.", []string{"generative-engine-optimization"}},
		{"answer engine optimization is the same canonical", "Lead answer engine optimization efforts.", []string{"generative-engine-optimization"}},
		{"content marketing", "Own content marketing strategy.", []string{"content-marketing"}},
		{"email marketing", "Own email marketing and deliverability.", []string{"email-marketing"}},
		{"influencer marketing", "Run influencer marketing campaigns.", []string{"influencer-marketing"}},
		{"copywriting", "Strong copywriting skills required.", []string{"copywriting"}},
		{"ppc", "Manage PPC campaigns across channels.", []string{"ppc"}},
		{"paid search is ppc", "Own paid search performance.", []string{"ppc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.text)
			for _, w := range tc.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, w)
				}
			}
		})
	}
}

// TestParse_SemIsNotAMarketingCanonical pins the deliberate omission: "sem" is a
// preposition in the Portuguese and Spanish postings this catalogue carries in bulk.
func TestParse_SemIsNotAMarketingCanonical(t *testing.T) {
	for _, text := range []string{
		"Profissional sem experiência prévia é bem-vindo.",
		"Puesto sem horario fijo.",
	} {
		if got := Parse(text); len(got) > 0 {
			t.Errorf("Parse(%q) = %v, want no canonicals", text, got)
		}
	}
}

// TestParse_GTMIsGoToMarket pins the homonym the right way round. In a job posting
// "GTM" is overwhelmingly go-to-market — "GTM strategy", "GTM motion" — while
// Google Tag Manager is spelled out or named as a container. Reading the bare
// abbreviation as the tag manager would mislabel the marketing corpus.
func TestParse_GTMIsGoToMarket(t *testing.T) {
	cases := []struct {
		name         string
		text         string
		want, unwant string
	}{
		{"gtm strategy", "You will own our GTM strategy end to end.", "go-to-market", "google-tag-manager"},
		{"gtm motion", "Shape the GTM motion for a new segment.", "go-to-market", "google-tag-manager"},
		{"spelled out go to market", "Own the go-to-market plan.", "go-to-market", "google-tag-manager"},
		{"bare abbreviation among tools is not the tag manager", "Tooling: GTM, GA4.", "", "google-tag-manager"},
		{"spelled out product", "Deploy tags through Google Tag Manager.", "google-tag-manager", "go-to-market"},
		{"container form", "Maintain the GTM container and its triggers.", "google-tag-manager", "go-to-market"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.text)
			if tc.want != "" && !slices.Contains(got, tc.want) {
				t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, tc.want)
			}
			if slices.Contains(got, tc.unwant) {
				t.Errorf("Parse(%q) = %v, must not contain %q", tc.text, got, tc.unwant)
			}
		})
	}
}

// TestParse_DisciplinePhrasesDoNotCorroborate pins the matcher rule this change
// adds. A phrase match is normally strong, and a strong match rescues the gated
// single-word canonicals. A marketing discipline is a concept, not a concrete
// technology, so it must tag without rescuing them — otherwise the "AI-powered"
// prose that saturates marketing postings tags the whole population with `ai`.
// A named product keeps corroborating, which is what recovers `seo`.
func TestParse_DisciplinePhrasesDoNotCorroborate(t *testing.T) {
	t.Run("discipline does not pull in the gated concept", func(t *testing.T) {
		got := Parse("AI-powered marketing automation for our sales team.")
		if !slices.Contains(got, "marketing-automation") {
			t.Errorf("Parse(...) = %v, want marketing-automation", got)
		}
		for _, unwanted := range []string{"ai", "automation"} {
			if slices.Contains(got, unwanted) {
				t.Errorf("Parse(...) = %v, must not contain %q", got, unwanted)
			}
		}
	})
	t.Run("product still corroborates", func(t *testing.T) {
		got := Parse("SEO Specialist — keyword research in Ahrefs and Semrush.")
		for _, want := range []string{"ahrefs", "semrush", "seo"} {
			if !slices.Contains(got, want) {
				t.Errorf("Parse(...) = %v, want %q", got, want)
			}
		}
	})
	t.Run("discipline stands alone", func(t *testing.T) {
		got := Parse("Own content marketing for the brand.")
		if !slices.Contains(got, "content-marketing") {
			t.Errorf("Parse(...) = %v, want content-marketing", got)
		}
	})
}

// TestParse_SalesAndSupportDoNotCorroborate mirrors
// TestParse_DisciplinePhrasesDoNotCorroborate for the sales/support phrases: they tag
// on their own but must not rescue a gated word that merely appears beside them.
func TestParse_SalesAndSupportDoNotCorroborate(t *testing.T) {
	t.Run("sales phrase does not pull in the gated concept", func(t *testing.T) {
		got := Parse("Manage our CRM as an account executive.")
		if !slices.Contains(got, "account-executive") {
			t.Errorf("Parse(...) = %v, want account-executive", got)
		}
		if slices.Contains(got, "crm") {
			t.Errorf("Parse(...) = %v, must not contain %q", got, "crm")
		}
	})
	t.Run("support phrase does not pull in the gated concept", func(t *testing.T) {
		got := Parse("Ticket resolution specialist troubleshooting via shell scripts the eng team wrote.")
		if !slices.Contains(got, "ticket-resolution") {
			t.Errorf("Parse(...) = %v, want ticket-resolution", got)
		}
		if slices.Contains(got, "shell") {
			t.Errorf("Parse(...) = %v, must not contain %q", got, "shell")
		}
	})
	t.Run("sales and support phrases stand alone", func(t *testing.T) {
		got := Parse("Business development lead driving cold outreach and pipeline management.")
		for _, want := range []string{"business-development", "cold-outreach", "pipeline-management"} {
			if !slices.Contains(got, want) {
				t.Errorf("Parse(...) = %v, want %q", got, want)
			}
		}
	})
	t.Run("lead generation does not pull in the gated concept", func(t *testing.T) {
		got := Parse("Lead generation using our CRM and analytics tools.")
		if !slices.Contains(got, "lead-generation") {
			t.Errorf("Parse(...) = %v, want lead-generation", got)
		}
		for _, unwanted := range []string{"crm", "analytics"} {
			if slices.Contains(got, unwanted) {
				t.Errorf("Parse(...) = %v, must not contain %q", got, unwanted)
			}
		}
	})
	t.Run("sales enablement does not pull in the gated concept", func(t *testing.T) {
		got := Parse("Sales enablement lead supporting reps with our CRM and analytics stack.")
		if !slices.Contains(got, "sales-enablement") {
			t.Errorf("Parse(...) = %v, want sales-enablement", got)
		}
		for _, unwanted := range []string{"crm", "analytics"} {
			if slices.Contains(got, unwanted) {
				t.Errorf("Parse(...) = %v, must not contain %q", got, unwanted)
			}
		}
	})
}

// TestParse_MarketingSeparatorInsensitive checks that the separator rule the
// matcher already guarantees holds for the new multi-word canonicals too — the
// dictionary lists only the space form, so the hyphen and underscore spellings
// depend on that normalization.
func TestParse_MarketingSeparatorInsensitive(t *testing.T) {
	for _, form := range []string{"paid social", "paid-social", "paid_social"} {
		got := Parse("We run " + form + " campaigns.")
		if !slices.Contains(got, "paid-social") {
			t.Errorf("Parse(%q) = %v, want paid-social", form, got)
		}
	}
	for _, form := range []string{"demand generation", "demand-generation", "demand_generation"} {
		got := Parse("Own " + form + " for EMEA.")
		if !slices.Contains(got, "demand-generation") {
			t.Errorf("Parse(%q) = %v, want demand-generation", form, got)
		}
	}
}

// TestParse_MinedBatch4 witnesses the LLM-mined batch 4 vocabulary: the industrial,
// electronics, enterprise-SaaS and compliance terms the enrichment signal named most
// and the dictionary did not resolve. Each group probes the routing decision that was
// made for it, and every rejected token is probed in the false context it was rejected
// for — a negative case here is what stops a later batch from re-adding it.
func TestParse_MinedBatch4(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		// Strong aliases: a coined product name or acronym tags on its own.
		{"jvm server stack", "Java shop running Spring Boot on JBoss, JDBC and JMS.",
			[]string{"java", "spring", "jboss", "jdbc", "jms"}, nil},
		{"apple toolchain", "iOS engineer: UIKit in Xcode, no SwiftUI legacy.",
			[]string{"uikit", "xcode"}, nil},
		{"security vendors", "SOC analyst: CrowdStrike, Fortinet and CyberArk; CISSP preferred.",
			[]string{"crowdstrike", "fortinet", "cyberark", "cissp"}, nil},
		{"regulatory frameworks", "Privacy engineer covering GDPR, CCPA and LGPD; HIPAA a plus.",
			[]string{"gdpr", "ccpa", "lgpd", "hipaa"}, nil},
		{"industrial automation", "Controls engineer: Modbus and I2C links into the HMI.",
			[]string{"modbus", "i2c", "hmi"}, nil},
		{"silicon verification", "ASIC verification in UVM, RTL design on a CMOS process.",
			[]string{"asic", "uvm", "rtl", "cmos"}, nil},
		{"aec software", "Estimator using Bluebeam, Navisworks and Procore.",
			[]string{"bluebeam", "navisworks", "procore"}, nil},

		// Gated: the product token is also ordinary English, so it needs a concrete
		// technology beside it. Each is probed in the false context it collides with.
		{"bootstrap a startup", "You will bootstrap the retail team from scratch.", nil, []string{"bootstrap"}},
		{"bootstrap the framework", "Frontend work in JavaScript with Bootstrap and SCSS.",
			[]string{"bootstrap", "scss", "javascript"}, nil},
		{"hibernate the laptop", "Set the store laptop to hibernate overnight.", nil, []string{"hibernate"}},
		{"hibernate the orm", "Java backend on Hibernate over PostgreSQL.",
			[]string{"hibernate", "java", "postgresql"}, nil},
		{"unity the value", "We value unity, teamwork and a positive attitude.", nil, []string{"unity"}},
		{"unity the engine", "Game developer: Unity and C# for mobile titles.",
			[]string{"unity", "csharp"}, nil},
		{"slack the noun", "Warehouse lead. Pick up the slack during peak season.", nil, []string{"slack"}},
		{"workplace tools corroborated", "Platform engineer on Kubernetes; we run on Slack, Notion and Zoom.",
			[]string{"slack", "notion", "zoom", "kubernetes"}, nil},
		{"asana the pose", "Yoga instructor: sequence each asana for mixed-ability classes.", nil, []string{"asana"}},
		{"bicep the muscle", "Personal trainer: bicep and triceps programming for members.", nil, []string{"bicep"}},
		{"bicep the azure dsl", "Azure infrastructure as code in Bicep and Terraform.",
			[]string{"bicep", "azure", "terraform"}, nil},
		{"puppet the show", "Children's entertainer running puppet shows at weekends.", nil, []string{"puppet"}},
		{"puppet the config tool", "Linux SRE: Puppet and Ansible across the fleet.",
			[]string{"puppet", "ansible", "linux"}, nil},
		{"parquet the flooring", "Fitter laying parquet flooring in period properties.", nil, []string{"parquet"}},
		{"parquet the format", "Data engineer: Parquet on S3 read through Spark.",
			[]string{"parquet", "spark"}, nil},
		{"aws services as names", "Report to Athena and Aurora, our duty managers.", nil, []string{"athena", "aurora"}},
		{"aws services corroborated", "AWS data stack: Athena over AWS Glue, Aurora for OLTP, Terraform managed.",
			[]string{"athena", "aurora", "aws-glue", "terraform"}, nil},
		{"flux the welding consumable", "Welder: flux core wire, MIG and TIG on structural steel.", nil, []string{"flux"}},
		{"prefect the school role", "Pastoral lead overseeing the prefect system and house points.", nil, []string{"prefect"}},

		// Phrase-only: the bare token is a place, a person, another language's everyday
		// word, or a different acronym entirely, so only the qualified form resolves.
		{"plc the company suffix", "Reporting into Barclays PLC group finance.", nil, []string{"plc"}},
		{"plc the controller", "Maintenance technician doing PLC programming and ladder logic.",
			[]string{"plc"}, nil},
		{"palo alto the city", "Hybrid role based in Palo Alto, three days on site.",
			nil, []string{"palo-alto-networks"}},
		{"palo alto the vendor", "Network security on Palo Alto Networks firewalls.",
			[]string{"palo-alto-networks"}, nil},
		{"primavera the season", "Campanha de primavera para a loja de moda.", nil, []string{"primavera-p6"}},
		{"primavera the scheduler", "Planner running Primavera P6 on a rail programme.",
			[]string{"primavera-p6"}, nil},
		{"maximo the superlative", "Aprovecha el maximo rendimiento del equipo comercial.", nil, []string{"maximo"}},
		{"maximo the cmms", "Reliability engineer administering IBM Maximo work orders.",
			[]string{"maximo"}, nil},
		{"pcb the pollutant", "Environmental consultant: PCB and asbestos remediation surveys.",
			nil, []string{"pcb"}},
		{"pcb the board", "Hardware engineer owning PCB design and bring-up.", []string{"pcb"}, nil},
		{"claude the given name", "You will report to Claude, our regional director.", nil, []string{"claude"}},
		{"claude the model", "AI engineer building on the Claude API and Hugging Face models.",
			[]string{"claude", "huggingface"}, nil},
		{"claude code is its own canonical", "We use Claude Code alongside Kubernetes.",
			[]string{"claude-code", "kubernetes"}, []string{"claude"}},
		{"s4hana routes to the suite", "SAP S4HANA migration work in Python.",
			[]string{"sap-s4hana", "python"}, []string{"sap-hana"}},
		{"copilot the aviator", "First officer and copilot duties on short-haul routes.",
			nil, []string{"github-copilot", "microsoft-copilot"}},
		{"copilot the assistant", "Engineers here use GitHub Copilot daily.", []string{"github-copilot"}, nil},
		{"workday the ordinary noun", "Enjoy a flexible workday and an early Friday finish.",
			nil, []string{"workday"}},
		{"workday the hcm suite", "HR systems analyst supporting Workday HCM and ADP Payroll.",
			[]string{"workday", "adp"}, nil},
		{"concur the verb", "We concur that customer focus comes first.", nil, []string{"concur"}},
		{"concur the expense tool", "Finance assistant processing claims in SAP Concur.",
			[]string{"concur"}, nil},

		// Shop-floor craft tags itself but must not vouch for the gated design words
		// beside it — the failure that made `cnc` a phrase rather than a word alias.
		{"cnc tags without corroborating", "CNC operator: read wireframes and 2D drawings before fabrication.",
			[]string{"cnc"}, []string{"wireframing"}},
		{"soldering tags without corroborating", "Assembler: soldering and oscilloscope checks; sketch out the rework.",
			[]string{"soldering", "oscilloscope"}, []string{"sketch"}},

		// Withheld entirely — each was mined above the floor and rejected. A tag here
		// would be the regression these cases exist to catch.
		{"informatica is portuguese for it", "Tecnico de informatica para suporte em loja.", nil, []string{"informatica"}},
		{"soar is sales prose", "Watch your commission soar in our high-growth team.", nil, []string{"soar"}},
		{"node is anatomy", "Oncology nurse: lymph node assessment and patient education.", nil, []string{"nodejs"}},
		{"mfa is a fine arts degree", "Studio lead, MFA in graphic design preferred.", nil, []string{"mfa"}},
		{"kms is kilometres", "Delivery driver covering 300 kms per shift.", nil, []string{"kms"}},
		{"excel stays untagged", "Cashier: strong Excel and Microsoft Office skills.",
			nil, []string{"excel", "microsoft-office"}},

		// Witnesses the first cut of this batch shipped without. Each gated term needs
		// both directions or the gate is untested in the one that matters.
		{"eclipse the astronomy club", "Observatory guide: run the solar eclipse viewing evenings.",
			nil, []string{"eclipse"}},
		{"eclipse the ide", "Java developer working in Eclipse against a Maven build.",
			[]string{"eclipse", "java"}, nil},
		{"loki the given name", "Kennel assistant: Loki and Freya need walking twice daily.",
			nil, []string{"loki"}},
		{"loki the log store", "SRE stack: Grafana, Loki and Prometheus on Kubernetes.",
			[]string{"loki", "grafana", "kubernetes"}, nil},
		{"zoom the verb", "Photographer: zoom and focus by hand on a manual lens.", nil, []string{"zoom"}},
		{"notion the noun", "Challenge the notion that retail cannot be premium.", nil, []string{"notion"}},
		{"asana the tool", "Program manager tracking delivery in Asana beside our Python services.",
			[]string{"asana", "python"}, nil},
		{"prefect the orchestrator", "Data platform: Prefect flows loading into Snowflake.",
			[]string{"prefect", "snowflake"}, nil},
		{"flux the gitops tool", "Kubernetes platform with Flux for GitOps and Terraform for infra.",
			[]string{"flux", "kubernetes", "terraform"}, nil},

		// Compliance frameworks tag the posting they describe, and vouch for nobody.
		// As strong word aliases they lifted the gate and tagged `slack` on a nurse.
		{"hipaa does not corroborate", "Registered nurse. HIPAA training required. Pick up the slack on busy shifts.",
			[]string{"hipaa"}, []string{"slack"}},
		{"gdpr does not corroborate", "Care coordinator. GDPR duties, a flexible workday, and we value unity.",
			[]string{"gdpr"}, []string{"unity", "workday"}},
		{"iso 9001 does not corroborate", "Injection moulding supervisor under ISO 9001; sketch out the shift rota.",
			[]string{"iso-9001"}, []string{"sketch"}},
		{"security certifications still tag", "Security engineer: NIST 800-53, FedRAMP and CMMC; CISSP required.",
			[]string{"nist", "fedramp", "cmmc", "cissp"}, nil},
		{"hipaa alone is not engineering", "Medical records clerk handling HIPAA requests.",
			[]string{"hipaa"}, nil},

		// Acronyms that belong to another industry unless qualified.
		{"asic the australian regulator", "Compliance officer: ASIC and AFSL reporting for our Sydney licence.",
			nil, []string{"asic"}},
		{"rtl the text direction", "Frontend i18n: RTL layouts for Arabic and Hebrew locales.",
			nil, []string{"rtl"}},
		{"cfd the trading product", "Trading desk: FX, CFD and equity derivatives for retail clients.",
			nil, []string{"cfd"}},
		{"cfd the simulation", "Thermal engineer running CFD simulation against MATLAB models.",
			[]string{"cfd"}, nil},
		{"kvm the closet switch", "IT support: rack the servers and use the KVM switch in the closet.",
			nil, []string{"kvm"}},
		{"kvm the hypervisor", "Linux virtualisation on the KVM hypervisor with Ansible.",
			[]string{"kvm", "linux", "ansible"}, nil},

		// Regressions for the two canonicals that were splitting a facet in two.
		{"bedrock stays one canonical", "Deploy on AWS Bedrock behind Terraform.",
			[]string{"aws-bedrock"}, []string{"bedrock"}},
		{"ros stays one canonical", "Robotics engineer: ROS2 stack in C++.",
			[]string{"ros2"}, []string{"ros"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if slices.Contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

// TestParse_CreativeToolchain covers the vocabulary the media crafts name. The
// omissions are the point as much as the entries: "animation" and "nuke" are
// deliberately absent, because the corroboration gate is lifted by ANY strong skill
// and a frontend posting always has one — gating them would tag every React posting
// that mentions a CSS animation.
func TestParse_CreativeToolchain(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"video suite", "Video editor: DaVinci Resolve and Final Cut Pro, colour grading to spec.",
			[]string{"davinci-resolve", "final-cut-pro", "color-grading"}, nil},
		{"american spelling of the grade", "Post-production lead handling color grading and delivery.",
			[]string{"color-grading"}, nil},
		{"short-form video", "UGC editor cutting vertical video in CapCut and Premiere Pro.",
			[]string{"capcut", "premiere-pro"}, nil},
		{"3d suite", "3D artist: modelling in Cinema 4D, sculpting in ZBrush, texturing in Substance Painter.",
			[]string{"cinema-4d", "zbrush", "substance-painter"}, nil},
		{"the two substance products are two skills", "Material artist: Substance Designer for procedural materials, ZBrush for sculpts.",
			[]string{"substance-designer", "zbrush"}, []string{"substance-painter"}},

		// c4d is gated in the word pass. As a phrase it was ungated, and a phrase match
		// is always strong — it tagged a pathology posting AND lifted the gate beside it.
		{"c4d the biomarker", "Transplant pathologist: interpret C4d staining and biopsy grading.",
			nil, []string{"cinema-4d"}},
		{"c4d the 3d suite", "Motion artist working in C4d and ZBrush.",
			[]string{"cinema-4d", "zbrush"}, nil},

		// The three craft names tag on their own but must never vouch for a gated word:
		// each is a duty listed in passing on postings from other disciplines.
		{"video editing does not corroborate", "Marketing intern, Spring 2026 cohort. Duties: video editing and scheduling.",
			[]string{"video-editing"}, []string{"spring"}},
		{"video editing does not corroborate unity", "Social media coordinator: video editing, and Unity within the team matters.",
			[]string{"video-editing"}, []string{"unity"}},
		{"storyboarding does not corroborate sketch", "Product manager: run workshops, build storyboards, sketch out flows.",
			[]string{"storyboarding"}, []string{"sketch"}},
		{"colour grading does not corroborate", "Events team handling colour grading of the pitch deck and the venue booking.",
			[]string{"color-grading"}, nil},
		{"game engines", "Gameplay programmer: Godot and Unreal Engine, C++ throughout.",
			[]string{"godot", "unreal-engine", "cpp"}, nil},
		{"the craft itself", "Motion designer doing storyboarding and video editing for launch films.",
			[]string{"storyboarding", "video-editing"}, nil},

		// Gated: the token is a name or ordinary word outside this craft.
		{"houdini the magician", "Events host: a Houdini-style escape act for corporate parties.",
			nil, []string{"houdini"}},
		// Both "houdini" and "maya" are gated, so the corroboration has to come from a
		// strong token — here the render pipeline's own Substance Painter.
		{"houdini the vfx suite", "FX artist: Houdini and Maya, texturing in Substance Painter.",
			[]string{"houdini", "maya", "substance-painter"}, nil},

		// Deliberate omissions.
		{"animation is never a skill", "Frontend engineer: React, CSS animation and Tailwind.",
			[]string{"react", "tailwind"}, []string{"animation"}},
		{"nuke is never a skill", "Platform engineer: nuke the cache and redeploy via Terraform.",
			[]string{"terraform"}, []string{"nuke"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if slices.Contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}
