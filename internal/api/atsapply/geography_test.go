package atsapply

import "testing"

// The live Garner Health Greenhouse posting's own wording, the case this rule exists for.
func TestIsGeographyLabel_CatchesTheLiveGarnerHealthWording(t *testing.T) {
	if !isGeographyLabel("Current State of Residence") {
		t.Error("want the live posting's residency question recognized")
	}
}

func TestIsGeographyLabel_CatchesEveryTerm(t *testing.T) {
	labels := []string{
		"What is your state of residence?",
		"What is your country of residence?",
		"What is your residency status?",
		"Where do you currently reside?",
		"Where are you currently located?",
		"Where are you based?",
		"This role must be based in the United States.",
		"You must reside in a state where we are registered to hire.",
		"This position must be located in an eligible state.",
	}
	for _, label := range labels {
		if !isGeographyLabel(label) {
			t.Errorf("isGeographyLabel(%q) = false, want true", label)
		}
	}
}

func TestIsGeographyLabel_LeavesOrdinaryQuestionsAlone(t *testing.T) {
	labels := []string{
		"What is your LinkedIn profile?",
		"Why do you want to work here?",
		"Do you have advanced proficiency in German?",
		"What is your phone number?",
	}
	for _, label := range labels {
		if isGeographyLabel(label) {
			t.Errorf("isGeographyLabel(%q) = true, want false", label)
		}
	}
}

func TestIsGeographyLabel_LeavesWorkAuthorizationToTheSensitiveGate(t *testing.T) {
	// "Do you have work authorization..." is already parked by isSensitiveLabel's
	// "authoriz" term; geography.go exists for the residency shape that gate misses, not
	// as a second path to the same outcome.
	if isGeographyLabel("Do you have legal authorization to work in this country?") {
		t.Error("want work-authorization wording left to isSensitiveLabel, not duplicated here")
	}
}

func TestIsGeographyLabel_IsCaseInsensitive(t *testing.T) {
	if !isGeographyLabel("CURRENT STATE OF RESIDENCE") {
		t.Error("want the geography check to ignore case")
	}
}
