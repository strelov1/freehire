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

		// Negated mentions — the phrase is present, but the sentence denies it.
		{"does not require citizenship", "This role does not require US citizenship; applicants worldwide are welcome.", false},
		{"no clearance required", "No Secret clearance is required for this position.", false},
		{"non-us citizens welcome", "We welcome non-US citizens to apply for this fully remote role.", false},
		{"cannot sponsor but no citizenship needed", "We cannot sponsor visas, and US citizenship is not required.", false},

		// A denial elsewhere must not hide a genuine assertion in a later sentence.
		{"negation in an earlier unrelated sentence", "This is not a contractor role. US citizenship is required.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := USOnlyFromDescription(tt.desc); got != tt.want {
				t.Errorf("USOnlyFromDescription(%q) = %v, want %v", tt.desc, got, tt.want)
			}
		})
	}
}
