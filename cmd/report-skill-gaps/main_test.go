package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/dictgap"
	"github.com/strelov1/freehire/internal/platform/db"
)

func skillRow(id int64, skill string) db.ListJobSkillsForGapReportRow {
	return db.ListJobSkillsForGapReportRow{ID: id, Skill: skill}
}

func TestFoldSkillCounts(t *testing.T) {
	t.Run("tallies every row into the running map", func(t *testing.T) {
		counts := map[string]int{}
		foldSkillCounts(counts, []db.ListJobSkillsForGapReportRow{
			skillRow(1, "Python"), skillRow(1, "Go"), skillRow(2, "Python"),
		}, 500, 999)

		if counts["Python"] != 2 || counts["Go"] != 1 {
			t.Fatalf("counts = %+v, want Python=2 Go=1", counts)
		}
	})

	t.Run("a full chunk resumes from the last id seen, not the range end", func(t *testing.T) {
		rows := []db.ListJobSkillsForGapReportRow{skillRow(10, "a"), skillRow(11, "b")}
		next := foldSkillCounts(map[string]int{}, rows, 2, 999)

		if next != 12 {
			t.Fatalf("next = %d, want 12 (last id 11 + 1)", next)
		}
	})

	t.Run("a partial chunk resumes at the range end", func(t *testing.T) {
		rows := []db.ListJobSkillsForGapReportRow{skillRow(10, "a")}
		next := foldSkillCounts(map[string]int{}, rows, 500, 999)

		if next != 999 {
			t.Fatalf("next = %d, want 999 (the range end, since the LIMIT wasn't hit)", next)
		}
	})

	t.Run("an empty chunk resumes at the range end", func(t *testing.T) {
		next := foldSkillCounts(map[string]int{}, nil, 500, 999)

		if next != 999 {
			t.Fatalf("next = %d, want 999", next)
		}
	})
}

func TestWriteSkillGapReport(t *testing.T) {
	var buf bytes.Buffer
	writeSkillGapReport(&buf, []dictgap.SkillGapCandidate{
		{Phrase: "Frobnicator", Count: 10},
		{Phrase: "Widgetize", Count: 3},
	}, 1)

	out := buf.String()
	if !strings.Contains(out, "10\tFrobnicator") {
		t.Errorf("output = %q, want a line for Frobnicator", out)
	}
	if strings.Contains(out, "Widgetize") {
		t.Errorf("output = %q, top=1 should exclude the second candidate", out)
	}
}
