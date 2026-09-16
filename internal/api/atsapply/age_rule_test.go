package atsapply

import "testing"

// "Are you at least 18 years of age?" is ~150 of the 12 352 question labels measured across
// 4 000 live forms (2026-09-16) — and the candidate answered it long ago, in
// screening_answers.age_18_or_older. Forms give the question an opaque id, so answerKeyFor
// can never reach it and only a label rule can; without one, a fact already on file parks
// the application. The same gap the salary rule below it was written for.
func TestLabelRule_AsksIfYouAreOfAge(t *testing.T) {
	for _, label := range []string{
		"Are you at least 18 years of age?",
		"Are you over the age of 18?",
		"Are you 18 years of age or older?",
		"Are you 18 or older?",
		"I confirm I am 18 or older",
	} {
		key, ok := matchLabelAnswerKey(label)
		if !ok {
			t.Errorf("matchLabelAnswerKey(%q) found nothing", label)
			continue
		}
		if key != "age_18_or_older" {
			t.Errorf("matchLabelAnswerKey(%q) = %q, want age_18_or_older", label, key)
		}
	}
}

// An age RANGE is a demographic question, asked voluntarily and answered by the candidate
// alone. Answering it from a yes/no fact would both be wrong and put a protected
// characteristic on a form nobody authorised.
func TestLabelRule_LeavesDemographicAgeAlone(t *testing.T) {
	for _, label := range []string{
		"What is your age range?",
		"How would you describe your racial/ethnic background?",
		"Which age bracket do you fall into?",
	} {
		if key, ok := matchLabelAnswerKey(label); ok {
			t.Errorf("matchLabelAnswerKey(%q) = %q, want no match — that is the candidate's own to answer", label, key)
		}
	}
}
