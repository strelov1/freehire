package gmailsync

import "testing"

func TestNormalizeSubject(t *testing.T) {
	cases := map[string]string{
		"Thank you for applying to Acme":     "thank you for applying to acme",
		"Re: Thank you for applying to Acme": "thank you for applying to acme",
		"RE: Re: Fwd:  Subject X":            "subject x",
		"  Subject   Y  ":                    "subject y",
		"FW: Interview":                      "interview",
		"Fwd: Fwd: Offer":                    "offer",
		"AW: Bewerbung":                      "bewerbung", // German "Re:"
		"":                                   "",
		"Re:":                                "",
	}
	for in, want := range cases {
		if got := NormalizeSubject(in); got != want {
			t.Errorf("NormalizeSubject(%q) = %q, want %q", in, got, want)
		}
	}
}
