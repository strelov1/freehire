package db

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// assignsDuplicateOf matches an ASSIGNMENT to duplicate_of — `SET duplicate_of =` or a bare
// `duplicate_of =` continuing a SET list — and not a read. Reads in this codebase always carry a
// table alias (`WHERE j.duplicate_of IS DISTINCT FROM …`, `JOIN walk w ON j.duplicate_of = w.id`)
// or sit behind WHERE/ON/AND, so anchoring at the start of a trimmed line with no qualifier
// separates the two. The `=` matters: without it `duplicate_of_role` would match this pattern.
var assignsDuplicateOf = regexp.MustCompile(`(?i)^\s*(set\s+)?duplicate_of\s*=`)

// insertsDuplicateOf matches duplicate_of appearing as a written column in an INSERT column list.
// The trailing comma or paren is what keeps duplicate_of_role from matching.
var insertsDuplicateOf = regexp.MustCompile(`(?i)\bduplicate_of\s*[,)]`)

// TestNoQueryWritesDuplicateOfDirectly pins the ownership rule that migration 0115 enforces at
// runtime.
//
// duplicate_of is derived by a trigger from duplicate_of_aggregator, duplicate_of_role and
// duplicate_of_fuzzy. A statement that assigns duplicate_of itself is not an error and does not
// fail: the trigger simply overwrites it with the derivation. So the write silently does nothing,
// which is the worst failure mode available — the author sees a successful UPDATE and a row that
// did not change.
//
// The rule exists because that is precisely how the defect this change fixes was possible: three
// passes writing one column, each assuming it owned it. Whoever adds a fourth marker source
// should be told which column to name, at test time rather than in production.
func TestNoQueryWritesDuplicateOfDirectly(t *testing.T) {
	const queryDir = "queries"

	entries, err := os.ReadDir(queryDir)
	if err != nil {
		t.Fatalf("read %s: %v", queryDir, err)
	}

	var owners int
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
			// Judge the SQL, not the prose: the migrations and several query comments discuss
			// duplicate_of at length, and a comment is not a write.
			body := stripSQLComments(stmt.body)
			lower := strings.ToLower(body)
			if !strings.Contains(lower, "insert into jobs") && !strings.Contains(lower, "update jobs") {
				continue
			}

			if strings.Contains(body, "duplicate_of_aggregator =") ||
				strings.Contains(body, "duplicate_of_role =") ||
				strings.Contains(body, "duplicate_of_fuzzy =") {
				owners++
			}

			for _, line := range strings.Split(body, "\n") {
				if assignsDuplicateOf.MatchString(line) {
					t.Errorf("%s: %s assigns duplicate_of directly — the trigger in "+
						"migrations/0115 derives that column, so the write would be silently "+
						"discarded. Name the owning column instead: duplicate_of_aggregator, "+
						"duplicate_of_role or duplicate_of_fuzzy.", path, stmt.name)
				}
			}
			if strings.Contains(lower, "insert into jobs") && insertsDuplicateOf.MatchString(body) {
				t.Errorf("%s: %s names duplicate_of in an INSERT column list — same problem, "+
					"same fix: insert the owning column.", path, stmt.name)
			}
		}
	}

	// Counting the population, not just the violations: a rule that stops matching anything
	// looks identical to a rule that passes.
	if owners < 4 {
		t.Errorf("only %d statements matched as owned-marker writers, expected at least 4 "+
			"(RecomputeRoleDuplicatesForCompanies, SuppressAggregatorDuplicatesForCompanies, "+
			"MarkFuzzyDuplicatesForCompany, MarkJobDuplicateOfRole) — the detection has "+
			"probably drifted from how the queries are written", owners)
	}
}
