package classify

import "testing"

// TestIsTech_LostTitles covers the title forms the catalogue demonstrably carries and
// this detector demonstrably missed. Every title here is a real prod title, taken from
// the 160 most common among 2,232,773 open canonical postings that carried NO is_tech
// signal on 2026-09-23 — 71,314 of which read as software or IT outright.
//
// The negatives matter more than the positives, and they come from the SAME corpus:
// they are postings a careless term would have swept in, not invented counterexamples.
// "Job Developer" is a placement counsellor. "Project Developer" builds real estate.
// "Project Engr II" and "Field Service Engr II" are construction and field service.
func TestIsTech_LostTitles(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  bool
	}{
		// --- Vendor platforms. The platform names the discipline as surely as a
		// language does, which is the anchor rule the requirement states.
		{"mulesoft", "MuleSoft Developer", true},
		{"pega", "Pega Developer", true},
		{"appian", "Appian Lead Developer", true},
		{"power platform", "Senior Power Platform Developer", true},
		{"guidewire", "Guidewire Developer", true},
		{"zoho", "Zoho Developer", true},
		{"netsuite", "NetSuite Developer", true},
		{"servicenow spaced", "Service Now Developer", true},
		{"peoplesoft", "PeopleSoft Developer", true},
		{"sitecore", "Sitecore Developer", true},
		{"aem", "AEM Developer", true},
		{"informatica", "Informatica Developer", true},
		{"hadoop", "Hadoop Developer", true},
		{"unity", "Unity Developer", true},
		{"laravel", "Laravel Developer", true},
		{"rust", "Rust Developer", true},
		{"flutter", "Senior Flutter Developer", true},
		{"mendix", "Mendix Lead Developer", true},
		{"sql", "SQL Developer", true},
		{"plsql", "Oracle PL/SQL Developer", true},
		{"cobol", "COBOL Developer", true},
		{"api", "API Developer", true},
		{"gis", "GIS Developer", true},

		// --- Level-qualified developer. Admissible because the match is a PHRASE:
		// "senior developer" cannot occur inside "Senior Business Developer", where the
		// two words are not adjacent. Verified on prod — of the twenty commonest open
		// titles carrying the phrase, all twenty are software roles.
		{"senior developer", "Senior Developer", true},
		{"lead developer", "Lead Developer", true},
		{"junior developer", "Junior Developer", true},
		{"senior developer qualified", "Java Senior Developer", true},

		// --- The same words with a different qualifier. This is the whole reason bare
		// "developer" is not a term, and the reason the level terms are phrases.
		{"business developer", "Business Developer", false},
		{"senior business developer", "Senior Business Developer", false},
		{"job developer", "Job Developer", false},
		{"product developer", "Product Developer", false},
		{"project developer", "Project Developer", false},

		// --- IT-anchored roles. Always two words: lowercased for matching, bare "it"
		// is the English pronoun.
		{"it officer", "IT Officer", true},
		{"it supervisor", "IT Supervisor", true},
		{"it assistant", "IT Assistant", true},
		{"it associate", "IT Associate", true},
		{"it trainer", "IT Trainer", true},
		{"it auditor", "Senior IT Internal Auditor", true},
		{"it coordinator", "IT Coordinator", true},
		{"it consultant", "IT Consultant", true},
		{"it technician", "IT Field Technician", true},
		{"it operations", "IT Operations Engineer", true},
		{"it intern", "IT Intern", true},
		{"it director", "IT Director", true},
		{"it executive", "IT Executive", true},
		{"head of it", "Head of IT", true},
		{"director of it", "Director of IT", true},
		{"it projektmanager", "IT-Projektmanager (m/w/d)", true},

		// The pronoun trap the two-word rule exists for.
		{"it as a pronoun", "Make It Happen Coordinator", false},
		{"it as a pronoun mid-title", "Own It Program Lead", false},

		// --- Surface forms. Word boundaries cannot see past themselves, so a plural
		// and an abbreviation are each their own entry — the same argument the
		// "software design engineering" term already carries.
		{"software engr roman", "Software Engr II", true},
		{"software engr advanced", "Advanced Software Engr", true},
		{"software engineers plural", "Software Engineers", true},
		{"data engineers plural", "Data Engineers", true},

		// The abbreviation is NOT the anchor. Both of these are in the same corpus.
		{"project engr", "Project Engr II", false},
		{"field service engr", "Field Service Engr II", false},
		{"advanced project engr", "Advanced Project Engr", false},

		// --- Second wave. Probing the corpus with the terms above showed what they
		// still left behind; these are that residue, and finding them is the whole
		// argument for probing rather than curating from memory.
		//
		// Spellings the existing terms cannot reach, because a hyphen, a slash, a
		// space or a comma is a word boundary and adjacency is required.
		{"it administrator hyphen", "IT-Administrator (m/w/d)", true},
		{"dot net spaced", "Dot Net Developer", true},
		{"j2ee slashed", "Java/J2EE Developer", true},
		{"react js spaced", "React JS Developer", true},
		{"website not web", "Website Developer", true},
		{"engineer comma software", "Engineer, Software", true},
		{"software eng abbreviated", "Sr Software Eng Supervisor", true},
		{"software engineering", "Software Engineering Intern", true},
		{"software engineering in a long title", "Associate Director, Software Engineering", true},

		// Russian surface forms the catalogue carries in volume.
		{"it specialist russian", "IT-специалист", true},
		{"it specialist english", "Senior IT Specialist", true},
		// A word wedged between "IT" and the role breaks adjacency and is NOT claimed.
		// "Senior IT Pillar Specialist" is a real prod title and stays unknown — the
		// dictionary carries the spellings the catalogue uses, not every arrangement
		// of the same words.
		{"it with a word wedged in", "Senior IT Pillar Specialist", false},

		// More anchored developer forms.
		{"quantitative", "Quantitative Developer", true},
		{"quant", "Quant Developer", true},
		{"automation", "Automation Developer", true},
		{"applications", "Senior Applications Developer", true},
		{"cloud developer", "Senior Cloud Developer", true},
		{"data developer", "Data Developer", true},
		{"ai developer", "AI Developer", true},
		{"ai agent developer", "AI Agent Developer", true},
		{"crm developer", "CRM Developer", true},
		{"integration developer", "Integration Developer", true},
		{"big data developer", "Big Data Developer", true},
		{"azure developer", "Azure Developer", true},
		{"ab initio", "Ab Initio Developer", true},
		{"mern", "MERN Stack Developer", true},

		// Engineer forms with a technology anchor.
		{"kubernetes engineer", "Kubernetes Engineer", true},
		{"react engineer", "Senior React Engineer", true},
		{"react native engineer", "Senior React Native Engineer", true},

		// Other anchored software roles.
		{"software consultant", "Software Consultant", true},
		{"software team lead", "Software Team Lead", true},
		{"sql dba", "SQL DBA", true},
		{"database analyst", "Database Analyst", true},
		{"cyber intelligence", "Cyber Intelligence Analyst", true},

		// Systems Engr stays OUT, and deliberately so: "Systems Engineer" is resolved
		// by the CATEGORY dictionary, which carries a blindness rule holding back
		// "Control Systems Engineer", "Power Systems Engineer" and the rest of the
		// industrial family (it-title-coverage). A term here would reach past that
		// rule and sweep them in, since this detector wins outright over any category.
		{"systems engr not claimed here", "Systems Engr II", false},
		{"control systems engr", "Control Systems Engr", false},

		// --- Third wave, from re-probing after the second. Same method, same reason.
		{"it business partner", "IT Business Partner", true},
		{"it supporter hyphen", "IT-Supporter (m/w/d)", true},
		{"it apprentice", "IT Apprentice", true},
		{"it lead", "IT Lead", true},
		{"cyber sec abbreviated", "Advanced Cyber Sec Archt/Engr", true},
		{"software intern", "Software Intern", true},
		{"software verification", "Software Verification Engineer", true},
		{"principal engineer software", "Principal Engineer Software", true},
		{"algorithm developer", "Algorithm Developer", true},
		{"database reliability", "Database Reliability Engineer", true},
		{"scala", "Scala Developer", true},
		{"teradata", "Teradata Developer", true},
		{"power apps", "Power Apps Developer", true},
		{"as400", "AS400 Developer", true},
		{"cloud consultant", "Cloud Consultant", true},
		{"java developers plural", "Java Developers", true},

		// "it lead" must not reach inside a longer word. Word boundaries already
		// guarantee this, and these pin it: a term this short is where a boundary bug
		// would show first.
		{"unit lead", "Unit Lead", false},
		{"audit lead", "Audit Lead", false},
		{"recruit leader", "Recruit Leader", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTech(tt.title); got != tt.want {
				t.Errorf("IsTech(%q) = %v, want %v", tt.title, got, tt.want)
			}
		})
	}
}
