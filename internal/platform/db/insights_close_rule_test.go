package db

import (
	"os"
	"path/filepath"
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

// TestInsightsRealCloseReasonsAgree pins every `closed_reason IN (...)` in the insights queries
// to realCloseReasons. sqlc cannot share a SQL fragment between queries, so each rollup
// carries its own copy of the list; this is what keeps the copies from drifting apart —
// the company lifetime rollup once counted expired and missed probe_expired while its
// own comment described the rule correctly.
func TestInsightsRealCloseReasonsAgree(t *testing.T) {
	// Comment lines are dropped first, then the whole of the files is matched at once, so
	// a list wrapped over several lines is checked like a one-line one.
	var sql []string
	for _, f := range readInsightsQueries(t) {
		for _, line := range strings.Split(f.src, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "--") {
				sql = append(sql, line)
			}
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
			t.Errorf("insights queries: closed_reason IN (%s) is %v, want %v", strings.TrimSpace(m[1]), got, realCloseReasons)
		}
	}
	if checked == 0 {
		t.Fatal("no closed_reason IN (...) in the insights queries — has the rule moved?")
	}
}

// insightsQuery is one queries/insights*.sql file: its path and its text.
type insightsQuery struct {
	path string
	src  string
}

// readInsightsQueries reads every queries/insights*.sql file, so the rule tests here hold
// every insights rollup to their rule whichever file it is written in.
func readInsightsQueries(t *testing.T) []insightsQuery {
	t.Helper()
	paths, err := filepath.Glob("queries/insights*.sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("glob queries/insights*.sql: %v (found %d)", err, len(paths))
	}
	out := make([]insightsQuery, 0, len(paths))
	for _, path := range paths {
		src, err := os.ReadFile(path) //nolint:gosec // a fixed glob over this package's own queries
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		out = append(out, insightsQuery{path: path, src: string(src)})
	}
	return out
}
