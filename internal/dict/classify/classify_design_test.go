package classify

import (
	"testing"
)

// TestParse_ITAdjacentCoverage covers professions found adjacent to the core IT
// disciplines during a coverage audit: content/localization crafts that were
// falling to the bare "designer" entry or resolving to nothing, and back-office
// functions (recruiting, legal, operations, customer_success, marketing,
// security) that were falling through to the generic manager/analyst
// catch-alls or staying unresolved. Deliberately excluded titles that stay
// unresolved are pinned in the precision block: they span too many non-IT
// industries (Editor, Proofreader, Controller) or are genuinely overloaded
// abbreviations (AR/CRO/AP) for a bare or narrow alias to be safe.
func TestParse_ITAdjacentCoverage(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// technical_writing — content/localization crafts, guard: beat bare designer
		{"Instructional Designer", "technical_writing"},
		{"Senior Instructional Designer", "technical_writing"},
		{"Learning Designer", "technical_writing"},
		{"Curriculum Designer", "technical_writing"},
		{"Learning Experience Designer", "technical_writing"},
		{"E-Learning Developer", "technical_writing"},
		{"Translator", "technical_writing"},
		{"French Translator", "technical_writing"},
		{"Localization Engineer", "technical_writing"},
		{"Переводчик", "technical_writing"},

		// recruiting — guard: beat manager-> fall-through
		{"Sourcer", "recruiting"},
		{"Talent Partner", "recruiting"},
		{"Employer Branding Manager", "recruiting"},

		// legal — guard: beat manager-> fall-through
		{"Privacy Officer", "legal"},
		{"Data Privacy Officer", "legal"},
		{"Data Privacy Manager", "legal"},
		{"Contracts Administrator", "legal"},
		{"Contract Administrator", "legal"},

		// operations
		{"Facilities Coordinator", "operations"},

		// it_support — the unspaced compound "help desk" cannot reach
		{"Helpdesk Technician", "it_support"},

		// customer_success — guard: beat unresolved
		{"Implementation Engineer", "customer_success"},

		// marketing — guard: beat manager-> fall-through / bare event false positive
		{"Growth Hacker", "marketing"},
		{"Event Manager", "marketing"},
		{"Events Coordinator", "marketing"},
		{"App Store Optimization Specialist", "marketing"},
		{"ASO Specialist", "marketing"},
		{"Conversion Rate Optimization Specialist", "marketing"},

		// security — IT-anchored audit only
		{"IT Auditor", "security"},

		// precision — deliberately left unresolved, existing behavior unchanged
		{"Editor", ""},                             // spans every industry, not IT-anchored
		{"Proofreader", ""},                        // ditto
		{"Controller", "finance"},                  // the financial controller; the stock, document and air-traffic ones are pinned
		{"AR Specialist", ""},                      // AR also means Augmented Reality
		{"AP Specialist", ""},                      // too thin a signal on its own
		{"CRO Specialist", ""},                     // bare CRO also Chief Revenue Officer
		{"Internal Auditor", ""},                   // spans every industry's audit function
		{"Reliability Engineer", ""},               // ambiguous without "site" (mechanical reliability exists)
		{"Event-Driven Architecture Engineer", ""}, // guard: bare "event" must not fire
		{"Machine Translation Engineer", ""},       // guard: bare "translation" must not fire (this is an ML role)
		{"Translation Engineer", ""},               // ditto
		{"Developer Onboarding Engineer", ""},      // guard: "onboarding engineer" is dual-use (internal platform tooling too)
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_AliasGapFill covers title phrasing for IT sub-roles that already have a
// home category (project_management, security, devops, data_engineering,
// data_analytics) but whose common surface forms the dictionary never learned —
// found by sampling live prod titles. No new category values are involved.
func TestParse_AliasGapFill(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// agile/PM
		{"Agile Coach", "project_management"},
		{"Senior Agile Coach", "project_management"},
		{"Release Train Engineer", "project_management"},
		{"Agile Transformation Lead", "project_management"},
		{"Agile Transformation Manager", "project_management"}, // guard: manager-> fall-through
		{"SAFe Scrum Master", "project_management"},
		{"SAFe Practitioner", "project_management"},
		{"Scaled Agile Framework Coach", "project_management"},

		// security — narrower technical niches
		{"IAM Engineer", "security"},
		{"Identity and Access Management Analyst", "security"}, // guard: analyst-> fall-through
		{"GRC Analyst", "security"},
		{"Vulnerability Management Engineer", "security"},
		{"Vulnerability Analyst", "security"}, // guard: analyst-> fall-through
		{"Incident Response Engineer", "security"},
		{"Red Team Operator", "security"},
		{"Red Teamer", "security"},
		{"Blue Team Analyst", "security"},
		{"Penetration Tester", "security"},
		{"Penetration Testing Engineer", "security"},
		{"Pentester", "security"},
		{"Pentest Engineer", "security"},
		{"Threat Intelligence Analyst", "security"},
		{"Threat Intel Lead", "security"},
		{"CISO", "security"},
		{"Chief Information Security Officer", "security"},
		{"DevSecOps Engineer", "security"}, // guard: stays security, not devops

		// data/devops
		{"Data Platform Engineer", "data_engineering"},
		{"Data Governance Lead", "data_engineering"},
		{"Data Governance Manager", "data_engineering"}, // guard: manager-> fall-through
		{"Data Steward", "data_engineering"},
		{"MLOps Engineer", "devops"}, // guard: stays devops, not ml_ai/data_engineering
		{"ML Ops Engineer", "devops"},
		{"Analytics Engineer", "data_analytics"},
		{"Senior Analytics Engineer", "data_analytics"},
		{"Platform Engineering Team Leader", "devops"}, // gerund form, not just "platform engineer"

		// precision — word-traps deliberately excluded, existing behavior unchanged
		{"Safe Driving Instructor", ""},       // bare "safe" must not resolve
		{"Customs Compliance Specialist", ""}, // bare "compliance" must not resolve to security
		{"Platform Engineer", "devops"},       // pre-existing alias unaffected
		{"Data Analyst", "data_analytics"},    // plain analyst role unaffected
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_DesignSplit covers the split of the design craft: engineering
// draughting (mechanical, electrical, civil, chip) resolves to engineering_design,
// while `design` keeps meaning product, visual and experience design. The bare
// "Design Engineer" goes to the engineering side — that population is
// overwhelmingly mechanical in the catalogue — so a product hybrid has to state a
// marker of its own. The guards at the end pin the neighbouring aliases that the
// inserted block sits next to.
func TestParse_DesignSplit(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// engineering design — the qualified forms
		{"Mechanical Design Engineer", "engineering_design"},
		{"Senior Electrical Design Engineer", "engineering_design"},
		{"Civil Design Engineer", "engineering_design"},
		{"Structural Designer", "engineering_design"},
		{"Piping Designer", "engineering_design"},
		{"Plumbing Designer / Drafter", "engineering_design"},
		{"Process Design Engineer", "engineering_design"},
		{"Packaging Design Engineer", "engineering_design"},
		{"Electrical Designer", "engineering_design"},
		{"Civil Designer", "engineering_design"},
		{"CAD Designer", "engineering_design"},
		{"Design Drafter", "engineering_design"},
		// The BIM / architectural-draughting family. It is the largest population still
		// left in `design` after the first pass — `revit` alone tags 2846 of those jobs —
		// and the bare "design engineer" alias cannot see any of these titles.
		{"Architectural Designer", "engineering_design"},
		{"BIM Designer", "engineering_design"},
		{"Revit Designer", "engineering_design"},
		{"Senior BIM Coordinator", "engineering_design"},
		{"BIM Specialist", "engineering_design"},
		{"Die Designer", "engineering_design"},
		{"Tool Designer", "engineering_design"},
		// Print and magazine layout is the product-design craft, so no bare
		// "layout designer" alias: the phrase names both trades.
		{"Magazine Layout Designer", "design"},
		{"Mold Designer", "engineering_design"},
		{"Draftsman", "engineering_design"},
		{"CAD Drafter", "engineering_design"},
		{"Design Technician", "engineering_design"},
		// Silicon and board design stay with `hardware`, which already owns the rest of
		// that team: "Hardware Design Engineer" and "FPGA Design Engineer" resolve there
		// through the earlier hardware aliases, so filing their colleagues under
		// draughting would split one discipline across two facets — and cost them the
		// technical treatment (enrichment, embeddings) they have today.
		{"PCB Design Engineer", "hardware"},
		{"PCB Layout Designer", "hardware"},
		{"Physical Design Engineer", "hardware"},
		{"Analog Design Engineer", "hardware"},
		{"RTL Design Engineer", "hardware"},
		{"VLSI Design Engineer", "hardware"},
		{"Senior VLSI Design Lead", "hardware"},
		{"Hardware Design Engineer", "hardware"},
		{"FPGA Design Engineer", "hardware"},
		// the bare title resolves to the engineering side
		{"Design Engineer", "engineering_design"},
		{"Senior Design Engineer", "engineering_design"},
		// product hybrids keep `design`, but only with an explicit marker
		{"Product Design Engineer", "design"},
		{"Design Systems Engineer", "design"},
		{"UI Engineer", "design"},
		{"UX Engineer", "design"},
		// The interface-design hybrids: the marker must be read BEFORE the bare
		// "design engineer" below, which would otherwise file them as draughting.
		{"UX Design Engineer", "design"},
		{"UI Design Engineer", "design"},
		{"UI/UX Design Engineer", "design"},
		{"Web Design Engineer", "design"},
		{"Design Engineer, Product", "design"},
		// product / visual design is untouched
		{"Senior Product Designer", "design"},
		{"UX Designer", "design"},
		{"UI/UX Designer", "design"},
		{"Visual Designer", "design"},
		{"Graphic Designer", "design"},
		{"Дизайнер интерфейсов", "design"},
		// Russian: "конструктор" is the draughting profession, not a UI designer, and
		// the hyphen is a word boundary so the compound form resolves through it.
		{"Инженер-конструктор", "engineering_design"},
		{"Конструктор металлоконструкций", "engineering_design"},

		// Software-anchored forms: "design" here qualifies the engineering, it is not
		// the craft. They must not be filed as draughting — that would take a software
		// job out of the technical catalogue entirely — and they must not fall through
		// to the business aliases further down the table either.
		{"Software Design Engineer", ""},
		{"Senior Software Design Engineer", ""},
		{"Software Design Engineer - Sales Tools", ""},
		{"Software Design Engineer, Support Platform", ""},
		{"Software Design Engineering Manager", ""},
		// SDET spelled out has a category of its own.
		{"Software Design Engineer in Test", "qa"},
		// "Systems Design Engineer" is NOT masked: a qualifier makes it draughting, and
		// blanking the category would strip the placement that vetoes deletion.
		{"HVAC Systems Design Engineer", "engineering_design"},
		{"Mechanical Systems Design Engineer", "engineering_design"},
		{"Systems Design Engineer", "engineering_design"},
		// Where a better category exists, say so rather than emitting nothing.
		{"Cloud Design Engineer", "devops"},
		{"Solution Design Engineer", "solutions_engineering"},
		{"Solutions Design Engineer", "solutions_engineering"},
		// These name design disciplines of their own and stay on the product side.
		{"Service Design Engineer", "design"},
		{"Experience Design Engineer", "design"},
		{"Game Design Engineer", "design"},
		// "Sound Design Engineer" left this set with the rest of audio when the
		// `creative` category arrived — see TestParse_CreativeMedia.

		// The rest of the silicon family rides with `hardware` too — the first pass
		// covered only six phrases and left these as draughting.
		{"ASIC Design Engineer", "hardware"},
		{"SoC Design Engineer", "hardware"},
		{"IC Design Engineer", "hardware"},
		{"Digital Design Engineer", "hardware"},
		{"Mixed Signal Design Engineer", "hardware"},
		{"DFT Design Engineer", "hardware"},
		{"Semiconductor Design Engineer", "hardware"},
		// The hyphenated spelling is the industry's own, and a hyphen is a word
		// boundary — so it needs its own alias, like "middle-east" and "ai-product".
		{"Mixed-Signal Design Engineer", "hardware"},
		{"Analog/Mixed-Signal Design Engineer", "hardware"},
		{"RF Design Engineer", "hardware"},
		{"RFIC Design Engineer", "hardware"},
		{"Analogue Design Engineer", "hardware"},
		{"Silicon Design Engineer", "hardware"},
		{"Memory Design Engineer", "hardware"},

		// "Digital design" names three crafts. The chip side is stated by a second
		// word — ASIC, verification, an engineering grade — and goes to `hardware`;
		// a building discipline goes to draughting; the bare phrase stays design.
		// Real open titles, 2026-10-25.
		{"ASIC Digital Design, Sr Staff Engineer - 18568", "hardware"},
		{"ASIC Digital Design, Architect - 18388", "hardware"}, // ahead of bare "architect"
		{"Digital Design Architect", "hardware"},
		{"Digital Design Verification Engineer", "hardware"},
		{"Staff Engineer, Digital Design Engineering", "hardware"},
		{"Principal Engineer - Digital Design", "hardware"},
		{"IC Digital Design Intern", "hardware"},
		{"Senior ASIC Research Engineer – Front-end Digital Design", "hardware"}, // not web frontend
		{"Digital Design Specialist (Electrical)", "engineering_design"},
		{"Electrical Digital Design Lead (Buildings)", "engineering_design"},
		{"Structural BIM-Digital Design Team Lead", "engineering_design"},
		{"Digital Design Manager", "design"},
		{"Graphic/Digital Design Intern", "design"},
		{"Werkstudent (m/w/d) Digital Design / UI-Design", "design"},

		// precision — the neighbours of the inserted block must not shift
		{"Hardware Design Engineer", "hardware"},        // hardware precedes the design block
		{"Content Designer", "technical_writing"},       // technical_writing still wins
		{"UX Writer", "technical_writing"},              // ditto
		{"Senior Firmware Design Engineer", "embedded"}, // embedded precedes the design block
		{"Network Design Engineer", "network_engineering"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}
