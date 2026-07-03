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

func TestNonTechFromDescription(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string
	}{
		// Positives — anchored non-technical role statements.
		{"sales representative", "We are hiring a sales representative for the DACH region.", "sales"},
		{"account executive", "This is an account executive role selling to enterprise.", "sales"},
		{"business development rep", "Join us as a business development representative.", "sales"},
		{"marketing manager", "We are looking for a marketing manager.", "marketing"},
		{"content marketing", "A content marketing specialist opening.", "marketing"},
		{"social media manager", "Hiring a social media manager for our brand.", "marketing"},
		{"seo specialist", "We need an SEO specialist to grow organic traffic.", "marketing"},
		{"customer support", "We are hiring a customer support representative.", "support"},
		{"customer success", "This customer success manager owns renewals.", "support"},
		{"help desk", "We are hiring a help desk technician.", "support"},
		{"office manager", "We are hiring an office manager for our Berlin HQ.", "management"},
		{"operations manager", "This is an operations manager position.", "management"},
		{"general manager", "Seeking a general manager for the new market.", "management"},
		{"hr manager", "Hiring an HR manager to build the people team.", "management"},

		// Trap negatives — incidental prose that must NOT set a category.
		{"sales team incidental", "Collaborate with our sales team on integrations.", ""},
		{"support engineers incidental", "Work closely with our support engineers.", ""},
		{"marketing org incidental", "Join our marketing org as a backend engineer.", ""},
		{"bare support", "We offer great support and a strong culture.", ""},

		// Tech-adjacent roles are explicitly excluded (never mislabel a tech job).
		{"sales engineer excluded", "We are hiring a sales engineer.", ""},
		{"solutions engineer excluded", "This is a solutions engineer role.", ""},
		{"engineering manager excluded", "We are hiring an engineering manager.", ""},
		{"product manager excluded", "Join us as a product manager.", ""},
		{"project manager excluded", "Seeking a project manager for the platform.", ""},
		{"data engineering manager excluded", "We are hiring a data engineering manager.", ""},

		// Topic mentions (a skill/outcome/tool, not a role) must NOT set a category —
		// they appear in the prose of unrelated roles.
		{"digital marketing skill mention", "Familiarity with digital marketing tools is a plus.", ""},
		{"customer success outcome", "You will drive customer success across our platform.", ""},
		{"help desk software mention", "Integrate with popular help desk software like Zendesk.", ""},

		// The detector resolves no technical category, and empty is empty.
		{"tech role stays empty", "Build resilient backend systems in Go.", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NonTechFromDescription(tt.desc); got != tt.want {
				t.Errorf("NonTechFromDescription(%q) = %q, want %q", tt.desc, got, tt.want)
			}
		})
	}
}
