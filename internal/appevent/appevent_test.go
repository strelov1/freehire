package appevent

import "testing"

func TestValidKindAcceptsTheVocabularyAndNothingElse(t *testing.T) {
	for _, k := range Kinds {
		if !ValidKind(k) {
			t.Errorf("ValidKind(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"", "reply", "APPLIED", "employer-reply"} {
		if ValidKind(k) {
			t.Errorf("ValidKind(%q) = true, want false", k)
		}
	}
}

func TestValidSourceAcceptsTheVocabularyAndNothingElse(t *testing.T) {
	for _, s := range Sources {
		if !ValidSource(s) {
			t.Errorf("ValidSource(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "gmail", "mail", "gmail_mail"} {
		if ValidSource(s) {
			t.Errorf("ValidSource(%q) = true, want false", s)
		}
	}
}

// A manually-recorded event dates from when the candidate got around to updating their
// board, not from when the thing happened. Letting one into day arithmetic would measure
// diligence and report it as market behaviour, so the split is asserted in both
// directions: a future edit cannot quietly promote a manual source.
//
// The line is "did somebody other than the candidate set this date", not "did it come
// from mail". A meeting read out of the candidate's calendar was arranged by an
// organiser and observed by us, so it belongs on the trusted side beside the mail.
func TestOnlyObservedSourcesAreTrustedForDayMath(t *testing.T) {
	trusted := map[string]bool{
		SourceMailGmail:      true,
		SourceMailHosted:     true,
		SourceMailExternal:   true,
		SourceCalendarGoogle: true,
		SourceUser:           false,
		SourceAssistant:      false,
		SourceSystem:         false,
	}
	if len(trusted) != len(Sources) {
		t.Fatalf("the vocabulary has %d sources but this test pins %d — a new source needs a verdict here", len(Sources), len(trusted))
	}
	for src, want := range trusted {
		if got := TrustedForDayMath(src); got != want {
			t.Errorf("TrustedForDayMath(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestAnUnknownSourceIsNotTrusted(t *testing.T) {
	if TrustedForDayMath("mail_carrier_pigeon") {
		t.Error("an unrecognised source was trusted for day math; unknown provenance must not read as observed")
	}
}

func TestSourceForMailMapsTheInboxVocabulary(t *testing.T) {
	cases := map[string]string{
		"gmail":    SourceMailGmail,
		"hosted":   SourceMailHosted,
		"external": SourceMailExternal,
	}
	for in, want := range cases {
		got, err := SourceForMail(in)
		if err != nil {
			t.Errorf("SourceForMail(%q) returned error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("SourceForMail(%q) = %q, want %q", in, got, want)
		}
	}
}

// Defaulting an unrecognised mail source to a mail-derived event would stamp unknown
// provenance as observed and admit it to timings. The caller must handle the error.
func TestSourceForMailRefusesAnUnknownStore(t *testing.T) {
	if _, err := SourceForMail("imap"); err == nil {
		t.Error("SourceForMail(\"imap\") returned no error; an unknown store must not default to a trusted source")
	}
}
