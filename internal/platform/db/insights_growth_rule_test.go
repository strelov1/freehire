package db

import (
	"os"
	"strings"
	"testing"
)

// existenceCompare is the one way an insights query may decide that a posting existed
// at the growth-window start.
const existenceCompare = "LEAST(posted_at, created_at) <= sqlc.arg('prev_ts')"

// TestInsightsPriorWindowCountsUseExistence pins the growth rule across every reader.
//
// Each insights rollup writes its own prior-window FILTER inline, so the rule — a
// posting existed from the earlier of its publication and first-seen dates — lives in
// as many copies as there are rollups. TestInsightsGrowthCountsExistenceNotDiscovery
// proves the copies that exist today; it cannot see a NEW rollup that copies the old
// `created_at <=` from git history, which is the likeliest way the bug returns. This
// reads the query file itself, so it can.
//
// The check is per line and deliberately blunt: every SQL line that names the window
// start must carry the rule. Matching the comparison instead would miss it written the
// other way round (`sqlc.arg('prev_ts') >= created_at`) or strictly (`<`).
func TestInsightsPriorWindowCountsUseExistence(t *testing.T) {
	src, err := os.ReadFile("queries/insights.sql")
	if err != nil {
		t.Fatalf("read insights.sql: %v", err)
	}
	var checked int
	for i, line := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") || !strings.Contains(line, "sqlc.arg('prev_ts')") {
			continue
		}
		checked++
		if !strings.Contains(line, existenceCompare) {
			t.Errorf("insights.sql:%d names prev_ts without %q (see the file header):\n%s", i+1, existenceCompare, line)
		}
	}
	if checked == 0 {
		t.Fatal("no line names sqlc.arg('prev_ts') — has the window parameter been renamed?")
	}
}
