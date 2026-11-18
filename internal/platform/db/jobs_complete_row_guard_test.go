package db

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The complete job row (db.Job) is the jobs_read view: jobs plus job_activity's liveness
// stamps. The BASE table's own row is db.JobRow and nothing may ask for all of its columns:
// sqlc expands `SELECT *`, `RETURNING *`, `jobs.*` and `sqlc.embed(jobs)` into an explicit list
// that names last_seen_at, hydrated_at and liveness_strikes, so such a statement would fail
// with 42703 the moment the contract release drops those columns while a binary that still
// carries it runs. A reader wants jobs_read; a writer returns the narrow columns it needs
// (usually the id) and the caller re-reads through GetJob.
var (
	// `select * from jobs` and `select *, x from jobs`, schema-qualified or not.
	reBaseTableStar = regexp.MustCompile(`(?s)\bselect\s+(distinct\s+)?\*\s*(,[^;]*?)?\bfrom\s+(public\.)?jobs\b`)
	// A relation reference with an optional alias: group 2 is the relation, group 5 the alias.
	reRelationRef = regexp.MustCompile(`\b(from|join|update|into)\s+([\w.]+)(\s+as)?(\s+(\w+))?`)
	reJobsWrite   = regexp.MustCompile(`\b(insert\s+into|update)\s+(public\.)?jobs\b`)
	reReturning   = regexp.MustCompile(`\breturning\s+(\*)?`)
)

// notAnAlias are the words that can follow a table name without being its alias.
var notAnAlias = map[string]bool{
	"set": true, "where": true, "on": true, "using": true, "left": true, "right": true, "inner": true,
	"cross": true, "full": true, "join": true, "order": true, "group": true, "limit": true, "for": true,
	"returning": true, "values": true, "as": true, "select": true, "union": true, "from": true, "and": true,
	"or": true, "natural": true, "offset": true, "having": true, "window": true, "except": true, "intersect": true,
	"tablesample": true, "lateral": true, "do": true, "default": true, "overriding": true, "with": true,
}

// jobsNames is every name the base table goes by in a statement: its own and each alias that
// is bound to it and to nothing else. A statement reuses short aliases across CTEs (`jobs a` in
// one, a CTE `agg a` in another), and `a.*` there cannot be attributed to the table, so an
// alias bound to more than one relation is not judged.
func jobsNames(sql string) []string {
	bound := map[string]map[string]bool{}
	for _, m := range reRelationRef.FindAllStringSubmatch(sql, -1) {
		alias := m[5]
		if alias == "" || notAnAlias[alias] {
			continue
		}
		rel := strings.TrimPrefix(m[2], "public.")
		if bound[alias] == nil {
			bound[alias] = map[string]bool{}
		}
		bound[alias][rel] = true
	}
	names := []string{"jobs"}
	for alias, rels := range bound {
		if len(rels) == 1 && rels["jobs"] {
			names = append(names, alias)
		}
	}
	return names
}

// jobsWriteReturnsStar reports whether the first RETURNING after a write on jobs is `*`. The
// first one is the write's own: a CTE write on another table later in the statement may
// legitimately return its own `*`.
func jobsWriteReturnsStar(sql string) bool {
	for _, w := range reJobsWrite.FindAllStringIndex(sql, -1) {
		if m := reReturning.FindStringSubmatch(sql[w[1]:]); m != nil && m[1] == "*" {
			return true
		}
	}
	return false
}

// baseTableCompleteRowUses names what a statement does that expands to every jobs column.
func baseTableCompleteRowUses(body string) []string {
	sql := strings.ToLower(stripSQLComments(body))
	var out []string
	if reBaseTableStar.MatchString(sql) {
		out = append(out, "SELECT * FROM jobs")
	}
	for _, name := range jobsNames(sql) {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\.\*`).MatchString(sql) {
			out = append(out, name+".* of the jobs table")
		}
		if regexp.MustCompile(`sqlc\.embed\(\s*` + regexp.QuoteMeta(name) + `\s*\)`).MatchString(sql) {
			out = append(out, "sqlc.embed("+name+")")
		}
	}
	if jobsWriteReturnsStar(sql) {
		out = append(out, "RETURNING * of a write on jobs")
	}
	return out
}

func TestBaseTableCompleteRowDetector(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
		want bool
	}{
		{"star select", "SELECT *\nFROM jobs\nWHERE id = $1", true},
		{"star select over the view", "SELECT *\nFROM jobs_read\nWHERE id = $1", false},
		{"qualified star", "SELECT j.* FROM jobs j", true},
		{"any alias", "SELECT jb.id, jb.* FROM jobs AS jb WHERE jb.id = $1", true},
		{"alias on an update", "UPDATE jobs x SET title = $2 WHERE x.id = $1 RETURNING x.*", true},
		{"schema qualified", "SELECT * FROM public.jobs", true},
		{"star plus columns", "SELECT *, 1 AS n FROM jobs", true},
		{"embed through an alias", "SELECT sqlc.embed(jb) FROM jobs jb", true},
		{"alias of another table", "SELECT x.* FROM user_jobs x JOIN jobs j ON j.id = x.job_id", false},
		{"alias reused for another relation", "WITH agg AS (SELECT a.id FROM jobs a) SELECT a.* FROM agg a", false},
		{"keyword is not an alias", "SELECT j.id FROM jobs WHERE id = 1 ORDER BY id", false},
		{"embed", "RETURNING sqlc.embed(jobs), x", true},
		{"update returning star", "UPDATE jobs SET title = $2 WHERE id = $1\nRETURNING *;", true},
		{"insert returning star", "INSERT INTO jobs (a) VALUES (1) ON CONFLICT DO UPDATE SET a = 2 RETURNING *;", true},
		{"narrow returning", "UPDATE jobs SET title = $2 WHERE id = $1\nRETURNING jobs.id;", false},
		{"another table's star", "WITH b AS (UPDATE jobs SET n = n + 1 WHERE id = $1 RETURNING jobs.id)\nINSERT INTO user_jobs (job_id) SELECT id FROM b RETURNING *", false},
		{"a comment is not SQL", "-- SELECT * FROM jobs\nSELECT id FROM jobs", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(baseTableCompleteRowUses(tc.sql)) > 0; got != tc.want {
				t.Errorf("baseTableCompleteRowUses(%q) = %v, want %v", tc.sql, got, tc.want)
			}
		})
	}
}

// TestNoStatementReturnsTheCompleteBaseJobsRow walks every query file. A statement that needs
// the whole job reads jobs_read; one that writes a job returns narrow columns.
func TestNoStatementReturnsTheCompleteBaseJobsRow(t *testing.T) {
	entries, err := os.ReadDir("queries")
	if err != nil {
		t.Fatalf("read queries: %v", err)
	}
	var walked int
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		path := filepath.Join("queries", entry.Name())
		src, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, stmt := range splitNamedQueries(string(src)) {
			walked++
			for _, use := range baseTableCompleteRowUses(stmt.body) {
				t.Errorf("%s: %s uses %s on the base jobs table. Read the complete row from jobs_read, "+
					"or return the narrow columns of a write and re-read through GetJob.", path, stmt.name, use)
			}
		}
	}
	if walked < 1000 {
		t.Errorf("walked only %d statements; the walker has probably stopped seeing the query files", walked)
	}
}
