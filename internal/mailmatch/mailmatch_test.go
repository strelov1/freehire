package mailmatch

import "testing"

func TestExtractCompany(t *testing.T) {
	cases := []struct {
		name     string
		fromName string
		subject  string
		want     string
	}{
		{"from_name hiring-team suffix", "Block Labs Hiring Team", "Block Labs Application Update", "block labs"},
		{"from_name workday suffix", "Motorola Solutions - Workday", "Thank you in your interest in Principal Engineer", "motorola solutions"},
		{"from_name llc suffix", "Very LLC", "Ilya, we've received your resume", "very"},
		{"from_name legal suffix behind trailing period", "Acme Inc.", "", "acme"},
		{"from_name trailing comma after team suffix", "Sardine Hiring Team,", "", "sardine"},
		{"subject thank-you-for-applying prefix", "", "Thank you for applying to Hyperproof", "hyperproof"},
		{"subject your-application-to prefix", "", "Your Application to Nametag", "nametag"},
		{"subject trailing emoji stripped", "Sardine Hiring Team", "Thank you for applying to Sardine! 🐟", "sardine"},
		{"ats pseudo-name from subject dropped", "", "Thank you for applying to Greenhouse!", ""},
		{"ats pseudo-name your-x-application dropped", "", "Your Greenhouse Application", ""},
		{"nothing extractable", "", "Ilya, we've received your resume", ""},
		// A relay whose display name is the ATS brand must not shadow the employer
		// the subject names. Observed on a live mailbox: 23 acknowledgements from
		// "Workable", each about a different employer, all resolved to the one
		// catalog company literally called Workable — so one application collected
		// everyone else's mail and could never look silent again.
		{"ats relay display name falls through to the subject",
			"Workable", "Thanks for applying to Derq", "derq"},
		{"ats relay display name alone extracts nothing", "Workable", "", ""},
		{"employer named in both still wins",
			"Sardine Hiring Team", "Thanks for applying to Sardine", "sardine"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractCompany(c.fromName, c.subject)
			if got != c.want {
				t.Fatalf("ExtractCompany(%q, %q) = %q, want %q", c.fromName, c.subject, got, c.want)
			}
		})
	}
}

// TestExtractCompany_UsesTheSharedFormList proves mail matching strips the same corporate
// forms the catalogue keys companies by. It carried its own six-token list, so a sender
// writing "Acme Limited" or "Acme GmbH & Co. KG" yielded a name that matched no company —
// and a mail that matches no company links to no application, silently.
func TestExtractCompany_UsesTheSharedFormList(t *testing.T) {
	for from, want := range map[string]string{
		"Acme Limited":       "acme",
		"Acme Robotics PLC":  "acme robotics",
		"Adyen N.V.":         "adyen",
		"Acme Recruiting AS": "acme recruiting",
		// A compound form has to come off whole, stepping over the punctuation between its
		// parts, or the sender never matches the plain company name it is written under.
		"Acme GmbH & Co. KG":    "acme",
		"Sun Technologies,Inc.": "sun technologies",
	} {
		if got := ExtractCompany(from, ""); got != want {
			t.Errorf("ExtractCompany(%q) = %q, want %q", from, got, want)
		}
	}
}
