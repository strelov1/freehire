package mailclassify

import "testing"

func TestIncompleteApplicationInVocabulary(t *testing.T) {
	c := Classification{Signal: SignalIncompleteApplication, Confidence: 0.9}.Sanitize()
	if c.Signal != SignalIncompleteApplication {
		t.Errorf("Sanitize coerced %q away; want it kept in the vocabulary", SignalIncompleteApplication)
	}
}

func TestIncompleteApplicationDoesNotAdvanceStage(t *testing.T) {
	// An unfinished application has not progressed — it must never move a stage.
	if stage, ok := AdvanceStage("applied", SignalIncompleteApplication); ok {
		t.Errorf("incomplete_application advanced stage to %q; want no advance", stage)
	}
	if stage, ok := AdvanceStage("", SignalIncompleteApplication); ok {
		t.Errorf("incomplete_application seeded stage %q from empty; want no advance", stage)
	}
}

func TestKeywordStatusIncompleteApplication(t *testing.T) {
	cases := []struct {
		name    string
		subject string
		body    string
	}{
		{"incomplete phrasing", "Action needed", "Your application is incomplete. Please complete your application to be considered."},
		{"finish phrasing wins over ack opener", "Thank you for starting", "Thank you for starting your application. You must finish your application within 7 days."},
	}
	for _, c := range cases {
		got, ok := KeywordStatus(c.subject, c.body)
		if !ok || got != SignalIncompleteApplication {
			t.Errorf("%s: KeywordStatus() = (%q, %v), want (incomplete_application, true)", c.name, got, ok)
		}
	}
}
