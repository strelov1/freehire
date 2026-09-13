package edulevel

import "testing"

func TestForRequirement(t *testing.T) {
	cases := []struct{ name, desc, want string }{
		{"unstated", "Strong coding skills required.", ""},
		{"bachelor", "Bachelor's degree in CS or equivalent.", "bachelor"},
		{"bsc abbrev", "BSc in Computer Science required.", "bachelor"},
		{"master", "A Master's degree is required.", "master"},
		{"mba", "An MBA is required.", "master"},
		{"phd", "PhD in Machine Learning required.", "phd"},
		{"phd dotted", "Ph.D. or equivalent research experience.", "phd"},
		{"phd beats bachelor", "Bachelor's or PhD in a quantitative field.", "phd"},
		{"bachelor degree no apostrophe", "A bachelor degree in CS is required.", "bachelor"},
		{"typographic apostrophe", "Bachelor’s degree required.", "bachelor"},
		{"typographic apostrophe, master", "Master’s degree required.", "master"},
		{"explicit none", "No degree required for this role.", "none"},
		{"degree word alone not enough", "This is a degree of difficulty.", ""},
		{"MS Office is not a master's", "Proficiency in MS Office and MS SQL Server.", ""},
		{"scrum master is not a degree", "Experienced scrum master leading the team.", ""},
		{"bare BS is not bachelor", "This role involves a lot of bs paperwork.", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ForRequirement(c.desc); got != c.want {
				t.Errorf("ForRequirement(%q) = %q, want %q", c.desc, got, c.want)
			}
		})
	}
}

func TestForDegree(t *testing.T) {
	cases := []struct{ name, degree, want string }{
		{"bsc", "BSc", "bachelor"},
		{"bsc dotted", "B.Sc.", "bachelor"},
		{"bare bs", "BS", "bachelor"},
		{"bachelor of science", "Bachelor of Science in Computer Science", "bachelor"},
		{"msc", "MSc", "master"},
		{"msc dotted", "M.Sc.", "master"},
		{"bare ms", "MS", "master"},
		{"mba", "MBA", "master"},
		{"master of x", "Master of Business Administration", "master"},
		{"phd", "PhD", "phd"},
		{"phd dotted", "Ph.D.", "phd"},
		{"doctorate", "Doctorate in Physics", "phd"},
		{"phd beats bachelor", "BSc, PhD in Physics", "phd"},
		{"unresolved certificate", "Certificate in Project Management", ""},
		{"unresolved diploma", "Diploma", ""},
		{"unresolved empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ForDegree(c.degree); got != c.want {
				t.Errorf("ForDegree(%q) = %q, want %q", c.degree, got, c.want)
			}
		})
	}
}
