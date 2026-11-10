package skilltag

import (
	"reflect"
	"testing"
)

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
