package classify

import "testing"

// Indian title forms. Every title is a real one from the Indian part of the catalogue
// (sampled 2026-10-25) unless the case says otherwise.
func TestParseIndianSeniority(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		// The SDE ladder: I is the entry grade, II the mid grade, III the senior one.
		{"SDE 1 - Backend", "junior"},
		{"SDE-1 (Frontend)", "junior"},
		{"SDE I", "junior"},
		{"SDE1 - Android", "junior"},
		{"SDE 2 - Data Engineering", "middle"},
		{"SDE-II, Payments", "middle"},
		{"SDE II", "middle"},
		{"SDE 3 - Backend", "senior"},
		{"SDE-III", "senior"},
		{"SDET II", "middle"},
		{"Software Development Engineer II", "middle"},
		{"Software Development Engineer - I", "junior"},
		{"Software Development Engineer III, Alexa", "senior"},
		// Elsewhere the ladder's first two rungs read the same way; a bare III does not,
		// because employers disagree about it.
		{"Software Engineer II", "middle"},
		{"Software Engineer I", "junior"},
		{"Data Analyst II", "middle"},
		{"Engineer - II", "middle"},
		{"Java Developer 2-5 Years", ""},
		{"Software Engineer III", ""},
		// A stated grade still outranks the rung.
		{"Senior Software Engineer II", "senior"},

		// Associate before a role names the entry grade; elsewhere it names none.
		{"Associate Software Engineer", "junior"},
		{"Associate Consultant - Web Developer", "junior"},
		{"Associate Product Manager", "junior"},
		{"Associate Data Scientist", "junior"},
		{"Senior Associate - Finance", "senior"},
		{"Associate", ""},
		{"Associate Director, Engineering", ""},
		{"Sales Associate", ""},

		// Member of Technical Staff names no grade, its graded acronyms do.
		{"Member of Technical Staff", ""},
		{"MTS", ""},
		{"SMTS - Platform", "senior"},
		{"LMTS, Java", "lead"},
		{"PMTS", "principal"},

		// The entry hires.
		{"Fresher - Software Developer", "junior"},
		{"Freshers - Customer Support", "junior"},
		{"Graduate Engineer Trainee", "junior"},
		{"Graduate Engineer Trainee (GET) - Mechanical", "junior"},
		{"Management Trainee - Sales", "junior"},
		{"Trainee Engineer", "junior"},
		{"Trainee - Accounts", "intern"},
		{"Apprentice - Electrical", "intern"},
		{"Team Leader - Operations", "lead"},

		// AVP is a corporate title below VP, not an executive.
		{"AVP - Credit Risk", ""},
		{"Assistant Vice President - Operations", ""},
		{"Associate Vice President, Delivery", ""},
		{"Vice President - Engineering", "c_level"},
		{"Principal Engineer", "principal"},
	}
	for _, tc := range tests {
		if got := Parse(tc.title).Seniority; got != tc.want {
			t.Errorf("Parse(%q).Seniority = %q, want %q", tc.title, got, tc.want)
		}
	}
}
