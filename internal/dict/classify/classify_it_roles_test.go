package classify

import (
	"testing"
)

// TestParse_ITCompanyRoles covers the ten IT-company role categories added by the
// expand-role-taxonomy change, with the fall-through guards that keep the terminal
// analyst->data_analytics / manager->management aliases (and bare sales / ux) from
// stealing a more specific role.
func TestParse_ITCompanyRoles(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// recruiting
		{"Technical Recruiter", "recruiting"},
		{"IT Recruiter", "recruiting"},
		{"Talent Acquisition Specialist", "recruiting"},
		{"Рекрутер", "recruiting"},
		// hr
		{"HR Business Partner", "hr"},
		{"HRBP", "hr"},
		{"People Operations Manager", "hr"},
		{"Менеджер по персоналу", "hr"},
		// finance — guards against analyst-> and manager-> fall-throughs
		{"Financial Analyst", "finance"},
		{"Chief Financial Officer", "finance"},
		{"Bookkeeper", "finance"},
		{"Finance Manager", "finance"},
		{"Главный бухгалтер", "finance"},
		// legal
		{"Legal Counsel", "legal"},
		{"Compliance Officer", "legal"},
		{"Paralegal", "legal"},
		{"Юрисконсульт", "legal"},
		// operations — guard against manager-> fall-through
		{"Operations Manager", "operations"},
		{"Office Manager", "operations"},
		{"Chief of Staff", "operations"},
		{"Специалист по закупкам", "operations"},
		// customer_success — split out of support
		{"Customer Success Manager", "customer_success"},
		{"Onboarding Specialist", "customer_success"},
		{"Renewals Manager", "customer_success"},
		// business_analysis — guard against analyst-> fall-through; BI routes to analytics
		{"Business Analyst", "business_analysis"},
		{"Systems Analyst", "business_analysis"},
		{"Системный аналитик", "business_analysis"},
		{"BI Analyst", "data_analytics"},
		{"Business Intelligence Analyst", "data_analytics"},
		// solutions_engineering — guard: beats bare sales
		{"Sales Engineer", "solutions_engineering"},
		{"Solutions Consultant", "solutions_engineering"},
		{"Пресейл-инженер", "solutions_engineering"},
		// developer_relations
		{"Developer Advocate", "developer_relations"},
		{"DevRel", "developer_relations"},
		{"Technical Evangelist", "developer_relations"},
		// technical_writing — guard: beats ux/designer
		{"Technical Writer", "technical_writing"},
		{"UX Writer", "technical_writing"},
		{"Content Designer", "technical_writing"},
		{"Технический писатель", "technical_writing"},

		// precision — existing behavior must be unchanged
		{"Solutions Architect", "architecture"}, // stays architecture, not solutions_engineering
		{"Content Writer", "marketing"},         // stays marketing, not technical_writing
		{"Account Manager", "sales"},            // stays sales, not customer_success
		{"Data Analyst", "data_analytics"},      // plain analyst role unaffected
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_UnnamedITRoles pins the IT roles that reached a category only through
// one of the table's bare fall-throughs — "analyst" reads every remaining analyst as
// data_analytics — so they resolved confidently WRONG rather than not at all. Each
// now names its own discipline.
func TestParse_UnnamedITRoles(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		{"Lead Service Desk Analyst", "it_support"},
		{"IT Service Desk Specialist Level II", "it_support"},
		{"SOC Analyst L2", "security"},
		{"Director - IT Infrastructure", "devops"},
		{"IT Infrastructure Analyst (Networks and Telecoms)", "devops"},
		{"MES Developer (Delmia Apriso)", "software_engineering"},
		// Unchanged: the bare nouns keep falling through as before.
		{"Business Analyst", "business_analysis"},
		{"Infrastructure Project Manager", "project_management"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// "IT Infrastructure" is a domain noun, so it must resolve only after every role
// noun. These are the titles that pin the placement: move the entry back up into the
// devops block and all three become devops, which is wrong for all three.
func TestParse_ITInfrastructureYieldsToTheRoleNoun(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		{"IT Infrastructure Security Engineer", "security"},
		{"IT Infrastructure Project Manager", "project_management"},
		// Not sales: "Cyber Security" names a discipline outright and the security
		// block outranks the sales one. The pin here is only that it is not devops.
		{"Business Development Executive (IT Infrastructure/Cyber Security)", "security"},
		// What is left once no role noun claims the title is the estate itself.
		{"Head of IT Infrastructure", "devops"},
		{"IT Infrastructure Manager", "devops"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_HungarianSoftwareTitles pins the Hungarian software vocabulary, added
// after auditing a general-population Hungarian board where the IT sections are not
// where every technical posting is filed. The negatives are the point: "fejlesztő"
// is the bare "developer" noun, and the table's comment lists the non-software
// senses the catalogue uses it for.
func TestParse_HungarianSoftwareTitles(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		{"Szoftverfejlesztő - ERP terület", "software_engineering"},
		{"Lead Szoftverfejlesztő", "software_engineering"},
		{"Szoftverfejlesztő mérnök (C#)", "software_engineering"},
		{"Termelési software mérnök", "software_engineering"},
		{"Hardverközeli alkalmazásfejlesztő", "software_engineering"},
		{"Üzleti alkalmazás fejlesztő gyakornok", "software_engineering"},
		{"Java fejlesztő", "software_engineering"},
		{"JAVA fejlesztők", "software_engineering"},
		{"Python fejlesztő (Python, Django)", "software_engineering"},
		{"Node.js fejlesztő", "software_engineering"},
		{"Senior .net fejlesztő", "software_engineering"},
		{"Oracle adatbázis fejlesztő", "software_engineering"},
		{"SAP ABAP fejlesztő (LJK)", "software_engineering"},
		{"Senior PEGA fejlesztő", "software_engineering"},
		{"Odoo-fejlesztő", "software_engineering"},
		{"Webfejlesztő", "software_engineering"},
		{"Adattárház fejlesztő", "data_engineering"},
		{"BI fejlesztő", "data_analytics"},
		{"BI riportfejlesztő", "data_analytics"},
		{"Senior mobilalkalmazás-fejlesztő (React Native)", "mobile"},
		// "beágyazott" precedes the plain software entry, so embedded wins.
		{"Beágyazott szoftverfejlesztő", "embedded"},

		// The bare noun stays unresolved in every non-software sense.
		{"Anyagfejlesztő - gumikeverékek", ""},
		{"Beszállítófejlesztő mérnök / Supplier Development Engineer (DMS)", ""},
		{"Folyamatfejlesztő és automatizációs szakértő", ""},
		{"Fejlesztő mérnök", ""},
		{"Fejlesztőpedagógus", ""},
		{"Képzés-fejlesztési gyakornok", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}

// TestParse_HungarianOperationsTitles pins the Hungarian vocabulary for the
// disciplines the software block above does not reach: running systems, testing
// them, securing them, and the network and analytics seats beside them. Every case
// is a live title, taken from the 7,207 open Hungarian postings the catalogue held
// on 2026-09-06 — the software block was mined the same way, and the negatives are
// again the point.
//
// The negatives are all bare nouns that read as technical to an English eye and are
// not: "üzemeltető" is whoever runs a thing (a machine, a building, a fuel station),
// "biztonsági" covers the armed guard and the fire warden, and "hálózat" is any
// network including a retail chain's.
func TestParse_HungarianOperationsTitles(t *testing.T) {
	cases := []struct{ title, wantCategory string }{
		// Systems administration. "rendszergazda" is a closed compound with no
		// non-IT sense: all 17 live occurrences are this role.
		{"Rendszergazda", "devops"},
		{"Linux rendszergazda", "devops"},
		{"IT Rendszergazda (Microsoft)", "devops"},
		{"Junior kliens rendszergazda", "devops"},
		{"Üzemeltető Rendszergazda – Windows (Linux)", "devops"},
		{"Junior / medior IT-rendszergazda", "devops"},
		// The qualified "üzemeltető" compounds, closed and spaced. Hungarian writes
		// both, so both are entries — a closed compound does not contain its spaced
		// twin on a word boundary.
		{"Rendszerüzemeltető", "devops"},
		{"IT rendszerüzemeltető", "devops"},
		{"Informatikai rendszerüzemeltető", "devops"},
		{"Alkalmazás üzemeltető", "devops"},
		{"Alkalmazásüzemeltető (banki rendszerek - fedezetlen hitelezés)", "devops"},
		{"Szenior alkalmazás üzemeltető - Vault", "devops"},
		{"IT üzemeltető", "devops"},
		{"IT Infrastruktúra Üzemeltető", "devops"},
		{"Informatikai üzemeltető", "devops"},
		{"IT rendszer- és szoftverüzemeltető informatikus", "devops"},
		// Network. "hálózatüzemeltető" is the closed compound again.
		{"Hálózati mérnök", "network_engineering"},
		{"Hálózati rendszermérnök", "network_engineering"},
		// "hálózatüzemeltető" is NOT admitted: a "hálózat" is any network, and a gas
		// utility runs one. Three of the four live occurrences are gas fitters
		// ("Hálózatüzemeltető /Gázszerelő", OPUS TIGÁZ Zrt.), and the fourth says
		// "Cisco" — which the dictionary reads on its own.
		{"Hálózatüzemeltető /Gázszerelő", ""},
		{"Hálózatüzemeltető/gázszerelő", ""},
		// Testing. The bare "tesztelő" is admitted on the same footing English's
		// bare "tester" already has: every live occurrence is a testing seat.
		{"Manuális tesztelő", "qa"},
		{"Automata tesztelő", "qa"},
		{"Szoftvertesztelő", "qa"},
		{"Tesztmenedzser (performancia tesztelés)", "qa"},
		// The bare noun is NOT admitted, on the evidence of a general-population board
		// rather than the developer boards the first sample came from: "Szivattyú
		// tesztelő" is whoever tests pumps. English's bare "tester" keeps its entry,
		// because the boards it is read on are not general-population ones.
		{"Szivattyú tesztelő", ""},
		{"Termék tesztelő", ""},
		// Security, in its three live spellings.
		{"Kiberbiztonsági munkatárs", "security"},
		{"Senior Kiberbiztonsági szakértő", "security"},
		{"Információbiztonsági felelős", "security"},
		{"Információ Biztonsági Felelős (Regionális)", "security"},
		{"IT biztonsági szakértő (GRC) (V-1218)", "security"},
		{"Német nyelvű Senior IT-biztonsági tanácsadó (REF5386A)", "security"},
		{"Junior informatikai biztonsági munkatárs", "security"},
		// Analytics, noun and adjective form.
		{"Adatelemző szakértő", "data_analytics"},
		{"Adatelemzési Munkatárs", "data_analytics"},

		// The bare nouns stay unresolved.
		{"Gépész üzemeltető", ""},
		{"Villamos Üzemeltető", ""},
		{"Épületüzemeltetési mérnök", ""},
		{"Létesítményüzemeltetési koordinátor", ""},
		{"Biztonsági operátor és fegyveres biztonsági őr", ""},
		{"Tűz- és munkabiztonsági szakértő", ""},
		{"Országos Üzlethálózati Tréner (Audiológia és Értékesítés)", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.wantCategory)
		}
	}
}
