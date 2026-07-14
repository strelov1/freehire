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
