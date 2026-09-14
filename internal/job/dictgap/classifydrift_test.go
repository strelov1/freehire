package dictgap

import "testing"

func TestClassifyDriftCandidatesSurfacesSeniorityDisagreement(t *testing.T) {
	report := ClassifyDriftCandidates([]TitleClassification{
		{Title: "Member of Technical Staff", Count: 12, EnrichmentSeniority: "senior", EnrichmentCategory: ""},
	})

	if len(report.Seniority) != 1 {
		t.Fatalf("got %d seniority candidates, want 1: %+v", len(report.Seniority), report.Seniority)
	}
	got := report.Seniority[0]
	if got.Title != "Member of Technical Staff" || got.Count != 12 {
		t.Fatalf("got %+v, want title/count preserved", got)
	}
	if got.EnrichmentValue != "senior" {
		t.Fatalf("got enrichment value %q, want %q", got.EnrichmentValue, "senior")
	}
	if got.DictionaryValue != "" {
		t.Fatalf("got dictionary value %q, want \"\" (grade-blind title)", got.DictionaryValue)
	}
}

func TestClassifyDriftCandidatesSurfacesCategoryDisagreement(t *testing.T) {
	report := ClassifyDriftCandidates([]TitleClassification{
		{Title: "Registered Nurse", Count: 4, EnrichmentSeniority: "", EnrichmentCategory: "backend"},
	})

	if len(report.Category) != 1 {
		t.Fatalf("got %d category candidates, want 1: %+v", len(report.Category), report.Category)
	}
	got := report.Category[0]
	if got.Title != "Registered Nurse" || got.EnrichmentValue != "backend" {
		t.Fatalf("got %+v, want enrichment value backend", got)
	}
}

func TestClassifyDriftCandidatesExcludesAgreement(t *testing.T) {
	report := ClassifyDriftCandidates([]TitleClassification{
		{Title: "Senior Backend Engineer", Count: 50, EnrichmentSeniority: "senior", EnrichmentCategory: "backend"},
	})

	if len(report.Seniority) != 0 || len(report.Category) != 0 {
		t.Fatalf("got seniority=%+v category=%+v, want both empty (dictionary agrees with enrichment)",
			report.Seniority, report.Category)
	}
}

func TestClassifyDriftCandidatesSkipsEmptyEnrichmentValue(t *testing.T) {
	report := ClassifyDriftCandidates([]TitleClassification{
		{Title: "Some Ambiguous Title", Count: 3, EnrichmentSeniority: "", EnrichmentCategory: ""},
	})

	if len(report.Seniority) != 0 || len(report.Category) != 0 {
		t.Fatalf("got seniority=%+v category=%+v, want both empty (no enrichment opinion to disagree with)",
			report.Seniority, report.Category)
	}
}

func TestClassifyDriftCandidatesSortsDescendingByCount(t *testing.T) {
	report := ClassifyDriftCandidates([]TitleClassification{
		{Title: "Member of Technical Staff", Count: 2, EnrichmentSeniority: "senior", EnrichmentCategory: ""},
		{Title: "Lead Generation Specialist", Count: 40, EnrichmentSeniority: "lead", EnrichmentCategory: ""},
	})

	if len(report.Seniority) != 2 {
		t.Fatalf("got %d seniority candidates, want 2: %+v", len(report.Seniority), report.Seniority)
	}
	if report.Seniority[0].Title != "Lead Generation Specialist" || report.Seniority[1].Title != "Member of Technical Staff" {
		t.Fatalf("got %+v, want Lead Generation Specialist (40) before Member of Technical Staff (2)", report.Seniority)
	}
}
