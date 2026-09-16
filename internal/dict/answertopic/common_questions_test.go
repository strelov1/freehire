package answertopic

import "testing"

// Measured over 12 352 question labels from 4 000 live application forms (2026-09-16): 69%
// keyed to nothing a bank could recall. These are the two biggest clusters, and both are
// safe to answer from what the candidate has already told us — unlike the demographic
// questions just behind them, which are the candidate's own to answer.

// "How did you hear about us" is the single most common question on any form (~314 of the
// sample). Employers write it a dozen ways and drop their own NAME into it, so folding alone
// gives every company its own topic — an answer stored for one is never recalled for another.
func TestOf_HowDidYouHearIsOneTopicWhoeverAsks(t *testing.T) {
	asked := []string{
		"How did you hear about us?",
		"How did you hear about this job?",
		"How did you hear about this position?",
		"How did you hear about this opportunity?",
		"How did you hear about Appian?",
		"How did you hear about Anduril?",
		"How did you find out about this role?",
		"Where did you hear about us?",
	}
	first, ok := Of(asked[0])
	if !ok {
		t.Fatalf("Of(%q) could not be keyed at all", asked[0])
	}
	for _, q := range asked[1:] {
		got, ok := Of(q)
		if !ok {
			t.Errorf("Of(%q) could not be keyed", q)
			continue
		}
		if got != first {
			t.Errorf("Of(%q) = %q, want the same topic as %q (%q) — otherwise every employer's phrasing needs its own answer",
				q, got, asked[0], first)
		}
	}
}

// "Are you 18 or older" appears ~150 times in the sample, in three phrasings — and the
// candidate has ALREADY answered it: screening_answers.age_18_or_older. A question whose
// answer we hold, parking an application, is the worst kind of gap.
func TestOf_BeingOfAgeIsOneTopicHoweverPhrased(t *testing.T) {
	asked := []string{
		"Are you at least 18 years of age?",
		"Are you over the age of 18?",
		"Are you 18 years of age or older?",
		"Are you 18 or older?",
	}
	first, ok := Of(asked[0])
	if !ok {
		t.Fatalf("Of(%q) could not be keyed at all", asked[0])
	}
	for _, q := range asked[1:] {
		got, ok := Of(q)
		if !ok {
			t.Errorf("Of(%q) could not be keyed", q)
			continue
		}
		if got != first {
			t.Errorf("Of(%q) = %q, want %q", q, got, first)
		}
	}
}

// The dictionary unifies phrasings of ONE question; it must not pull separate questions
// together. An age RANGE is a demographic question the candidate answers themselves, and a
// company asking where you heard about it is not asking where you live.
func TestOf_KeepsGenuinelyDifferentQuestionsApart(t *testing.T) {
	hear, _ := Of("How did you hear about us?")
	age, _ := Of("Are you at least 18 years of age?")
	rangeQ, _ := Of("What is your age range?")
	reside, _ := Of("Do you currently reside in the United States?")

	for _, pair := range [][2]string{{hear, age}, {hear, reside}, {age, rangeQ}, {age, reside}} {
		if pair[0] == pair[1] {
			t.Errorf("two different questions share topic %q", pair[0])
		}
	}
}
