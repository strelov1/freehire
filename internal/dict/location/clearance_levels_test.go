package location

import (
	"slices"
	"testing"
)

// Every case below is a phrase from a live posting in the 2026-10-31 sample (lightly
// trimmed), unless its name says it is a constructed near-miss.
func TestClearanceLevelsFromDescription(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want []string
	}{
		// The US ladder in prose.
		{"active secret", "Candidates must have an active Secret clearance and verify, before applying.", []string{"secret"}},
		{"obtain secret", "This position requires the ability to obtain and maintain a Secret U.S. security clearance.", []string{"secret"}},
		{"issued secret", "Applicants must have a current, active in-scope DoD-issued <b>Secret</b> security clearance at the time of application.", []string{"secret"}},
		{"secret level", "Must be able to obtain and maintain a secret-level security clearance.", []string{"secret"}},
		{"quoted secret", "The candidate must be eligible for a “Secret” security clearance.", []string{"secret"}},
		{"top secret", "Must be a US citizen and hold a current Top Secret clearance.", []string{"top_secret"}},
		{"ts/sci is not also top secret", "Active TS/SCI clearance; polygraph preferred.", []string{"ts_sci"}},
		{"top secret/sci is not also secret", "US citizens with DoD Top Secret/SCI only.", []string{"ts_sci"}},
		{"top secret with sci eligibility", "Active U.S. Government Top Secret security clearance with SCI eligibility.", []string{"ts_sci"}},
		{"ts/sci without polygraph is still ts/sci", "Security Clearance Type: TS/SCI without Polygraph", []string{"ts_sci"}},
		{"a ladder of two", "Active Secret clearance is required to start, with the ability to obtain a TS/SCI.", []string{"secret", "ts_sci"}},
		{"public trust clearance", "Must be able to obtain a DHS Public Trust clearance prior to starting.", []string{"public_trust"}},
		{"obtain a public trust", "Security clearance required: this position requires the candidate to be able to obtain a Public Trust.", []string{"public_trust"}},
		{"confidential clearance", "You must be able to obtain and maintain a Confidential security clearance.", []string{"confidential"}},

		// Interim rides beside a rung.
		{"interim secret", "An interim Secret clearance is acceptable until the full clearance is granted. Active DoD Secret clearance or higher.", []string{"secret", "interim"}},
		{"interim and/or final", "An interim and/or final U.S. Secret clearance post-start is required.", []string{"secret", "interim"}},
		{"interim refused", "Active Secret clearance at minimum. Interim Secret clearances are not allowed.", []string{"secret"}},
		{"interim alone names no rung", "Hire dates depend on the issuance of an interim or final government security clearance.", []string{"interim", "other"}},

		// Clearance FIELDS, where the label is the anchor.
		{"field secret", "<p><b>Clearance Type:</b></p><p>Secret</p><p><b>Travel:</b> Yes</p>", []string{"secret"}},
		{"nested field", "<p><b>Security Clearance Type: </b></p>DoD Clearance: Confidential\n\n<p><b>Security Clearance Status: </b></p>Active and existing security clearance required after day 1",
			[]string{"confidential"}},
		{"field ts/sci", "Clearance Required: TS/SCI with Poly", []string{"ts_sci"}},
		{"field public trust", "Security Clearance Level: Public Trust (MBI)", []string{"public_trust"}},
		{"field without a colon", "<p>Security clearance required.</p><p>Clearance Level</p><p>Secret</p><p>The salary range for this role is $74,500.</p>", []string{"secret"}},

		// A field that REFUSES a rung must not assert it: a Leidos template carries this
		// line on every posting beside a real field.
		{"denied public trust field", "Clearance Level Must Currently Possess: Secret\nClearance Level Must Be Able to Obtain: Secret\nPublic Trust/Other Required: None",
			[]string{"secret"}},
		{"affirmed public trust field", "Clearance Level Must Currently Possess: None\nPublic Trust/Other Required: MBI (T2)\nThis role requires a security clearance.",
			[]string{"public_trust"}},

		// Non-US schemes and level-less requirements are other.
		{"uk sc", "SC security clearance is a must.", []string{"other"}},
		{"au nv1", "Must hold a current NV1 clearance.", []string{"other"}},
		{"au field", "Clearance Type: AU- Secret (NV1)", []string{"other"}},
		{"nato secret", "Required security clearance: NATO Secret.", []string{"other"}},
		{"no level named", "An active and transferable U.S. government issued security clearance is required prior to start date.", []string{"other"}},

		// Near-misses: the words without the requirement, or the requirement without
		// the rung.
		{"trade secret, no clearance", "Protect our trade secrets.", nil},
		{"secret idiom beside a clearance", "Security clearance required. We'll let you in on a secret: there's no perfect candidate.", []string{"other"}},
		{"secret management beside a clearance", "Security clearance required. Integrate secret management into pipelines.", []string{"other"}},
		{"kept confidential beside a clearance", "Security clearance required. All applications will be kept confidential.", []string{"other"}},
		{"public trust as copy", "Security clearance required. Officers perform duties in a manner that furthers public trust.", []string{"other"}},
		{"denied rung", "Department of Defense Secret security clearance: though not required of this role, holders are preferred. An active security clearance helps.", []string{"other"}},
		{"silent", "We are hiring a Go engineer.", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClearanceLevelsFromDescription(tt.desc)
			if !slices.Equal(got, tt.want) {
				t.Errorf("ClearanceLevelsFromDescription = %v, want %v", got, tt.want)
			}
			if (len(got) > 0) != RequiresClearanceFromDescription(tt.desc) {
				t.Errorf("levels %v disagree with RequiresClearanceFromDescription", got)
			}
		})
	}
}
