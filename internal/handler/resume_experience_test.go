package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// fakeBank records what the upload path handed the experience bank, so the wiring can be
// tested without a database: what matters here is WHICH entries were built from a parsed
// CV, and whether the import ran at all.
type fakeBank struct {
	calls     int
	entries   []experience.ImportEntry
	sourceRef string
	err       error
}

func (f *fakeBank) Import(_ context.Context, _ int64, entries []experience.ImportEntry, sourceRef string) (experience.ImportResult, error) {
	f.calls++
	f.entries = entries
	f.sourceRef = sourceRef
	return experience.ImportResult{}, f.err
}

func structuredFixture() resumeextract.Structured {
	return resumeextract.Structured{
		FullName: "Ilya Strelov",
		Email:    "someone@example.test",
		Experience: []resumeextract.Experience{
			{
				Title: "Senior Software Engineer", Company: "RingCentral", Location: "USA, Remote",
				Start: "2023-09", End: "Present", Summary: "Global SaaS leader",
				Highlights: []string{"Cut latency 20s to 1s", "Sustained a 99.999% SLA"},
				Stack:      []string{"golang", "mongodb"},
			},
			{
				Title: "Team Lead", Company: "Sber", Start: "2020-04", End: "2022-04",
				Highlights: []string{"Cut report load by 95%"},
			},
		},
		Projects: []resumeextract.Project{
			{Name: "telagon.io", Link: "https://telagon.io", Highlights: []string{"1.4M+ channels indexed"}},
		},
	}
}

func TestImportEntriesFromStructured(t *testing.T) {
	entries := importEntriesFromStructured(structuredFixture())

	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3 (two roles and one project)", len(entries))
	}

	first := entries[0]
	if first.Employment.Kind != experience.KindJob {
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
	if project.Employment.Kind != experience.KindProject {
		t.Errorf("project kind = %q, want project", project.Employment.Kind)
	}
	if project.Employment.Company != "telagon.io" {
		t.Errorf("project = %+v, want the project named as its place", project.Employment)
	}
}

// A CV states "Present" rather than a flag, so the current role is derived from the end
// label. Getting this wrong would mark every past job as current on the seeded CV.
func TestImportEntriesDeriveTheCurrentRole(t *testing.T) {
	tests := []struct {
		end  string
		want bool
	}{
		{"Present", true},
		{"present", true},
		{"Current", true},
		{"", true}, // an open-ended role
		{"2022-04", false},
		{"Apr 2022", false},
	}
	for _, tt := range tests {
		entries := importEntriesFromStructured(resumeextract.Structured{
			Experience: []resumeextract.Experience{{Company: "X", Title: "Y", End: tt.end}},
		})
		if got := entries[0].Employment.Current; got != tt.want {
			t.Errorf("end %q -> current = %v, want %v", tt.end, got, tt.want)
		}
	}
}

// Contacts have no business in the bank: the bank is what the candidate did, and the
// identity stays on the résumé record.
func TestImportEntriesCarryNoContacts(t *testing.T) {
	for _, entry := range importEntriesFromStructured(structuredFixture()) {
		if entry.Employment.Summary == "Ilya Strelov" || entry.Employment.Company == "someone@example.test" {
			t.Errorf("a contact field leaked into the bank: %+v", entry.Employment)
		}
	}
}

func TestImportExperienceRunsOnASuccessfulExtract(t *testing.T) {
	bank := &fakeBank{}
	h := &resumeHandlers{bank: bank}

	h.importExperience(context.Background(), 7, structuredFixture(), "2026-07-28T10:00:00Z")

	if bank.calls != 1 {
		t.Fatalf("bank.calls = %d, want 1", bank.calls)
	}
	if bank.sourceRef != "2026-07-28T10:00:00Z" {
		t.Errorf("sourceRef = %q, want the upload stamp", bank.sourceRef)
	}
	if len(bank.entries) != 3 {
		t.Errorf("entries = %d, want 3", len(bank.entries))
	}
}

// The bank is best-effort exactly like the rest of the derive path: an unconfigured bank
// or a failing import must never surface to the upload.
func TestImportExperienceIsBestEffort(t *testing.T) {
	h := &resumeHandlers{bank: nil}
	h.importExperience(context.Background(), 7, structuredFixture(), "stamp") // must not panic

	failing := &fakeBank{err: errors.New("database down")}
	h = &resumeHandlers{bank: failing}
	h.importExperience(context.Background(), 7, structuredFixture(), "stamp")
	if failing.calls != 1 {
		t.Errorf("bank.calls = %d, want the failure swallowed after one attempt", failing.calls)
	}
}

// An extraction that produced no work history has nothing to bank; the import must not
// run at all rather than call with an empty slice.
func TestImportExperienceSkipsAnEmptyStructure(t *testing.T) {
	bank := &fakeBank{}
	h := &resumeHandlers{bank: bank}

	h.importExperience(context.Background(), 7, resumeextract.Structured{FullName: "Ilya Strelov"}, "stamp")

	if bank.calls != 0 {
		t.Errorf("bank.calls = %d, want 0 — nothing was extracted to bank", bank.calls)
	}
}
