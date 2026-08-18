package skilltag

import (
	"slices"
	"testing"
)

func TestParse_AIEnablement(t *testing.T) {
	// The adoption side of AI: driving usage inside an organisation rather than
	// building models. It is a skill, not a role — these postings are Program
	// Managers, Architects and Change Managers whose SUBJECT is AI adoption, so
	// tagging it lets the role and the subject filter together instead of one
	// displacing the other.
	for _, desc := range []string{
		"You will lead AI enablement across the business units.",
		"Own our AI adoption programme and measure whether it landed.",
		"Drive the AI transformation roadmap with the exec team.",
	} {
		got := Parse(desc)
		if !slices.Contains(got, "ai-enablement") {
			t.Errorf("Parse(%q) = %v, want it to contain ai-enablement", desc, got)
		}
	}
}

func TestParse_AIEnablementLeavesModelWorkAlone(t *testing.T) {
	// Training a model is the opposite job from teaching people to use one, so
	// the annotation vocabulary must not collapse into this tag.
	for _, desc := range []string{
		"Label training data and review model output as an AI trainer.",
		"Data annotation specialist for our vision models.",
	} {
		if got := Parse(desc); slices.Contains(got, "ai-enablement") {
			t.Errorf("Parse(%q) = %v, want no ai-enablement", desc, got)
		}
	}
}
