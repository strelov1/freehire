package cv

import (
	"testing"

	"github.com/strelov1/freehire/internal/resumeextract"
)

func TestSeedMapsStructured(t *testing.T) {
	s := resumeextract.Structured{
		FullName: "Ada Lovelace",
		Headline: "Backend Engineer",
		Location: "London, UK",
		Email:    "ada@example.com",
		Phone:    "+44 000",
		Summary:  "Ten years of systems work.",
		Links:    []string{"github.com/ada"},
		Experience: []resumeextract.Experience{
			{Title: "Senior Engineer", Company: "Analytical Engines", Location: "London", Start: "2018", End: "Present",
				Summary: "Pioneering computing company.", Highlights: []string{"Built the difference engine."}, Stack: []string{"Assembly"}},
		},
		Education: []resumeextract.Education{
			{Degree: "BSc Mathematics", Institution: "Cambridge", Year: "1835"},
		},
		Languages: []string{"English", "French"},
	}

	doc := Seed(s)

	if doc.Header.FullName != "Ada Lovelace" || doc.Header.Email != "ada@example.com" {
		t.Errorf("header not seeded: %+v", doc.Header)
	}
	if doc.Header.Location != "London, UK" {
		t.Errorf("header location not seeded: %+v", doc.Header)
	}
	if len(doc.Header.Links) != 1 || doc.Header.Links[0] != "github.com/ada" {
		t.Errorf("links not seeded: %+v", doc.Header.Links)
	}
	// Summary prefers the extracted summary (falls back to the headline when absent).
	if doc.Summary != "Ten years of systems work." {
		t.Errorf("summary not seeded: %q", doc.Summary)
	}
	if len(doc.Experience) != 1 {
		t.Fatalf("experience not seeded: %+v", doc.Experience)
	}
	e := doc.Experience[0]
	if e.Role != "Senior Engineer" || e.Company != "Analytical Engines" || e.Location != "London" || e.Start != "2018" || e.End != "Present" {
		t.Errorf("experience fields not seeded: %+v", e)
	}
	if e.Summary != "Pioneering computing company." {
		t.Errorf("experience summary not seeded: %q", e.Summary)
	}
	if len(e.Bullets) != 1 || e.Bullets[0] != "Built the difference engine." {
		t.Errorf("experience highlights not carried into bullets: %+v", e.Bullets)
	}
	if len(e.Stack) != 1 || e.Stack[0] != "Assembly" {
		t.Errorf("experience stack not seeded: %+v", e.Stack)
	}
	if len(doc.Education) != 1 || doc.Education[0].Degree != "BSc Mathematics" || doc.Education[0].End != "1835" {
		t.Errorf("education not seeded: %+v", doc.Education)
	}
	if len(doc.Languages) != 2 || doc.Languages[0].Name != "English" {
		t.Errorf("languages not seeded: %+v", doc.Languages)
	}
}

func TestSeedEmptyStructureIsValidSkeleton(t *testing.T) {
	doc := Seed(resumeextract.Structured{})
	before := doc
	doc.Sanitize()
	if !equalDocument(before, doc) {
		t.Errorf("seed of empty structure is not sanitize-stable")
	}
}
