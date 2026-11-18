package skilltag

import (
	"slices"
	"testing"
)

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
