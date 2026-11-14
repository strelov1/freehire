package db

import (
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
// reads the query files themselves, so it can.
//
// The check is per line and deliberately blunt: every SQL line that names the window
// start must carry the rule. Matching the comparison instead would miss it written the
// other way round (`sqlc.arg('prev_ts') >= created_at`) or strictly (`<`).
func TestInsightsPriorWindowCountsUseExistence(t *testing.T) {
	var checked int
	for _, f := range readInsightsQueries(t) {
		for i, line := range strings.Split(f.src, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "--") || !strings.Contains(line, "sqlc.arg('prev_ts')") {
				continue
			}
			checked++
			if !strings.Contains(line, existenceCompare) {
				t.Errorf("%s:%d names prev_ts without %q (see insights.sql's header):\n%s", f.path, i+1, existenceCompare, line)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no line names sqlc.arg('prev_ts') — has the window parameter been renamed?")
	}
}
