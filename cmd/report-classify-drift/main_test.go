package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/dictgap"
	"github.com/strelov1/freehire/internal/platform/db"
)

func titleRow(title string, count int64, seniority, category string) db.ListTitlesForClassifyDriftRow {
	return db.ListTitlesForClassifyDriftRow{
		Title:               title,
		JobCount:            count,
		EnrichmentSeniority: seniority,
		EnrichmentCategory:  category,
	}
}

func TestMergeTitleClassifications(t *testing.T) {
	t.Run("a title seen in only one chunk is added as-is", func(t *testing.T) {
		acc := map[string]*dictgap.TitleClassification{}
		mergeTitleClassifications(acc, []db.ListTitlesForClassifyDriftRow{
			titleRow("Senior Backend Engineer", 5, "senior", "backend"),
		})

		got, ok := acc["Senior Backend Engineer"]
		if !ok {
			t.Fatalf("title not merged: %+v", acc)
		}
		if got.Count != 5 || got.EnrichmentSeniority != "senior" || got.EnrichmentCategory != "backend" {
			t.Fatalf("got %+v, want Count=5 EnrichmentSeniority=senior EnrichmentCategory=backend", got)
		}
	})

	t.Run("a title split across chunks sums its count instead of overwriting it", func(t *testing.T) {
		acc := map[string]*dictgap.TitleClassification{}
		mergeTitleClassifications(acc, []db.ListTitlesForClassifyDriftRow{
			titleRow("Registered Nurse", 3, "", "healthcare"),
		})
		mergeTitleClassifications(acc, []db.ListTitlesForClassifyDriftRow{
			titleRow("Registered Nurse", 4, "", "healthcare"),
		})

		got := acc["Registered Nurse"]
		if got.Count != 7 {
			t.Fatalf("count = %d, want 7 (3+4 across chunks)", got.Count)
		}
	})
}

func TestWriteDriftReport(t *testing.T) {
	report := dictgap.DriftReport{
		Seniority: []dictgap.DriftCandidate{
			{Title: "Member of Technical Staff", Count: 12, DictionaryValue: "", EnrichmentValue: "senior"},
		},
		Category: []dictgap.DriftCandidate{
			{Title: "Registered Nurse", Count: 4, DictionaryValue: "", EnrichmentValue: "healthcare"},
		},
	}

	var buf bytes.Buffer
	if err := writeDriftReport(&buf, report, 200); err != nil {
		t.Fatalf("writeDriftReport: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "12\tMember of Technical Staff\t\tsenior") {
		t.Errorf("output = %q, want a seniority line for Member of Technical Staff", out)
	}
	if !strings.Contains(out, "4\tRegistered Nurse\t\thealthcare") {
		t.Errorf("output = %q, want a category line for Registered Nurse", out)
	}
}
