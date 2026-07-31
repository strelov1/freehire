package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvmatch"
	"github.com/strelov1/freehire/internal/matchanalysis"
)

// A job-match score is assembled from the rendered text, the vacancy and the cached
// requirement ledger. These tests exercise that assembly directly; the HTTP shell (auth,
// ownership, the 409s) is covered by the integration suite.

func scoringTemplate(t *testing.T) cv.Template {
	t.Helper()
	tmpl, err := cv.ResolveTemplate("classic-ats")
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}
	return tmpl
}

func TestJobMatchScore_ReadsTheRenderedTextLayer(t *testing.T) {
	h := &cvHandlers{cvRenderer: &fakeCVRenderer{pdf: []byte(scorableCV)}, extractPDFText: textFromPDF}

	score, err := h.cvJobMatchScore(context.Background(), cv.Document{Summary: "ignored by the fake"}, scoringTemplate(t),
		cvmatch.Input{JobTitle: "Senior Backend Engineer", JobSkills: []string{"go", "kafka", "terraform"}})
	if err != nil {
		t.Fatalf("cvJobMatchScore: %v", err)
	}

	// The fake's text layer names Go and Kafka but not Terraform.
	if len(score.MissingSkills) != 1 || score.MissingSkills[0] != "terraform" {
		t.Errorf("missing skills = %v, want [terraform]", score.MissingSkills)
	}
	if score.Overall <= 0 {
		t.Errorf("overall = %d, want a positive score", score.Overall)
	}
}

// The document is the sole scoring input: nothing about the base CV or the candidate's
// banked profile may reach it. Rendering exactly once is how that stays true — and it is
// what makes this score cheap enough to refresh on every save, where the two-render delta
// is not.
func TestJobMatchScore_RendersOnlyTheTailoredCopy(t *testing.T) {
	r := &fakeCVRenderer{pdf: []byte(scorableCV)}
	h := &cvHandlers{cvRenderer: r, extractPDFText: textFromPDF}

	if _, err := h.cvJobMatchScore(context.Background(), cv.Document{}, scoringTemplate(t),
		cvmatch.Input{JobTitle: "Backend Engineer", JobSkills: []string{"go"}}); err != nil {
		t.Fatalf("cvJobMatchScore: %v", err)
	}

	if r.calls != 1 {
		t.Errorf("render calls = %d, want exactly 1", r.calls)
	}
}

func TestJobMatchScore_WithoutARendererReportsTheToolchain(t *testing.T) {
	h := &cvHandlers{}

	_, err := h.cvJobMatchScore(context.Background(), cv.Document{}, scoringTemplate(t),
		cvmatch.Input{JobTitle: "Backend Engineer", JobSkills: []string{"go"}})

	if !errors.Is(err, errNoRenderer) {
		t.Fatalf("err = %v, want errNoRenderer", err)
	}
}

// Without a cached analysis the heaviest category drops out and the rest still score.
func TestJobMatchScore_WithoutAnAnalysisScoresTheOtherCategories(t *testing.T) {
	h := &cvHandlers{cvRenderer: &fakeCVRenderer{pdf: []byte(scorableCV)}, extractPDFText: textFromPDF}

	score, err := h.cvJobMatchScore(context.Background(), cv.Document{}, scoringTemplate(t),
		cvmatch.Input{JobTitle: "Senior Backend Engineer", JobSkills: []string{"go"}})
	if err != nil {
		t.Fatalf("cvJobMatchScore: %v", err)
	}

	for _, c := range score.Categories {
		if c.ID == cvmatch.CategoryRequirements && c.Available {
			t.Error("requirements coverage must be unavailable with no cached analysis")
		}
	}
	if score.Overall <= 0 {
		t.Errorf("overall = %d, want the remaining categories to still score", score.Overall)
	}
}

// The CV's skills are parsed from the rendered text, not from the stored document, so the
// job-match score and the ATS delta can never disagree about what the CV says.
func TestJobMatchScore_SkillsComeFromTheRenderedText(t *testing.T) {
	// A realistic text layer: "rust" is an ordinary English word too, so the skill
	// dictionary only tags it where the surrounding text corroborates a technical reading.
	h := &cvHandlers{
		cvRenderer:     &fakeCVRenderer{pdf: []byte("Jane Roe\nSummary\nSystems engineer. Stack: Rust, PostgreSQL, gRPC.")},
		extractPDFText: textFromPDF,
	}

	score, err := h.cvJobMatchScore(context.Background(),
		cv.Document{Summary: "Kubernetes platform lead"}, // never rendered by the fake
		scoringTemplate(t),
		cvmatch.Input{JobTitle: "Systems Engineer", JobSkills: []string{"rust", "kubernetes"}})
	if err != nil {
		t.Fatalf("cvJobMatchScore: %v", err)
	}

	if len(score.MissingSkills) != 1 || score.MissingSkills[0] != "kubernetes" {
		t.Errorf("missing = %v, want [kubernetes] — the document's own text must not count", score.MissingSkills)
	}
}

// Only the requirement's text and priority are borrowed from the cache; the status is
// recomputed, because the cached one was reached against the base profile.
func TestJobMatchRequirements_BorrowTextAndPriorityOnly(t *testing.T) {
	got := cvJobMatchRequirements(&matchanalysis.Analysis{
		RequirementMatch: []matchanalysis.Requirement{
			{Text: "5+ years of Go", Priority: "required", Status: "missing-gap", Evidence: "not found"},
			{Text: "Strong communication", Priority: "preferred", Status: "covered"},
		},
	})

	if len(got) != 2 {
		t.Fatalf("requirements = %d, want 2", len(got))
	}
	if got[0].Text != "5+ years of Go" || got[0].Priority != "required" {
		t.Errorf("first requirement = %+v, want its text and priority carried", got[0])
	}
	if got[0].CachedStatus != "missing-gap" {
		t.Errorf("cached status = %q, want it carried for the unverifiable fallback", got[0].CachedStatus)
	}
}

func TestJobMatchRequirements_OfANilAnalysisIsEmpty(t *testing.T) {
	if got := cvJobMatchRequirements(nil); len(got) != 0 {
		t.Errorf("requirements = %v, want none", got)
	}
}
