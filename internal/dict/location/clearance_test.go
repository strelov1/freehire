package location

import "testing"

func TestRequiresClearanceFromDescription(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want bool
	}{
		// UK schemes. The vocabulary is short-token heavy, so every entry is
		// anchored by a following noun rather than standing alone.
		{"uk sc clearance", "You must hold or be eligible for SC clearance.", true},
		{"uk sc cleared", "We are looking for SC cleared engineers.", true},
		{"uk dv clearance", "Candidates must hold transferable UK DV clearance.", true},
		{"uk security vetting", "You must be eligible to pass National Security Vetting.", true},
		{"uk bpss", "BPSS is required before your start date.", true},

		// US schemes.
		{"us security clearance", "An active US security clearance is required.", true},
		{"us secret clearance", "Applicants must hold a Secret clearance.", true},
		{"us ts/sci", "Clearance: Active TS/SCI with CI Polygraph.", true},
		{"us polygraph", "This role requires a full scope polygraph.", true},
		{"us public trust clearance", "A Public Trust clearance is required.", true},

		// AU schemes.
		{"au nv1", "Must hold a current NV1 clearance.", true},
		{"au baseline", "Applicants require a Baseline clearance.", true},
		{"au positive vetting", "Candidates will hold active positive vetting clearance (PV).", true},

		// Variants found only by sampling live descriptions. Each was missed by an
		// entry that looks like it should have covered it: "edv" is not "dv" under a
		// whole-word match, and "top secret/sci" is neither "ts/sci" nor "top secret
		// clearance".
		{"uk enhanced dv", "The role requires active eDV clearance (West).", true},
		{"us top secret slash sci", "Onsite. US citizens with DoD Top Secret/SCI only.", true},

		// Scheme-neutral.
		{"active clearance", "An active clearance is required for this role.", true},

		// Bare short tokens must never fire on their own: they collide with
		// ordinary words and unrelated initialisms.
		{"bare sc is not a clearance", "We are an SC-registered charity based in Bristol.", false},
		{"bare dv is not a clearance", "The DV team owns deployment and verification.", false},
		{"bare nv is not a clearance", "Our NV office covers Nevada accounts.", false},

		// Unrelated senses of the word.
		{"customs clearance", "Handle inbound and outbound customs clearance paperwork.", false},
		{"medical clearance", "Medical clearance is required before your start date.", false},
		{"clearance sale", "Plan the seasonal clearance sale campaign.", false},

		// A bare mention of the word alone is never enough.
		{"bare clearance token", "Clearance specialist for a central billing team.", false},

		// Silence is unknown, not a negative.
		{"silent description", "We are hiring a backend engineer to work on our Go services.", false},
		{"empty description", "", false},

		// Being able to OBTAIN a clearance counts: eligibility for one turns on
		// nationality and residency, so a candidate who cannot hold one cannot
		// obtain one either.
		{"obtainable clearance", "Must be able to obtain and maintain a US Secret clearance.", true},
		{"eligible for clearance", "You must be eligible for SC clearance.", true},
		// ...including when no scheme is named at all. Sampled live: "will need to
		// be able to obtain a clearance" carried no other anchor and was missed.
		{"obtain an unnamed clearance", "You will need to be able to obtain a clearance for this contract.", true},

		// Negation cancels the sentence it sits in.
		{"denied requirement", "No security clearance is required for this role.", false},
		{"denied requirement, other phrasing", "This position does not require a security clearance.", false},

		// ...but only that sentence. An anchor asserted elsewhere still marks it.
		{
			"denial does not suppress another sentence",
			"No security clearance is required. You must hold an active TS/SCI clearance.",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresClearanceFromDescription(tt.desc); got != tt.want {
				t.Errorf("RequiresClearanceFromDescription(%q) = %v, want %v", tt.desc, got, tt.want)
			}
		})
	}
}

// TestRequiresClearanceLabelledField covers the form ATS postings use to state the
// requirement as a structured field rather than prose. A phrase list alone misses
// it: in the sampled catalogue rows it was roughly a fifth of all true positives.
func TestRequiresClearanceLabelledField(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want bool
	}{
		{"clearance names a scheme", "Clearance: Secret\nWork hours: M-F", true},
		{"clearance names a compound scheme", "Clearance: TS with SCI eligibility. Active.", true},
		{"clearance level label", "Clearance Level: Public Trust (Secret eligibility)", true},
		{"clearance type label", "CLEARANCE TYPE: Polygraph\nTRAVEL: 10%", true},
		{"clearance required yes", "CLEARANCE REQUIRED FOR START: Yes", true},
		{"ability to obtain", "Clearance Required: Ability to Obtain Public Trust", true},

		// A label that denies the requirement must not mark the posting.
		{"clearance required no", "Clearance Required: No\nTravel: minimal", false},
		{"clearance none", "Clearance: None", false},
		{"clearance n/a", "Clearance: N/A", false},

		// A label whose value says nothing recognisable leaves it unmarked, which
		// is the safe default.
		{"unrecognised value", "Clearance: see the attached document", false},

		// The short scheme tokens must match as WORDS. As bare substrings they hide
		// inside ordinary English — "sc" in "describe" and "discuss", "ts" in
		// "contracts", "dv" in "advise" — and every one of those would assert a
		// clearance requirement out of a value that denies knowing one.
		{"sc hides inside describe", "Clearance: describe your situation at interview", false},
		{"sc hides inside discuss", "Clearance: discuss with your recruiter", false},
		{"ts hides inside contracts", "Clearance: depends on the contracts involved", false},
		{"dv hides inside advise", "Clearance: we will advise on this later", false},

		// Descriptions are stored as HTML, and tags routinely sit between a label
		// and its value. Reading the markup rather than the visible text turned
		// this exact row — a real posting — into a false positive: the denial
		// "None/Not Required" was unreachable behind </b></p>.
		{
			"html between label and denial",
			"<p><b>Security Clearance: </b></p>None/Not Required<p></p>",
			false,
		},
		{
			"html between label and value",
			"<p><b>Clearance Level Must Currently Possess:</b></p><p>Top Secret/SCI</p>",
			true,
		},
		{
			"label split by markup",
			"<p>Minimum Clearance Required to Start: TS/SCI with Polygraph</p>",
			true,
		},

		// A denial written with a slash must be read as a denial. Splitting the
		// value on whitespace alone leaves the token "none/not", which matches no
		// denial and lets the label assert itself.
		{"slash-joined denial", "Clearance: None/Not Required", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresClearanceFromDescription(tt.desc); got != tt.want {
				t.Errorf("RequiresClearanceFromDescription(%q) = %v, want %v", tt.desc, got, tt.want)
			}
		})
	}
}
