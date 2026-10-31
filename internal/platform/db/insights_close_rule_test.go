package db

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// realCloseReasons is the one answer to "did this posting really close?" for every
// insights rollup that measures how long postings stay open. It says the POSTING is
// over: it dropped off a board we crawl, its source took it down, the liveness probe
// read it as expired, or the employer closed it here. Every other closed_reason
// describes our crawler (board_unreachable, feed_empty), our rules (expired, the 45-day
// age cutoff) or our decisions (board_retired, moderated, source_misattributed), and the
// empty reason is unknown — counting any of them publishes our pipeline as the market's
// speed.
var realCloseReasons = []string{"employer_closed", "feed_removed", "probe_expired", "unseen"}

var closedReasonIn = regexp.MustCompile(`closed_reason\s+IN\s*\(([^)]*)\)`)

// TestInsightsRealCloseReasonsAgree pins every `closed_reason IN (...)` in insights.sql
// to realCloseReasons. sqlc cannot share a SQL fragment between queries, so each rollup
// carries its own copy of the list; this is what keeps the copies from drifting apart —
// the company lifetime rollup once counted expired and missed probe_expired while its
// own comment described the rule correctly.
func TestInsightsRealCloseReasonsAgree(t *testing.T) {
	src, err := os.ReadFile("queries/insights.sql")
	if err != nil {
		t.Fatalf("read insights.sql: %v", err)
	}
	// Comment lines are dropped first, then the whole file is matched at once, so a
	// list wrapped over several lines is checked like a one-line one.
	var sql []string
	for _, line := range strings.Split(string(src), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			sql = append(sql, line)
		}
	}
	var checked int
	for _, m := range closedReasonIn.FindAllStringSubmatch(strings.Join(sql, "\n"), -1) {
		checked++
		var got []string
		for _, v := range strings.Split(m[1], ",") {
			got = append(got, strings.Trim(strings.TrimSpace(v), "'"))
		}
		slices.Sort(got)
		if !slices.Equal(got, realCloseReasons) {
			t.Errorf("insights.sql: closed_reason IN (%s) is %v, want %v", strings.TrimSpace(m[1]), got, realCloseReasons)
		}
	}
	if checked == 0 {
		t.Fatal("no closed_reason IN (...) in insights.sql — has the rule moved?")
	}
}
