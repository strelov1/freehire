package location

import "testing"

func TestWorkModeFromDescription(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string
	}{
		// Positives — clear, anchored phrases.
		{"fully remote", "This is a fully remote position open to anyone.", "remote"},
		{"remote-first", "We are a remote-first company.", "remote"},
		{"100 percent remote", "The role is 100% remote.", "remote"},
		{"work from anywhere", "You can work from anywhere in the EU.", "remote"},
		{"remote position", "Remote position with occasional travel.", "remote"},
		{"hybrid role", "This is a hybrid role based in Berlin.", "hybrid"},
		{"hybrid working", "We offer hybrid working arrangements.", "hybrid"},
		{"days in the office", "You will spend 3 days in the office each week.", "hybrid"},
		{"on-site only", "This job is on-site only.", "onsite"},
		{"must be on-site", "Candidates must be on-site in our HQ.", "onsite"},
		{"in-office position", "An in-office position in Munich.", "onsite"},

		// Priority — hybrid beats remote when both appear.
		{"hybrid beats remote", "A hybrid role with some remote days.", "hybrid"},

		// Trap negatives — incidental tokens that must NOT trigger a match.
		{"distributed systems", "Experience building distributed systems at scale.", ""},
		{"hybrid cloud", "You will manage our hybrid cloud infrastructure.", ""},
		{"remote server", "Debug issues on a remote server over SSH.", ""},
		{"remote team", "Collaborate with a remote team across time zones.", ""},
		{"bare in office", "Free snacks in office and a great culture.", ""},
		{"incidental from our office", "Enjoy free lunch from our office cafeteria.", ""},
		{"no arrangement phrase", "We build payments infrastructure in Go.", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorkModeFromDescription(tt.desc); got != tt.want {
				t.Errorf("WorkModeFromDescription(%q) = %q, want %q", tt.desc, got, tt.want)
			}
		})
	}
}

// TestRemoteContradicted covers the denial dictionary. Every "does not fire" case below is
// a sentence taken from a real prod posting the catalogue serves as remote, not an invented
// one — see the change's design.md for the sample the list was measured against.
func TestRemoteContradicted(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want bool
	}{
		// The reported posting (freehire#2555): NVIDIA JR2020330 states both denials.
		{"nvidia on-site", "This position is 100% on-site based at either our Dallas or Houston " +
			"Contract Manufacturing (CM) facility.", true},
		{"nvidia no arrangement", "Candidates must be able to work full-time on-site at one of " +
			"these locations (this role does not offer remote or hybrid arrangements).", true},

		// The denial families, as employers actually write them.
		{"not a remote position", "This is not a remote position. Essential functions:", true},
		{"not a remote role", "FTC & perm opportunities. This is not a remote role.", true},
		{"not a remote job", "This is not a remote job and candidate has to be in Chennai office daily.", true},
		{"dash form", "Executive Assistant to the General Manager (Mentor, OH) onsite – not a remote position", true},
		{"bare 100% onsite", "Lancaster, Ohio • Full-time • 100% onsite. Recommended salary range:", true},
		{"no remote work", "No remote work is possible for this role.", true},
		{"remote work not available", "Remote work is not available for this team.", true},

		// Markup must not decide the answer: an editor's <strong> inside the phrase, and an
		// &nbsp; between its words, are both real and both meaningless.
		{"phrase split by a tag", "This is not a <strong>REMOTE POSITION</strong>.", true},
		{"phrase split by an entity", "This is&nbsp;not a remote position.", true},

		// A denial scoped to a trial period or a follow-on role is not a denial about THIS
		// posting's arrangement. Both sentences are verbatim from prod.
		{"trial period", "Work location status: fully on-site for the first 90 days at the " +
			"Edwardsville, IL headquarters. After successful completion of training you may work remotely.", false},
		{"trial period, 100%", "The role is 100% on-site for the first 30-90 days at headquarters.", false},
		{"follow-on role", "Intern must be willing to live/work in the Washington, DC region. " +
			"This is not a remote job if hired afterward.", false},

		// Trap negatives — phrases deliberately left OUT of the dictionary because prose
		// qualifies them routinely, plus the incidental tokens the gap-filling list already guards.
		{"parking on-site only", "Free parking on-site only for badged employees.", false},
		{"must be onsite occasionally", "You must be onsite for quarterly planning weeks.", false},
		{"fully on-site alone", "The team is fully on-site in Berlin.", false},
		{"remote team", "Collaborate with a remote team across time zones.", false},
		{"hybrid cloud", "You will manage our hybrid cloud infrastructure.", false},
		{"genuinely remote", "This is a fully remote position open to anyone in the EU.", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoteContradicted(tt.desc); got != tt.want {
				t.Errorf("RemoteContradicted(%q) = %v, want %v", tt.desc, got, tt.want)
			}
		})
	}
}

// TestRemoteContradictedReadsPastAQualifiedDenial guards the scan itself rather than the
// dictionary: a posting that qualifies one denial and then states another unqualified one is
// still denying remote work, so stopping at the first qualified match would be wrong.
func TestRemoteContradictedReadsPastAQualifiedDenial(t *testing.T) {
	desc := "This is not a remote position if hired afterward. " +
		"To be clear: this is not a remote position."
	if !RemoteContradicted(desc) {
		t.Error("RemoteContradicted stopped at the qualified match instead of scanning on")
	}
}
