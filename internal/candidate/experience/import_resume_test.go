package experience

import (
	"testing"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

func structuredFixture() resumeextract.Structured {
	return resumeextract.Structured{
		FullName: "Ilya Strelov",
		Email:    "someone@example.test",
		Experience: []resumeextract.Experience{
			{
				Title: "Senior Software Engineer", Company: "RingCentral", Location: "USA, Remote",
				Start: &perioddate.PeriodDate{Year: 2023, Month: 9}, Current: true, Summary: "Global SaaS leader",
				Highlights: []string{"Cut latency 20s to 1s", "Sustained a 99.999% SLA"},
				Stack:      []string{"golang", "mongodb"},
			},
			{
				Title: "Team Lead", Company: "Sber",
				Start: &perioddate.PeriodDate{Year: 2020, Month: 4}, End: &perioddate.PeriodDate{Year: 2022, Month: 4},
				Highlights: []string{"Cut report load by 95%"},
			},
		},
		Projects: []resumeextract.Project{
			{Name: "telagon.io", Link: "https://telagon.io", Highlights: []string{"1.4M+ channels indexed"}},
		},
	}
}

func TestImportEntriesFromStructured(t *testing.T) {
	entries := EntriesFromResume(structuredFixture())

	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3 (two roles and one project)", len(entries))
	}

	first := entries[0]
	if first.Employment.Kind != KindJob {
		t.Errorf("kind = %q, want job", first.Employment.Kind)
	}
	if first.Employment.Company != "RingCentral" || first.Employment.Role != "Senior Software Engineer" {
		t.Errorf("employment = %+v, want the CV's company and title", first.Employment)
	}
	if len(first.Claims) != 2 {
		t.Errorf("claims = %d, want the role's two highlights", len(first.Claims))
	}
	if len(first.Employment.Stack) != 2 {
		t.Errorf("stack = %q, want the role's technology line carried over", first.Employment.Stack)
	}

	project := entries[2]
	if project.Employment.Kind != KindProject {
		t.Errorf("project kind = %q, want project", project.Employment.Kind)
	}
	if project.Employment.Company != "telagon.io" {
		t.Errorf("project = %+v, want the project named as its place", project.Employment)
	}
	if project.Employment.Link != "https://telagon.io" {
		t.Errorf("project link = %q, want the portfolio URL retained", project.Employment.Link)
	}
}

// Current is copied straight from what resumeextract decided (the model reads "Present"
// or an absent end date and sets Current itself, per its own schema) — EntriesFromResume
// no longer derives it from an End label.
func TestImportEntriesCarryCurrentThrough(t *testing.T) {
	tests := []struct {
		current bool
	}{{true}, {false}}
	for _, tt := range tests {
		entries := EntriesFromResume(resumeextract.Structured{
			Experience: []resumeextract.Experience{{Company: "X", Title: "Y", Current: tt.current}},
		})
		if got := entries[0].Employment.Current; got != tt.current {
			t.Errorf("current %v -> got %v", tt.current, got)
		}
	}
}

// Contacts have no business in the bank: the bank is what the candidate did, and the
// identity stays on the résumé record.
func TestImportEntriesCarryNoContacts(t *testing.T) {
	for _, entry := range EntriesFromResume(structuredFixture()) {
		if entry.Employment.Summary == "Ilya Strelov" || entry.Employment.Company == "someone@example.test" {
			t.Errorf("a contact field leaked into the bank: %+v", entry.Employment)
		}
	}
}
