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

// A key derived from nothing can never be recalled, so it must be refused rather than
// stored. This is a label of pure punctuation, which real forms do produce.
//
// It is NOT the labelless-field case: internal/api/atsapply keys a DOM-only field (
// Greenhouse's `country`) by its id through questionText, so such a field reaches Of as
// "country" and keys perfectly well.
func TestOf_RefusesAQuestionThatFoldsToNothing(t *testing.T) {
	for _, empty := range []string{"", "   ", "???", "-- --"} {
		if got, ok := Of(empty); ok {
			t.Errorf("Of(%q) = %q, true; want a refusal — nothing can be recalled by this key", empty, got)
		}
	}
}

// A polite wrapper containing an apostrophe must strip, not become dead code. The fold
// converts apostrophes to spaces, so the wrapper must too.
func TestOf_PoliteWrapperWithApostropheStrips(t *testing.T) {
	withWrapper, ok := Of("We'd like to know your state of residence")
	if !ok {
		t.Fatal("Of refused a well-formed question with apostrophe wrapper")
	}
	withoutWrapper, _ := Of("your state of residence")
	if withWrapper != withoutWrapper {
		t.Errorf("Of(\"We'd like to know your state of residence\") = %q, want %q — the wrapper should strip even when it contains an apostrophe", withWrapper, withoutWrapper)
	}
}

// This catalogue aggregates Russian- and Hungarian-language sources, so a screening question
// arrives in one of those languages. An ASCII-only fold drops every rune of it, and the
// question is refused with "this question cannot be saved — it has no readable text", which
// is not true of a perfectly readable question.
func TestOf_KeysANonLatinQuestion(t *testing.T) {
	for _, question := range []string{
		"Какой у вас желаемый доход?",
		"Mennyi a fizetési igénye?",
		"Wie hoch ist Ihre Gehaltsvorstellung?",
	} {
		got, ok := Of(question)
		if !ok {
			t.Errorf("Of(%q) refused a readable question", question)
			continue
		}
		if got == "" {
			t.Errorf("Of(%q) = %q — an empty key can never be recalled", question, got)
		}
	}
}

// The severe case, not merely the refused one. With an ASCII-only fold "Зарплата (USD)"
// keeps only the parenthesised currency and folds to "usd" — so every other question
// mentioning USD collapses onto that one topic, and the candidate's salary figure is sent
// as the answer to something else entirely. This is what
// TestOf_DifferentQuestionsKeepDifferentTopics exists to prevent, in the language the
// dictionary happens not to be written in.
func TestOf_NonLatinQuestionsDoNotCollapseOntoTheirLatinFragments(t *testing.T) {
	salary, ok := Of("Зарплата (USD)")
	if !ok {
		t.Fatal("Of refused a readable question")
	}
	budget, ok := Of("Бюджет проекта (USD)")
	if !ok {
		t.Fatal("Of refused a readable question")
	}
	if salary == budget {
		t.Errorf("two different questions both key to %q — answering one with the other misreports the candidate", salary)
	}
	if salary == "usd" || budget == "usd" {
		t.Errorf("a question folded to its currency alone: %q / %q", salary, budget)
	}
}

// Stable for a non-Latin question too: the same question in two casings keys alike.
func TestOf_IsStableForANonLatinQuestion(t *testing.T) {
	first, _ := Of("Какой у вас желаемый доход?")
	again, _ := Of("  КАКОЙ У ВАС ЖЕЛАЕМЫЙ ДОХОД  ")
	if first != again || first == "" {
		t.Errorf("Of is not stable for a non-Latin question: %q vs %q", first, again)
	}
}
