package answertopic

import "testing"

// The three phrasings a salary question actually arrives in. They must key alike, or the
// candidate answers the same question once per employer — the whole point of the bank.
func TestOf_SalaryPhrasingsShareATopic(t *testing.T) {
	first, ok := Of("What is your desired salary?")
	if !ok {
		t.Fatal("Of refused a well-formed question")
	}
	for _, phrasing := range []string{
		"Desired salary",
		"  desired   SALARY  ",
		"Please tell us your desired salary.",
	} {
		got, ok := Of(phrasing)
		if !ok {
			t.Fatalf("Of(%q) refused a well-formed question", phrasing)
		}
		if got != first {
			t.Errorf("Of(%q) = %q, want %q — the same question phrased differently", phrasing, got, first)
		}
	}
}

// Two genuinely different questions must NOT collapse: a bank that merges them answers one
// with the other, in the candidate's name, to an employer.
func TestOf_DifferentQuestionsKeepDifferentTopics(t *testing.T) {
	desired, _ := Of("What is your desired salary?")
	current, _ := Of("What is your current salary?")
	if desired == current {
		t.Errorf("desired and current salary both key to %q — answering one with the other misreports the candidate", desired)
	}
}

// The dictionary is an accelerator, not the only route: a question it has never heard of
// still gets a stable key from the fold alone.
func TestOf_AnUnknownQuestionStillKeysStably(t *testing.T) {
	first, ok := Of("Which state do you currently reside in?")
	if !ok {
		t.Fatal("Of refused a well-formed question the dictionary does not know")
	}
	again, _ := Of("which state do you currently reside in")
	if first != again {
		t.Errorf("Of is not stable for an unknown question: %q vs %q", first, again)
	}
}

// Greenhouse's hidden proxy inputs reach this with an empty label (see
// internal/api/atsapply/domscan.go). A key derived from nothing can never be recalled, so
// it must be refused rather than stored.
func TestOf_RefusesAQuestionThatFoldsToNothing(t *testing.T) {
	for _, empty := range []string{"", "   ", "???", "-- --"} {
		if got, ok := Of(empty); ok {
			t.Errorf("Of(%q) = %q, true; want a refusal — nothing can be recalled by this key", empty, got)
		}
	}
}
