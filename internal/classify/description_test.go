package classify

import "testing"

func TestSeniorityFromDescription(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string
	}{
		// Positives — intent-anchored grade statements.
		{"head of via intent anchor", "We are looking for a Head of Engineering to lead the team.", "c_level"},
		{"vp of", "You will be our VP of Engineering.", "c_level"},
		{"principal engineer", "This is a principal engineer position.", "principal"},
		{"staff engineer", "Join us as a staff engineer.", "staff"},
		{"lead role", "This is a lead role on the platform team.", "lead"},
		{"looking for a lead", "We are looking for a lead developer.", "lead"},
		{"senior-level", "A senior-level backend opening.", "senior"},
		{"senior position", "This senior position is fully remote.", "senior"},
		{"looking for a senior", "We are looking for a senior engineer.", "senior"},
		{"mid-level", "A mid-level role with growth.", "middle"},
		{"entry-level", "An entry-level opportunity for grads.", "junior"},
		{"junior position", "This junior position suits new grads.", "junior"},
		{"internship", "A summer internship in our Berlin office.", "intern"},

		// Priority — a higher grade wins when two appear.
		{"principal beats senior", "A principal engineer mentoring senior staff.", "principal"},

		// Trap negatives — incidental prose that must NOT set a grade.
		{"senior management", "You will collaborate with senior management.", ""},
		{"lead the team", "You will lead the team of five engineers.", ""},
		{"junior colleagues", "Mentor junior colleagues across the org.", ""},
		{"our staff", "We care deeply about our staff and culture.", ""},
		{"principal component analysis", "Experience with principal component analysis.", ""},
		{"report to head of", "You will report to the Head of Product.", ""},
		{"years of experience", "We require 5+ years of experience in Go.", ""},
		{"senior level of commitment", "We expect a senior level of commitment.", ""},
		{"no grade phrase", "Build resilient payment systems with us.", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SeniorityFromDescription(tt.desc); got != tt.want {
				t.Errorf("SeniorityFromDescription(%q) = %q, want %q", tt.desc, got, tt.want)
			}
		})
	}
}
