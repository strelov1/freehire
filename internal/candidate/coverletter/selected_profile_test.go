package coverletter

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

func TestSelectedProfileRemovesUnselectedAchievementProse(t *testing.T) {
	p := resumeextract.Professional{Headline: "SECRET", Summary: "SECRET", Experience: []resumeextract.Experience{{Title: "Engineer", Company: "Acme", Summary: "SECRET", Highlights: []string{"SECRET"}}}, Projects: []resumeextract.Project{{Name: "Project", Highlights: []string{"SECRET"}}}}
	clean := SelectedProfile(p)
	prompt, err := draftUserPrompt(Input{Candidate: clean}, nil, "en")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "SECRET") {
		t.Fatal("unselected claims reached draft prompt")
	}
	if clean.Experience[0].Company != "Acme" || p.Experience[0].Summary != "SECRET" {
		t.Fatal("chronology lost or input mutated")
	}
}
