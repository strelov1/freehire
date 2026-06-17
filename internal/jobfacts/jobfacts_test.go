package jobfacts

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/enrich"
)

func TestEmploymentType(t *testing.T) {
	cases := []struct {
		name, title, desc, want string
	}{
		{"unstated -> empty", "Software Engineer", "Build great things.", ""},
		{"internship", "Software Engineering Intern", "A summer internship program.", "internship"},
		{"intern word not internal", "Engineer", "Work on internal international internet tools.", ""},
		{"part time", "Barista", "This is a part-time role, 20h/week.", "part_time"},
		{"contract", "Consultant", "6-month contract, fixed-term engagement.", "contract"},
		{"contractor", "Dev", "We hire a contractor for this.", "contract"},
		{"freelance", "Designer", "Freelance, remote.", "contract"},
		{"temporary -> contract", "Picker", "Temporary seasonal position.", "contract"},
		{"full time", "Engineer", "Full-time, permanent position.", "full_time"},
		{"internship beats full-time", "Intern", "A full-time internship for students.", "internship"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EmploymentType(c.title, c.desc); got != c.want {
				t.Errorf("EmploymentType(%q,%q) = %q, want %q", c.title, c.desc, got, c.want)
			}
		})
	}
}

func TestEducationLevel(t *testing.T) {
	cases := []struct{ name, desc, want string }{
		{"unstated", "Strong coding skills required.", ""},
		{"bachelor", "Bachelor's degree in CS or equivalent.", "bachelor"},
		{"bsc abbrev", "BSc in Computer Science required.", "bachelor"},
		{"master", "A Master's degree is preferred.", "master"},
		{"mba", "An MBA is a plus.", "master"},
		{"phd", "PhD in Machine Learning required.", "phd"},
		{"phd dotted", "Ph.D. or equivalent research experience.", "phd"},
		{"phd beats bachelor", "Bachelor's required, PhD preferred.", "phd"},
		{"bachelor degree no apostrophe", "A bachelor degree in CS is required.", "bachelor"},
		{"explicit none", "No degree required for this role.", "none"},
		{"degree word alone not enough", "This is a degree of difficulty.", ""},
		{"MS Office is not a master's", "Proficiency in MS Office and MS SQL Server.", ""},
		{"scrum master is not a degree", "Experienced scrum master leading the team.", ""},
		{"bare BS is not bachelor", "This role involves a lot of bs paperwork.", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EducationLevel(c.desc); got != c.want {
				t.Errorf("EducationLevel(%q) = %q, want %q", c.desc, got, c.want)
			}
		})
	}
}

func TestExperienceYearsMin(t *testing.T) {
	ptr := func(n int) *int { return &n }
	cases := []struct {
		name, desc string
		want       *int
	}{
		{"unstated", "Great communication skills.", nil},
		{"plain", "5 years of experience required.", ptr(5)},
		{"plus", "7+ years building distributed systems.", ptr(7)},
		{"range low end", "3-5 years of relevant experience.", ptr(3)},
		{"to range", "2 to 4 years experience.", ptr(2)},
		{"yrs abbrev", "10 yrs experience.", ptr(10)},
		{"min across mentions", "5 years of Go and 2 years of Kubernetes.", ptr(2)},
		{"age ignored", "Must be 18 years of age. 4+ years experience.", ptr(4)},
		{"hyperbole capped out", "100 years of fun, no experience needed.", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExperienceYearsMin(c.desc)
			switch {
			case got == nil && c.want == nil:
			case got == nil || c.want == nil:
				t.Errorf("ExperienceYearsMin(%q) = %v, want %v", c.desc, got, c.want)
			case *got != *c.want:
				t.Errorf("ExperienceYearsMin(%q) = %d, want %d", c.desc, *got, *c.want)
			}
		})
	}
}

// TestValuesAreInVocabulary guards that every value the matchers can return is a
// member of the enrichment contract's controlled vocabulary, so jobfacts and the
// served enum never drift apart.
func TestValuesAreInVocabulary(t *testing.T) {
	for _, v := range []string{"internship", "part_time", "contract", "full_time"} {
		if !slices.Contains(enrich.EmploymentTypeValues, v) {
			t.Errorf("employment_type %q not in enrich.EmploymentTypeValues", v)
		}
	}
	for _, v := range []string{"none", "bachelor", "master", "phd"} {
		if !slices.Contains(enrich.EducationLevelValues, v) {
			t.Errorf("education_level %q not in enrich.EducationLevelValues", v)
		}
	}
}
