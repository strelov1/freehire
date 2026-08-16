package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryCompanySlugWriteAlsoWritesTheFoldedColumn pins a rule that only a comment
// would otherwise hold.
//
// jobs.company_slug_folded duplicates `replace(company_slug, '-', ”)` as a stored
// column, because the planner cannot estimate a predicate on an expression when the
// values arrive as a parameter (migrations/0109 carries the measurements: 271s per
// batch against 491ms for the same shape over a plain column). It is NOT a generated
// column — adding one would rewrite a 7.4M-row, 95 GB table under ACCESS EXCLUSIVE —
// so the engine does not maintain it. The write paths do.
//
// That means a new statement writing company_slug and forgetting the folded column is
// silently wrong in the worst way available: nothing errors, the row simply stops
// matching the suppression pass, and the aggregator duplicate it should have hidden
// stays in the index. This test is cheaper than discovering that from a bug report.
func TestEveryCompanySlugWriteAlsoWritesTheFoldedColumn(t *testing.T) {
	const queryDir = "queries"

	entries, err := os.ReadDir(queryDir)
	if err != nil {
		t.Fatalf("read %s: %v", queryDir, err)
	}

	// Only jobs carries the folded column; a write to some other table's company_slug
	// (company_votes, company_feedback, application_events …) is not in scope.
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
			if !writesJobsCompanySlug(stmt.body) {
				continue
			}
			checked++
			if !strings.Contains(stmt.body, "company_slug_folded") {
				t.Errorf("%s: %s writes jobs.company_slug but not company_slug_folded — "+
					"the folded column is maintained by the write paths, not by the engine "+
					"(see migrations/0109). Add `company_slug_folded = replace(<the slug>, '-', '')`.",
					path, stmt.name)
			}
		}
	}

	// Counting the population, not just the violations: a rule that silently stops
	// matching anything looks identical to a rule that passes.
	if checked < 4 {
		t.Errorf("only %d statements matched as jobs.company_slug writers, expected at least 4 "+
			"(UpsertJob, UpsertManualJob, InsertPrivateJob, RenameSlugCompany) — the detection "+
			"below has probably drifted from how the queries are written", checked)
	}
}

type namedQuery struct {
	name string
	body string
}

// splitNamedQueries cuts a sqlc query file into its `-- name: X` statements.
func splitNamedQueries(src string) []namedQuery {
	var out []namedQuery
	var cur *namedQuery
	for _, line := range strings.Split(src, "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), "-- name:"); ok {
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &namedQuery{name: strings.Fields(after)[0]}
			continue
		}
		if cur != nil {
			cur.body += line + "\n"
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

// writesJobsCompanySlug reports whether a statement inserts into or updates
// jobs.company_slug. Deliberately loose on the write side and strict about the table:
// a false positive costs one obvious column to add, a false negative costs a silently
// unsuppressed duplicate.
func writesJobsCompanySlug(body string) bool {
	lower := strings.ToLower(body)
	touchesJobs := strings.Contains(lower, "insert into jobs") || strings.Contains(lower, "update jobs")
	if !touchesJobs {
		return false
	}
	// An INSERT names the column in its list; an UPDATE assigns to it. Reading it in a
	// WHERE/ON/RETURNING clause is not a write, so those forms must not match.
	return strings.Contains(lower, "company, company_slug,") ||
		strings.Contains(lower, "set company_slug =") ||
		strings.Contains(lower, "company_slug = @new_slug") ||
		strings.Contains(lower, "company_slug = excluded.company_slug")
}
