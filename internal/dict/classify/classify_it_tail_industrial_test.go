package classify

import (
	"testing"
)

// TestParse_ITTailCoverage pins the IT titles the catalogue carries in volume that
// the category dictionary had no word for. Measured on prod 2026-09-02: 47.5% of open
// postings reached the index with no role, ALL of them because no category resolved.
func TestParse_ITTailCoverage(t *testing.T) {
	cases := []struct {
		title        string
		wantCategory string
		why          string
	}{
		// Russian software. The qualified spellings put the technology FIRST, and a
		// hyphen is a word boundary, so only the bare token reaches them.
		{"Программист", "software_engineering", ""},
		{"Разработчик", "software_engineering", ""},
		{"Java-разработчик", "software_engineering", "the bare token is the only alias that can see this"},
		{"Python-разработчик", "software_engineering", ""},
		{"Инженер-программист", "software_engineering", ""},
		{"Техник-программист", "software_engineering", ""},
		{"Системный администратор", "devops", ""},
		{"Администратор баз данных", "devops", ""},
		{"Сетевой администратор", "network_engineering", ""},

		// The Systems Engineer family. The non-IT namesakes are declared blind ABOVE
		// the bare alias, so they keep resolving to nothing instead of being swept in.
		{"Systems Engineer", "software_engineering", "1440 open postings spell it exactly"},
		{"System Engineer", "software_engineering", ""},
		{"Systems Engineer II", "software_engineering", ""},
		{"IT Systems Engineer", "software_engineering", ""},
		{"Software Systems Engineer", "software_engineering", ""},
		{"Linux Systems Engineer", "devops", ""},
		{"Cyber Systems Engineer", "security", ""},
		{"Control Systems Engineer", "", "industrial; must not be swept into software"},
		{"Power Systems Engineer", "", ""},
		{"Electrical Systems Engineer", "", ""},
		{"Quality Systems Engineer", "", ""},

		// Vendor platforms — naming the product states the discipline.
		{"ServiceNow Developer", "software_engineering", ""},
		{"ServiceNow Engineer", "software_engineering", ""},
		{"ServiceNow Administrator", "devops", ""},
		{"Salesforce Administrator", "software_engineering", ""},
		{"Salesforce Engineer", "software_engineering", ""},
		{"Salesforce Consultant", "software_engineering", ""},
		{"Salesforce Developer", "software_engineering", "already resolved; must not regress"},
		{"Mainframe Developer", "software_engineering", ""},
		{"Oracle DBA", "devops", ""},
		{"SharePoint Administrator", "devops", ""},
		{"Tableau Developer", "data_analytics", ""},

		// Infrastructure and end-user IT.
		{"Data Center Technician", "devops", ""},
		{"Data Center Engineer", "devops", ""},
		{"Release Engineer", "devops", ""},
		{"Cloud Operations Engineer", "devops", ""},
		{"Cloud Migration Engineer", "devops", ""},
		{"Network Operations Engineer", "devops", ""},
		{"Network Specialist", "network_engineering", ""},
		{"Network Technician", "network_engineering", ""},
		{"IT Specialist", "it_support", "the same desk \"IT Support Specialist\" already resolves to"},
		{"IT Technician", "it_support", ""},

		// German fused compounds. German joins a title's role words into one
		// unbroken word with no separator, so the word-boundary matcher that
		// resolves a hyphenated Russian title or a spaced English one can never
		// reach the German spelling unless the fused form is its own alias.
		{"Systemadministrator (m/w/d)", "devops", ""},
		{"IT-Systemadministrator (m/w/d)", "devops", "a hyphen is a word boundary, so the bare alias reaches this"},
		{"IT Systemadministrator (m/w/d)", "devops", "a space is a word boundary too"},
		{"Netzwerkadministrator (m/w/d)", "network_engineering", ""},
		{"Datenbankadministrator (m/w/d)", "devops", ""},
		{"Netzwerktechniker (m/w/d)", "network_engineering", ""},
		{"IT-Netzwerktechniker (m/w/d)", "network_engineering", ""},
		{"Softwaretester (m/w/d)", "qa", "bare \"tester\" cannot see this — it is fused, not word-bounded"},
		{"Anwendungsentwickler (m/w/d)", "software_engineering", ""},
		{"Inhouse Anwendungsentwickler (m/w/d)", "software_engineering", ""},

		// Systemtechniker/Systemelektroniker also name non-IT disciplines (electrical,
		// physical-security systems work) — the same trap the Systems Engineer family
		// above documents. Only the IT-qualified spellings resolve.
		{"IT Systemtechniker (m/w/d)", "devops", ""},
		{"IT-Systemtechniker (m/w/d)", "devops", ""},
		{"IT Systemelektroniker (m/w/d)", "devops", ""},
		{"IT-Systemelektroniker (m/w/d)", "devops", ""},
		{"Systemtechniker (m/w/d) Elektrotechnik", "", "electrical/controls work, not IT"},
		{"Systemtechniker Sicherheitstechnik (m/w/d)", "", "physical security systems, not IT"},

		// Fachinformatiker: the German formal IT-specialist title and
		// apprenticeship. It never names a non-IT role, so the bare word resolves;
		// its two dominant qualifiers (declared before the bare fallback) resolve
		// more precisely when adjacent to the word.
		{"Fachinformatiker (m/w/d)", "devops", "bare fallback; never names a non-IT role"},
		{"Fachinformatiker Systemintegration (m/w/d)", "devops", ""},
		{"Fachinformatiker für Systemintegration (m/w/d)", "devops", ""},
		{"Fachinformatiker Anwendungsentwicklung (m/w/d)", "software_engineering", ""},
		{"Fachinformatiker für Anwendungsentwicklung (m/w/d)", "software_engineering", ""},

		// SPS-Programmierer (PLC/industrial-controller programming) is explicitly
		// out of scope, unchanged: it is already deliberately excluded from a
		// software category, on the same reasoning as "CNC Programmer" above.
		{"SPS-Programmierer (m/w/d)", "", "industrial/PLC, not software — same as CNC Programmer"},

		// The integration family.
		{"Integration Engineer", "software_engineering", ""},
		{"Systems Integration Engineer", "software_engineering", ""},
		{"Software Integration Engineer", "software_engineering", ""},
		{"Data Integration Engineer", "software_engineering", ""},
		{"Cloud Integration Engineer", "software_engineering", ""},

		// Collisions the mining pass surfaced. All four are artefacts of naive
		// substring matching in that script ("count-ERP-erson", "P-IT T-echnician");
		// the production dictionary matches on word boundaries and cannot make that
		// mistake, but the next reader will not know that.
		{"Parts Counterperson", "", ""},
		{"Parts Interpreter", "retail", "the dealership parts counter; the point here is that it is not IT"},
		{"Pit Technician", "", ""},
		{"SAP Operations Clerk Part Time Day", "", ""},

		// Customer-facing engineering keeps what it has.
		{"Sales Engineer", "solutions_engineering", ""},
		{"Support Engineer", "support", ""},
	}

	for _, tc := range cases {
		if got := Parse(tc.title).Category; got != tc.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q %s", tc.title, got, tc.wantCategory, tc.why)
		}
	}
}

// TestParse_IndustrialEngineering pins the engineering seats outside software — the
// whole residue left after the IT wave, 51 994 open postings measured on prod, half of
// it Russian and none of that half carrying an English alias.
func TestParse_IndustrialEngineering(t *testing.T) {
	cases := []struct {
		title        string
		wantCategory string
		why          string
	}{
		// The seats a plant, factory or field-service organisation staffs.
		{"Project Engineer", "industrial_engineering", ""},
		{"Quality Engineer", "industrial_engineering", ""},
		{"Senior Quality Engineer", "industrial_engineering", ""},
		{"Process Engineer", "industrial_engineering", ""},
		{"Manufacturing Engineer", "industrial_engineering", ""},
		{"Production Engineer", "industrial_engineering", ""},
		{"Maintenance Engineer", "industrial_engineering", ""},
		{"Controls Engineer", "industrial_engineering", ""},
		{"Automation Engineer", "industrial_engineering", "deferred by the IT wave for want of a home"},
		{"Commissioning Engineer", "industrial_engineering", ""},
		{"Validation Engineer", "industrial_engineering", ""},
		{"Industrial Engineer", "industrial_engineering", ""},
		{"Field Service Engineer", "industrial_engineering", ""},
		{"Field Engineer", "industrial_engineering", ""},
		{"Site Engineer", "industrial_engineering", ""},
		{"Facilities Engineer", "industrial_engineering", ""},
		{"Supplier Quality Engineer", "industrial_engineering", ""},
		{"Environmental Engineer", "industrial_engineering", ""},
		{"Geotechnical Engineer", "industrial_engineering", ""},
		{"Application Engineer", "industrial_engineering", "the industrial reading; the field form goes to pre-sales"},
		{"Applications Engineer", "industrial_engineering", ""},

		// NO bare "engineer" alias, though 689 open postings spell it exactly. It was
		// tried; the existing suite rejected it, and that rejection is the right
		// answer — "Product Engineer", "Growth Engineer" and "Staff Engineer" are
		// pinned to no category on purpose, and `Categories()` returns every match, so
		// a bare alias would append this category to every engineering title there is.
		{"Engineer", "", ""},
		{"Engineer II", "", ""},
		{"Associate Engineer", "", ""},
		{"Product Engineer", "", "pinned elsewhere on purpose; the bare alias would have overridden it"},
		{"Reliability Engineer", "", "mechanical and site reliability share the phrase"},

		// The IT lookalikes, declared above the bare alias.
		{"IT Engineer", "software_engineering", ""},
		{"Database Engineer", "devops", ""},
		{"Business Intelligence Engineer", "data_analytics", ""},
		{"Electronics Engineer", "hardware", "the rest of electronics already lives there"},
		{"Field Application Engineer", "solutions_engineering", "the semiconductor pre-sales title"},

		// The Russian family.
		{"Инженер", "industrial_engineering", ""},
		{"Инженер-технолог", "industrial_engineering", ""},
		{"Инженер-энергетик", "industrial_engineering", ""},
		{"Инженер-механик", "industrial_engineering", ""},
		{"Инженер-электроник", "industrial_engineering", ""},
		{"Инженер ПТО", "industrial_engineering", ""},
		{"Главный инженер", "industrial_engineering", ""},
		{"Инженер по подготовке производства", "industrial_engineering", ""},
		{"Инженер по наладке и испытаниям", "industrial_engineering", ""},
		{"Технолог", "industrial_engineering", ""},

		// Russian titles that name another discipline, declared above the bare token.
		{"Инженер-проектировщик", "engineering_design", "a draughtsman, not a plant engineer"},
		{"Инженер по защите информации", "security", ""},

		// Every discipline that names itself must be untouched by the bare alias.
		{"Backend Engineer", "backend", ""},
		{"Data Engineer", "data_engineering", ""},
		{"Security Engineer", "security", ""},
		{"Site Reliability Engineer", "sre", ""},
		{"Sales Engineer", "solutions_engineering", ""},
		{"Support Engineer", "support", ""},
		{"Systems Engineer", "software_engineering", "the IT wave's call, unchanged"},
		{"Software Engineer", "software_engineering", ""},
		{"QA Automation Engineer", "qa", "the QA alias resolves above automation"},
		{"Mechanical Design Engineer", "engineering_design", ""},
		{"Design Engineer", "engineering_design", ""},
		{"Machine Learning Engineer", "ml_ai", ""},
		{"Программист", "software_engineering", "the IT wave's call, unchanged"},
	}

	for _, tc := range cases {
		if got := Parse(tc.title).Category; got != tc.wantCategory {
			t.Errorf("Parse(%q).Category = %q, want %q %s", tc.title, got, tc.wantCategory, tc.why)
		}
	}
}
