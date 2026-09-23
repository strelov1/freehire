package main

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/dictgap"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The SQL groups per id chunk, so one title arrives once per chunk it appears in.
// Summing here is what makes the ranking describe the catalogue rather than the
// largest chunk a title happened to land in.
func TestMergeTitleCountsSumsAcrossChunks(t *testing.T) {
	acc := map[string]int{}
	mergeTitleCounts(acc, []db.ListTitlesForUnclassifiedReportRow{
		{Title: "Швея", JobCount: 10},
		{Title: "Горничная", JobCount: 4},
	})
	mergeTitleCounts(acc, []db.ListTitlesForUnclassifiedReportRow{
		{Title: "Швея", JobCount: 12},
	})

	if acc["Швея"] != 22 {
		t.Errorf("Швея = %d, want 22 (10+12)", acc["Швея"])
	}
	if acc["Горничная"] != 4 {
		t.Errorf("Горничная = %d, want 4", acc["Горничная"])
	}
}

// The report prints count-first, tab-separated, so it pipes into sort/cut/grep — the
// tools a curator actually reads 1.5M titles with.
func TestWriteReportPrintsCountThenTitle(t *testing.T) {
	var out strings.Builder
	err := writeReport(&out, []dictgap.TitleCount{
		{Title: "Музыкальный руководитель", Count: 40},
		{Title: "Швея", Count: 12},
	}, 10)
	if err != nil {
		t.Fatalf("writeReport: %v", err)
	}
	want := "40\tМузыкальный руководитель\n12\tШвея\n"
	if out.String() != want {
		t.Errorf("report = %q, want %q", out.String(), want)
	}
}

// The cap is what keeps the report readable: there are 1.5M distinct unrecognised
// titles, and printing all of them is the same as printing none.
func TestWriteReportHonoursTheCap(t *testing.T) {
	var out strings.Builder
	if err := writeReport(&out, []dictgap.TitleCount{
		{Title: "a", Count: 3},
		{Title: "b", Count: 2},
		{Title: "c", Count: 1},
	}, 2); err != nil {
		t.Fatalf("writeReport: %v", err)
	}
	if lines := strings.Count(out.String(), "\n"); lines != 2 {
		t.Errorf("printed %d lines, want 2", lines)
	}
	if strings.Contains(out.String(), "c") {
		t.Error("printed past the cap")
	}
}
