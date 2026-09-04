package classify

import "testing"

func TestIsTech(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  bool
	}{
		// Positives — confident software/IT titles (from the prod unknown bucket).
		{"software engineer", "Senior Software Engineer", true},
		{"software engineer II", "Senior Software Engineer II", true},
		{"web3 developer", "Senior Web3 Developer", true},
		{"salesforce developer", "Senior Salesforce Developer", true},
		{"backend developer", "Backend Developer", true},
		{"full stack developer", "Full Stack Developer", true},
		{"devops engineer", "DevOps Engineer", true},
		{"sre", "Site Reliability Engineer", true},
		{"data scientist", "Lead Data Scientist", true},
		{"ml engineer", "Machine Learning Engineer", true},
		{"system administrator", "Senior System Administrator", true},
		{"it administrator", "Senior IT Administrator für Business Software", true},
		{"python developer", "Python Developer (Remote)", true},
		{"programmer", "COBOL Programmer", true},
		{"qa engineer", "QA Engineer", true},
		// Generalist software titles that state no sub-discipline. MTS is software in
		// 294 of 300 sampled prod postings (xAI, Pure Storage, Cockroach Labs); the
		// semiconductor tail carries its own fab suffixes.
		{"member of technical staff", "Member of Technical Staff", true},
		{"member of the technical staff", "Member of the Technical Staff, Pretraining", true},
		{"founding engineer", "Founding Engineer", true},
		// "AI-native" describes the toolchain, not the discipline — still software.
		{"ai-native engineer", "Senior AI-Native Engineer", true},
		{"ai native engineer", "AI Native Engineer", true},
		// The IT service desk: `support` is a non-tech category, so this list is the
		// only thing that reads the desk as the IT work it is.
		{"service desk", "Lead Service Desk Analyst", true},
		{"help desk", "Help Desk Technician (Tier 1)", true},
		{"helpdesk", "Mitarbeiter IT Support und Helpdesk (m/w/d)", true},
		{"technical support analyst", "Technical Support Analyst", true},
		{"it support", "1st Line IT Support Technician", true},
		{"it supporter", "1st Level IT Supporter (m/w/d)", true},
		{"desktop support", "Desktop Support Analyst - Tier 1", true},
		{"deskside support", "Deskside Support Engineer (Osaka)", true},
		{"end user support", "Analyst, End User Support", true},
		{"end-user support", "Desktop Support Specialist – SCCM, Intune, End-User Support", true},

		// Trap negatives — non-software engineering / non-tech that carry "engineer"
		// or other shared words. These MUST stay unflagged (bias: leave in unknown).
		{"mechanical engineer", "Senior Mechanical Engineer", false},
		{"manufacturing engineer", "Senior Manufacturing Engineer", false},
		{"project engineer", "Sr. Project Engineer", false},
		{"drainage engineer", "Senior Professional Engineer - Drainage", false},
		{"optical engineer", "Senior Optical Characterization Engineer", false},
		{"sales engineer", "Sales Engineer", false},
		{"geologist", "Senior Geologist", false},
		{"business developer", "Business Developer", false},
		// "Product Engineer" is deliberately absent from the term list: a prod sample
		// of 300 split 142 software (Attio, clasp, Circleback) against 64 manufacturing
		// (ABB, Howmet Aerospace, Texas Instruments, Flextronics). Not software-anchored,
		// so it stays unknown — the named role carries it instead.
		{"product engineer", "Product Engineer", false},
		// The desk terms above must not broaden into the surrounding nouns: bare
		// support is the whole customer-service population, and a front desk is a
		// lobby. (Both still resolve a category — `support` and `administration` —
		// this only pins that the TITLE detector makes no claim about them.)
		{"customer support analyst", "Customer Support Analyst", false},
		{"support specialist", "Support Specialist", false},
		{"front desk", "Front Desk Agent", false},
		// Bare "technical support" is deliberately absent: sampled live titles give
		// AGV, automotive, controls, logistics and instructional support alongside the
		// office-IT ones, so only the analyst form above is anchored enough.
		{"technical support engineer", "AGV Technical Support Engineer", false},
		{"technical support specialist", "Automotive Technical Support Specialist", false},
		// #2421 asked for these two as negatives. Note what the assertion can and
		// cannot say: both resolve a category that IS in vocab.TechCategories
		// (project_management, business_analysis), so both are already technical
		// EVIDENCE and were before this list existed. All that is pinned here is that
		// the title detector adds no claim of its own — which is why the IT-prefixed
		// forms the issue proposed would have changed no outcome.
		{"project manager", "Project Manager", false},
		{"business analyst", "Business Analyst", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTech(tt.title); got != tt.want {
				t.Errorf("IsTech(%q) = %v, want %v", tt.title, got, tt.want)
			}
		})
	}
}

// A software title that spells "design" in the middle must still read as technical.
// "software design engineer" is not adjacent to any techTitleTerms entry — wordmatch
// needs adjacency — so without its own term it fell through to unknown once the
// category stopped being `design`.
func TestIsTech_SoftwareDesignEngineer(t *testing.T) {
	for _, title := range []string{
		"Software Design Engineer",
		"Senior Software Design Engineer",
		"Software Design Engineer in Test",
		// The "-ing" spelling needs its own term: wordmatch is boundary-aware, so
		// "engineer" cannot see "engineering", and these titles carry no category at
		// all — this detector is the only thing left to read them as technical.
		"Software Design Engineering Manager",
		"Director, Software Design Engineering",
	} {
		if !IsTech(title) {
			t.Errorf("IsTech(%q) = false, want true", title)
		}
	}
}
