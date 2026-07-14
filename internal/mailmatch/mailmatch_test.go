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
