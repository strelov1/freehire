package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryLastActivityCountsTheEmployersMoves pins the one rule behind every
// last_activity_at: an application last moved at its apply date, its newest linked
// incoming mail, or the employer's newest move in their freehire pipeline (a live
// stage_set event with source 'employer') — whichever is latest.
//
// Six statements compute it (the board, the drawer, the orphaned rows, the follow-up
// match, the nudge's re-check before send, the ghost signal), and their comments say
// they mirror each other. The employer input was first added to two of them, which left
// the nudge re-check calling an application silent that the match had seen answered.
// A seventh reader without the input fails here instead.
func TestEveryLastActivityCountsTheEmployersMoves(t *testing.T) {
	const queryDir = "queries"
	entries, err := os.ReadDir(queryDir)
	if err != nil {
		t.Fatalf("read %s: %v", queryDir, err)
	}
	var checked int
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		path := filepath.Join(queryDir, entry.Name())
		src, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, stmt := range splitNamedQueries(string(src)) {
			body := stripSQLComments(stmt.body)
			if !strings.Contains(body, "AS last_activity_at") {
				continue
			}
			checked++
			if !strings.Contains(body, "source = 'employer'") {
				t.Errorf("%s: %s computes last_activity_at without the employer's pipeline moves — "+
					"add the newest live stage_set event with source = 'employer' to its GREATEST, "+
					"as ListUserJobs does.", path, stmt.name)
			}
		}
	}
	if checked < 6 {
		t.Fatalf("found %d last_activity_at statements, want at least 6 — has the rule moved?", checked)
	}
}
