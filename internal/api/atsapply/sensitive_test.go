package atsapply

import "testing"

func TestIsSensitiveLabel_CatchesEveryPortedCategory(t *testing.T) {
	labels := []string{
		"What is your desired salary?",
		"What is your current compensation?",
		"Will you now or in the future require sponsorship?",
		"Do you require a visa to work in this country?",
		"Do you have work authorization for this role?",
		"Do you have the right to work in the UK?",
		"What gender do you identify as?",
		"What is your race?",
		"Please specify your ethnic background.",
		"Are you a protected veteran?",
		"Do you have a disability?",
		"This demographic data is voluntary.",
		"What is your sexual orientation?",
		"What is your religion?",
		"What is your national origin?",
		"What is your date of birth?",
		"Do you consent to genetic information disclosure?",
	}
	for _, label := range labels {
		if !isSensitiveLabel(label) {
			t.Errorf("isSensitiveLabel(%q) = false, want true", label)
		}
	}
}

// Found live (task 5.1's smoke check) against a real Greenhouse posting: "work authoriz"
// as a fixed-order phrase does not match this real, common phrasing at all — "authorization
// to work" has the words in the opposite order.
func TestIsSensitiveLabel_CatchesAuthorizationToWorkPhrasing(t *testing.T) {
	label := "Do you currently have legal authorization to work in the country in which the job you're applying for is located?"
	if !isSensitiveLabel(label) {
		t.Errorf("isSensitiveLabel(%q) = false, want true — a real live posting asked exactly this", label)
	}
}

func TestIsSensitiveLabel_LeavesOrdinaryQuestionsAlone(t *testing.T) {
	labels := []string{
		"Where did you first hear about this role?",
		"Do you have advanced proficiency in German?",
		"Why do you want to work here?",
		"What is your LinkedIn profile?",
		"Please describe a project you're proud of.",
	}
	for _, label := range labels {
		if isSensitiveLabel(label) {
			t.Errorf("isSensitiveLabel(%q) = true, want false", label)
		}
	}
}

func TestIsSensitiveLabel_IsCaseInsensitive(t *testing.T) {
	if !isSensitiveLabel("WHAT IS YOUR DESIRED SALARY?") {
		t.Error("want the sensitive check to ignore case")
	}
}
