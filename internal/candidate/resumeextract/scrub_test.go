package resumeextract

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// A placeholder the model copied out of the redacted CV must never be persisted as if it
// were a fact about the candidate. `resume_structured` is seeded into the base CV and
// rendered into the PDF, so "[REDACTED_NAME_1] is a backend engineer" would reach a
// recruiter. The system prompt already forbids this — and one line further down tells the
// model to copy each highlight verbatim, which is exactly how it happens anyway.
func TestExtract_DropsRedactionPlaceholdersTheModelCopiedBack(t *testing.T) {
	cv := "Ivan Petrov ivan@petrov.io https://ivanp.dev\nSenior Go Engineer at Acme."
	// Every string field the model controls, carrying a placeholder: the summary, an
	// entry summary, a highlight, a stack entry, a language, a skill, a certification,
	// and a project's name/link/highlight.
	m := &recordingModel{resp: `{
		"headline":"Senior Go Engineer",
		"summary":"[REDACTED_NAME_1] is a backend engineer.",
		"experience":[{"title":"Senior Go Engineer","company":"Acme",
			"summary":"Hired by [REDACTED_NAME_1].",
			"highlights":["Shipped the billing service","Reachable at [REDACTED_EMAIL_1]"],
			"stack":["Go","[REDACTED_LINK_1]"]}],
		"education":[{"degree":"BSc [REDACTED_NAME_1]","institution":"MIT"}],
		"languages":["English","[REDACTED_NAME_1]"],
		"skills":["Go","[REDACTED_EMAIL_1]"],
		"certifications":["CKA","[REDACTED_NAME_1]"],
		"projects":[{"name":"Atlas","link":"[REDACTED_LINK_1]","highlights":["Wrote it"]}]
	}`}
	e := NewExtractor(llm.NewWithModel(m), nameSpanDetector{names: []string{"Ivan Petrov"}})

	got, err := e.Extract(context.Background(), cv)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Nothing anywhere in the persisted structure may carry a placeholder.
	for field, value := range map[string]string{
		"summary":            got.Summary,
		"experience summary": got.Experience[0].Summary,
		"education degree":   got.Education[0].Degree,
		"project link":       got.Projects[0].Link,
	} {
		if strings.Contains(value, "[REDACTED_") {
			t.Errorf("%s = %q, want the placeholder blanked", field, value)
		}
	}
	for name, list := range map[string][]string{
		"highlights":     got.Experience[0].Highlights,
		"stack":          got.Experience[0].Stack,
		"languages":      got.Languages,
		"skills":         got.Skills,
		"certifications": got.Certifications,
	} {
		for _, v := range list {
			if strings.Contains(v, "[REDACTED_") {
				t.Errorf("%s contains %q, want the placeholder entry gone", name, v)
			}
		}
	}

	// Blanking, not dropping the whole entry: the real content beside the placeholder
	// survives, and Sanitize's nonEmpty pass is what removes the emptied list members.
	if len(got.Experience) != 1 || got.Experience[0].Company != "Acme" {
		t.Fatalf("experience = %+v, want the Acme role kept", got.Experience)
	}
	if len(got.Experience[0].Highlights) != 1 || got.Experience[0].Highlights[0] != "Shipped the billing service" {
		t.Errorf("highlights = %v, want only the real one", got.Experience[0].Highlights)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "Go" {
		t.Errorf("skills = %v, want only the real one", got.Skills)
	}
	if got.Education[0].Institution != "MIT" {
		t.Errorf("education = %+v, want the institution kept", got.Education[0])
	}
	if got.Projects[0].Name != "Atlas" {
		t.Errorf("project = %+v, want the name kept", got.Projects[0])
	}

	// The contact fields are overwritten from detection regardless, so a placeholder
	// there was never the hazard — but the real value must still land.
	if got.FullName != "Ivan Petrov" {
		t.Errorf("full_name = %q, want the detected value", got.FullName)
	}
}
