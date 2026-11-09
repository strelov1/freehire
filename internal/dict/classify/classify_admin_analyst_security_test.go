package classify

import (
	"testing"
)

// TestParse_AdminVAAndImmigration covers the nontech-derive-coverage wave: the
// abbreviated/coordinator/specialist spellings of administrative support work, the
// virtual-assistant and front-desk titles, and the immigration-practice family. See
// docs/superpowers/specs/2026-09-19-nontech-derive-coverage-design.md.
//
// The bare alias "assistant" is deliberately never added — in live titles the word
// states a GRADE ("Assistant Controller") or qualifies a non-administrative trade
// ("Maintenance Assistant") far more often than it names admin work. Only qualified
// phrases earn an entry, so the grade-word and trade tail stays unresolved rather than
// gaining a categoryNone sentinel it does not need.
func TestParse_AdminVAAndImmigration(t *testing.T) {
	cases := []struct {
		title        string
		wantCategory string
		why          string
	}{
		// Administration: abbreviated and coordinator/specialist spellings.
		{"Admin Assistant", "administration", ""},
		{"Senior Admin Assistant", "administration", "a grade prefix must not block the match"},
		{"Administrative Coordinator", "administration", ""},
		{"Administrative Specialist", "administration", ""},
		{"Front Desk Agent", "administration", ""},
		{"Front Desk Coordinator", "administration", ""},
		{"Virtual Assistant", "administration", ""},
		{"Executive Virtual Assistant", "administration", ""},
		{"Executive Assistant to the CEO", "administration", ""},
		{"Executive Assistant to General Manager", "administration", "the manager is who is assisted, not the role"},
		{"Personal Assistant to General Manager", "administration", ""},
		{"Remote Executive Assistant", "administration", ""},
		{"Personal Assistant", "administration", ""},
		{"Medical Virtual Assistant (MVA)", "administration", ""},
		{"Virtual Medical Assistant", "administration", "a clinic's VA, not the clinical seat"},
		{"Real Estate Virtual Assistant", "administration", ""},
		{"Online Assistant", "administration", ""},
		{"Asistente Virtual", "administration", ""},
		{"Assistente Administrativo", "administration", ""},
		{"Assistante de direction (H/F)", "administration", ""},
		{"Ассистент руководителя", "administration", ""},
		{"Личный помощник руководителя", "administration", ""},
		// The "VA" abbreviation is never an alias: in this catalogue it is a US state,
		// Veterans Affairs, or the Swedish water-and-sewage trade.
		{"Business Analyst (Hybrid, local to DC/MD/VA)", "business_analysis", ""},
		{"Erfaren VA-projektör", "", "Swedish water and sewage, not a virtual assistant"},
		{"Customer Service Advisor", "support", ""},
		{"Nissan Service Advisor", "support", "a dealership's customer desk"},
		// The supermarket checkout lane, not a frontend developer.
		{"Front End Cashier", "retail", ""},
		{"Front End Clerk (Cashier or Courtesy Clerk)", "retail", ""},
		{"Heavy Equipment/ Large Front End Loader Operator", "skilled_trades", ""},
		{"Senior Front End Developer", "frontend", ""},

		// The bare-"assistant" exclusion: grade words and non-administrative trades
		// must stay unresolved, exactly as they are today.
		{"Assistant Controller", "finance", "grade word, not administration: the controller files it"},
		{"Assistant Superintendent", "skilled_trades", "grade word, not administration: the superintendent files it"},
		{"Assistant Director", "", "grade word, not a category"},
		{"Maintenance Assistant", "", "a trade, not admin work"},
		{"Clinic Assistant", "", "a trade, not admin work"},
		{"Laboratory Assistant", "science", "the laboratory, not admin work"},

		// Legal: the immigration-practice family.
		{"Immigration Paralegal", "legal", ""},
		{"Immigration Specialist", "legal", ""},
		{"Immigration Assistant", "legal", "must not fall through to administration"},
		{"Immigration Consultant", "legal", ""},
		{"Immigration Case Manager", "legal", "must not be stolen by the terminal manager fall-through"},
	}

	for _, tc := range cases {
		if got := Parse(tc.title).Category; got != tc.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q %s", tc.title, got, tc.wantCategory, tc.why)
		}
	}
}

// TestParse_AnalystIsQualifiedOrNothing pins the replacement of the bare "analyst"
// fall-through with the qualified forms the catalogue actually carries.
//
// The bare alias sat beside bare "manager" and was written as the same idea — a title with
// no recognised function falls through — but the two are not symmetric in what they cost.
// "manager" falls through to `management`, which is non-technical; "analyst" fell through
// to `data_analytics`, which is technical, so an unrecognised analyst was declared a data
// role. Measured on the open catalogue on 2026-09-06 that facet held 102,222 postings and
// 77,086 of them (75%) carried no data, analytics or BI word at all: 13,748 were Board
// Certified Behavior Analysts (school ABA therapists), the rest investment, purchasing,
// risk, pricing, research and policy analysts. All 102,222 had is_tech = true, so every one
// was eligible for the enrichment budget.
//
// The qualified entries below are the technical families the bare alias was hiding, with
// their live counts. They resolve MORE precisely than it did, not less.
func TestParse_AnalystIsQualifiedOrNothing(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// The technical families, each replacing what the bare alias used to claim.
		{"Test Analyst", "qa"}, // 524 live
		{"Senior Cyber Threat Analyst - Assessment", "security"},           // 263 live
		{"Cyber Analyst", "security"},                                      // 80 live
		{"Network Analyst", "network_engineering"},                         // 256 live
		{"Technical Analyst", "software_engineering"},                      // 605 live
		{"Applications Analyst - Epic Ambulatory", "software_engineering"}, // 648 live
		{"Application Analyst", "software_engineering"},
		{"IT Analyst", "software_engineering"},         // 478 live
		{"Technology Analyst", "software_engineering"}, // 344 live
		{"Software Analyst", "software_engineering"},   // 70 live
		{"Infrastructure Analyst", "devops"},           // 82 live
		{"Cloud Analyst", "devops"},                    // 18 live

		// Already resolved, and must stay resolved: the qualifier does the work.
		{"Senior Data Analyst (Martech)", "data_analytics"},
		{"BI Analyst", "data_analytics"},
		{"Analytics Engineer, Supply Chain", "data_analytics"},
		{"Аналитик данных", "data_analytics"},
		{"Системный аналитик", "business_analysis"},
		{"Финансовый аналитик", "finance"},
		{"SOC Analyst L2", "security"},
		{"Vulnerability Analyst", "security"},

		// "credit analyst" CONTAINS "it analyst" as a substring. The word-boundary match
		// is what stops 982 live credit analysts being called IT staff, so it is pinned.
		{"Credit Analyst I", ""},

		// The bare noun resolves to nothing in every sense the catalogue carries.
		{"Analyst", ""},
		{"Board Certified Behavior Analyst (BCBA)", "healthcare"}, // the clinical ABA seat, not an analyst
		{"Purchasing Analyst", ""},
		{"Investment Analyst", ""},
		{"Global Tax Analyst", ""},
		{"Senior Policy Analyst (DES)", ""},
		{"Risk Analyst", ""},
		{"Parts Inventory Analyst", ""},
		{"Catastrophe Modeling Analyst", ""},
		{"Sourcing Analyst", ""},
		{"Аналитик", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_SecurityIsQualifiedOrNothing pins the replacement of the bare "security"
// alias with the qualified forms the catalogue actually carries — the same call the bare
// "analyst" fall-through got above, for the same reason and with a sharper cost.
//
// The bare alias was not careless: it existed so that "Chief Information Security
// Officer" would claim the security facet, because the obvious alternative — a
// "security officer" → personal_services entry — tagged the CISO as a guard on the
// multi-category CV path, where every matching alias is returned and declaration order
// decides nothing. See TestCategories_ServiceOverlaps.
//
// What it cost was invisible from inside this package. `security` is in
// vocab.TechCategories, and jobderive treats a technical CATEGORY as enough to set
// is_tech TRUE on its own — ahead of the non-tech dictionary, deliberately, so that
// "Backend Engineer — Teller Systems" is not refused on its accidental "teller" match. So the bare alias did not merely mis-file a mall guard's facet: it asserted the
// guard was a technical posting, and no later evidence could take that back. Measured
// over the 150 commonest live titles carrying the word (10 276 open postings), it
// declared 10 156 of them technical; with the qualified forms, 5 917.
//
// The rule is: qualified or nothing. A title that names the discipline keeps the facet;
// an unqualified one claims neither facet and lets is_tech stay unknown.
func TestParse_SecurityIsQualifiedOrNothing(t *testing.T) {
	for _, tc := range []struct {
		title string
		want  string
		why   string
	}{
		// Qualified: the discipline is named.
		{"Security Engineer", "security", ""},
		{"Senior Security Engineer", "security", ""},
		{"Security Architect", "security", ""},
		{"Security Analyst", "security", ""},
		{"Information Security Officer", "security", "the officer title that IS technical"},
		{"Chief Information Security Officer", "security", "what the bare alias existed to protect"},
		{"IT Security Analyst", "security", ""},
		{"Application Security Engineer", "security", ""},
		{"Network Security Engineer", "security", ""},
		{"Cloud Security Engineer", "security", ""},
		{"Security Operations Analyst", "security", ""},
		{"Cyber Security Engineer", "security", "already qualified before this change"},

		// The unqualified officer is the guard: 175 of 192 sampled titles name no
		// information-security word. Counts are open postings.
		{"Security Officer", "public_safety", "1 102 of them, and a guard in nearly all"},
		{"Unarmed Security Officer", "public_safety", ""},
		{"Mall Security Officer", "public_safety", ""},
		{"Retail Security Officer", "public_safety", ""},
		{"Information Systems Security Officer (ISSO)", "security", "pinned ahead of the guard"},
		{"Data Security Officer", "security", ""},
		{"Security Officer, Information Security", "security", "the discipline named after the noun still wins"},
		{"Facility Security Officer", "", "defence-industry clearance administration, neither craft"},
		{"Security Specialist", "", "602, and genuinely either"},
		{"Security Supervisor", "", "149"},
		{"Security", "", "91, bare"},

		// The guard category, through its own entry rather than the fall-through.
		{"Security Guard", "public_safety", "was resolving to the infosec facet once"},
	} {
		if got := Parse(tc.title).Category; got != tc.want {
			t.Errorf("Parse(%q).Category = %q, want %q %s", tc.title, got, tc.want, tc.why)
		}
	}
}
