package skilltag

import (
	"slices"
	"testing"
)

// TestParse_MinedBatch5 covers the 2026-10 enrichment-mining batch. The absent
// cases matter as much as the present ones: each locks in a routing decision — a
// gated word that must not tag alone, a discipline phrase that must not vouch, or a
// term deliberately left out because its bare form has another reading.
func TestParse_MinedBatch5(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		// Strong aliases: coined names tag on their own.
		{"python data stack", "Backend in FastAPI with Pydantic, SQLAlchemy and asyncio; Polars for analytics.",
			[]string{"pydantic", "sqlalchemy", "asyncio", "polars"}, nil},
		{"dotnet stack", "C# developer: Blazor front end, WCF services, LINQ, tests in xUnit and NUnit.",
			[]string{"blazor", "wcf", "linq", "xunit", "nunit"}, nil},
		{"aws services", "Deploy to Fargate behind CloudFront, audit with CloudTrail, DNS in Route53.",
			[]string{"fargate", "cloudfront", "cloudtrail", "route53"}, nil},
		{"infra tooling", "Terragrunt over OpenTofu, Kustomize overlays, builds in Bazel.",
			[]string{"terragrunt", "opentofu", "kustomize", "bazel"}, nil},
		{"embedded", "Embedded Linux on Yocto, FreeRTOS on the MCU, AUTOSAR stack, QNX in the head unit.",
			[]string{"yocto", "freertos", "autosar", "qnx"}, nil},
		{"networking", "Data-centre fabric: EVPN over VXLAN, EIGRP at the edge, IPv6 dual stack, RDMA over InfiniBand.",
			[]string{"evpn", "eigrp", "ipv6", "rdma", "infiniband"}, nil},
		{"security vendors", "Run Snyk, Qualys and Zscaler; identity governance in SailPoint.",
			[]string{"snyk", "qualys", "zscaler", "sailpoint"}, nil},
		{"aliases onto existing canonicals", "Admin VMware vSphere and ESXi on RedHat; HDFS and a DWH; ES6 with TailwindCSS.",
			[]string{"vmware", "rhel", "hadoop", "data-warehousing", "javascript", "tailwind"}, nil},
		{"sre", "SRE team owning on-call and ITSM processes.", []string{"sre", "itsm"}, nil},

		// Phrases.
		{"architecture patterns", "We follow domain-driven design, clean architecture and CQRS.",
			[]string{"domain-driven-design", "clean-architecture", "cqrs"}, nil},
		{"azure services", "Azure Monitor and Application Insights; messaging on Azure Service Bus; Cosmos DB.",
			[]string{"azure-monitor", "azure-application-insights", "azure-service-bus", "cosmos-db"}, nil},
		{"security practice", "Lead threat modeling, vulnerability management and threat intelligence.",
			[]string{"threat-modeling", "vulnerability-management", "threat-intelligence"}, nil},
		{"databricks governance", "Lakehouse on Databricks with Unity Catalog and a medallion architecture.",
			[]string{"unity-catalog", "medallion-architecture"}, nil},

		// Gated words: alone on a non-technical posting they must not tag.
		{"stripe as a stripe", "Line painter: paint a white stripe along every parking bay.",
			nil, []string{"stripe"}},
		{"stripe with a stack", "Payments engineer integrating Stripe from a Node.js service.",
			[]string{"stripe"}, nil},
		{"retail gadgets", "Sales associate: help customers pick laptops with a fast GPU and Bluetooth headsets.",
			nil, []string{"gpu", "bluetooth"}},
		{"gpu with a stack", "Train models on GPU clusters with PyTorch and CUDA.",
			[]string{"gpu"}, nil},
		{"hr generalist", "HR generalist maintaining employee records in our HRIS.",
			nil, []string{"hris"}},
		{"facilities planner", "Maintenance planner scheduling work orders in the CMMS; react to urgent requests.",
			[]string{"cmms"}, []string{"react"}},
		{"perforce as an adverb", "The team must perforce travel to the client site weekly.",
			nil, []string{"perforce"}},

		// Discipline and office-tool phrases tag, but never vouch for a gated word.
		{"fmea does not vouch", "Quality planner: run FMEA reviews and react quickly to line stoppages.",
			[]string{"fmea"}, []string{"react"}},
		{"ms project does not vouch", "Project coordinator using MS Project; sketch out the timelines.",
			[]string{"microsoft-project"}, []string{"sketch"}},
		{"business intelligence does not vouch", "Business intelligence lead; react to stakeholder asks.",
			[]string{"business-intelligence"}, []string{"react"}},

		// Left out on purpose: each bare form has another reading.
		{"italian flowers", "Negozio di fiori a Milano cerca commessa.", nil, []string{"sap-fiori"}},
		{"polish stage", "Etap rekrutacji: rozmowa z zespołem.", nil, []string{"etap"}},
		{"insurance group policy", "Benefits administrator for the group policy renewals.", nil, []string{"group-policy"}},
		{"grams of sugar", "Each serving has 5g of sugar.", nil, []string{"5g"}},
		{"bare step functions", "Tutor algebra: linear and step functions.", nil, []string{"step-functions"}},
		{"bare hyperion", "Join the Hyperion hotel group front desk.", nil, []string{"oracle-hyperion"}},
		{"jee exam", "Physics teacher preparing students for the JEE exam.", nil, []string{"j2ee"}},
		{"canadian dollars", "Salary CDN$ 90,000.", nil, []string{"cdn"}},
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
