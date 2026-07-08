package location

import "testing"

func TestUSOnlyFromDescription(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want bool
	}{
		// Positives — hard, US-specific eligibility statements.
		{"citizen and clearance", "Must be a U.S. Citizen and eligible for a U.S. SECRET clearance.", true},
		{"us citizen", "This role requires a US Citizen.", true},
		{"united states citizen", "Applicants must be United States citizens.", true},
		{"us citizenship", "US citizenship is required for this position.", true},
		{"us citizenship dotted", "U.S. citizenship required.", true},
		{"secret clearance", "Candidates must hold an active Secret clearance.", true},
		{"top secret via substring", "An active Top Secret clearance is mandatory.", true},
		{"ts sci", "Requires a current TS/SCI with polygraph.", true},

		// Trap negatives — incidental tokens that must NOT trigger a match.
		{"join us", "Join us! We are hiring engineers worldwide.", false},
		{"corporate citizen", "We strive to be a good corporate citizen.", false},
		{"global citizen", "We welcome every global citizen to apply.", false},
		{"trade secret", "You will help protect our trade secrets.", false},
		{"security engineer", "We are hiring an Application Security Engineer.", false},
		{"generic security clearance", "A UK SC security clearance is a plus.", false},
		{"worldwide", "Open to candidates anywhere in the world.", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := USOnlyFromDescription(tt.desc); got != tt.want {
				t.Errorf("USOnlyFromDescription(%q) = %v, want %v", tt.desc, got, tt.want)
			}
		})
	}
}
