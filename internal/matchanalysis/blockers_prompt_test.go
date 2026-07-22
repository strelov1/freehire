package matchanalysis

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/hardconstraint"
)

func TestStage1PromptCarriesUnmetBlockers(t *testing.T) {
	in := Input{
		JobTitle:       "Senior Engineer",
		JobDescription: "desc",
		Blockers: []hardconstraint.Blocker{
			{Category: hardconstraint.CategoryExperience, Reason: "Requires 5+ years; résumé shows 3.", Met: false},
			{Category: hardconstraint.CategoryEducation, Reason: "Requires a bachelor degree; résumé meets it.", Met: true},
		},
	}
	p := stage1UserPrompt(in, nil)
	if !strings.Contains(p, "Requires 5+ years; résumé shows 3.") {
		t.Error("stage-1 prompt should carry the unmet experience blocker")
	}
	if strings.Contains(p, "résumé meets it") {
		t.Error("stage-1 prompt should NOT list a met constraint")
	}
}

func TestStage1PromptOmitsBlockerSectionWhenAllMet(t *testing.T) {
	in := Input{JobTitle: "x", JobDescription: "y", Blockers: []hardconstraint.Blocker{
		{Category: hardconstraint.CategoryLanguage, Reason: "English present.", Met: true},
	}}
	if strings.Contains(stage1UserPrompt(in, nil), "Hard constraints the candidate does NOT meet") {
		t.Error("no blocker section when nothing is unmet")
	}
}
