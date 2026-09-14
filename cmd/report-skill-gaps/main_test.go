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

	t.Run("a chunk that ends mid-job holds back that job's rows and resumes AT its id", func(t *testing.T) {
		// The LATERAL expansion is one row per (job id, skill): nothing guarantees
		// the row LIMIT lands on a job-id boundary. Job 11 might have more skills
		// than fit in this chunk, so its rows must not be tallied yet, and the next
		// chunk must start AT id 11 (not 12) to re-read it whole rather than
		// silently losing whatever this chunk's LIMIT cut off.
		counts := map[string]int{}
		rows := []db.ListJobSkillsForGapReportRow{
			skillRow(10, "a"),
			skillRow(11, "b"),
			skillRow(11, "c"),
		}
		next := foldSkillCounts(counts, rows, 3, 999)

		if next != 11 {
			t.Fatalf("next = %d, want 11 (resume AT the boundary job, not past it)", next)
		}
		if counts["a"] != 1 {
			t.Fatalf("counts = %+v, want job 10's row tallied", counts)
		}
		if counts["b"] != 0 || counts["c"] != 0 {
			t.Fatalf("counts = %+v, want job 11's rows held back until it is re-read whole", counts)
		}
	})

	t.Run("a chunk entirely made of one id is tallied and moves past it", func(t *testing.T) {
		// If a single job's own skill list alone fills the row limit, there is no
		// earlier id to fall back to — holding it back would repeat forever.
		// Trading a possible undercount for guaranteed forward progress.
		counts := map[string]int{}
		rows := []db.ListJobSkillsForGapReportRow{skillRow(10, "a"), skillRow(10, "b")}
		next := foldSkillCounts(counts, rows, 2, 999)

		if next != 11 {
			t.Fatalf("next = %d, want 11 (move past the single id rather than repeat it forever)", next)
		}
		if counts["a"] != 1 || counts["b"] != 1 {
			t.Fatalf("counts = %+v, want both of job 10's rows tallied (nothing earlier to fall back to)", counts)
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
	if err := writeSkillGapReport(&buf, []dictgap.SkillGapCandidate{
		{Phrase: "Frobnicator", Count: 10},
		{Phrase: "Widgetize", Count: 3},
	}, 1); err != nil {
		t.Fatalf("writeSkillGapReport: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "10\tFrobnicator") {
		t.Errorf("output = %q, want a line for Frobnicator", out)
	}
	if strings.Contains(out, "Widgetize") {
		t.Errorf("output = %q, top=1 should exclude the second candidate", out)
	}
}
