package roletag

import (
	"slices"
	"testing"
)

func TestDerive_AIEnablement(t *testing.T) {
	// The adoption cluster: internal roles that get an organisation USING AI —
	// training, governance, licence ownership, change management. Distinct from
	// the ai_engineering family already in this table, which BUILDS AI.
	cases := []struct {
		title string
		want  string
	}{
		{"AI Enablement Lead", "ai_enablement"},
		{"Head of AI Adoption", "ai_enablement"},
		{"AI Transformation Manager", "ai_enablement"},
		{"AI Skills Coach", "ai_enablement"},
		// The bare "ai enablement" alias covers its own longer phrasings, so
		// they need no entry of their own.
		{"AI Enablement Coach", "ai_enablement"},
		{"AI Enablement Specialist", "ai_enablement"},
	}

	for _, c := range cases {
		got := Derive("", "", c.title)
		if !slices.Contains(got, c.want) {
			t.Errorf("Derive(%q) = %v, want it to contain %q", c.title, got, c.want)
		}
	}
}

func TestDerive_AIEnablementGradesLikeAnyNamedRole(t *testing.T) {
	// "Lead" is a real rung on these titles, so the role composes with a grade
	// rather than sitting in nonGradeable.
	got := Derive("lead", "management", "AI Enablement Lead")

	if !slices.Contains(got, "lead_ai_enablement") {
		t.Errorf("Derive = %v, want the graded composite lead_ai_enablement", got)
	}
}

func TestDerive_AIEnablementExcludesModelSideTraining(t *testing.T) {
	// "AI trainer" names two opposite jobs: labelling data so a MODEL learns, and
	// teaching PEOPLE to use AI. Only the second is enablement, so the word is
	// deliberately not an alias — folding annotation work in here would make the
	// role useless to both searches.
	for _, title := range []string{"AI Trainer", "AI Model Trainer", "Data Annotation Specialist"} {
		if got := Derive("", "", title); slices.Contains(got, "ai_enablement") {
			t.Errorf("Derive(%q) = %v, want no ai_enablement", title, got)
		}
	}
}

func TestCatalog_HasAIEnablement(t *testing.T) {
	// The catalog is the source of truth for the picker labels emitted into the
	// web contracts; an unlisted role is unpickable.
	if label := Catalog()["ai_enablement"]; label != "AI Enablement" {
		t.Errorf("Catalog()[ai_enablement] = %q, want %q", label, "AI Enablement")
	}
}
