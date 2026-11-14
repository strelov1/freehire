package db

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Release B moves every write of last_seen_at, hydrated_at and liveness_strikes to job_activity.
// No statement may name them on jobs any more: the contract release drops the columns, and a
// binary that still names one fails with 42703 on every call. The prepare-against-a-dropped-column
// rehearsal (job_activity_contract_integration_test.go) is the authority, because it sees what sqlc
// expanded; this cheap walk catches the plain spellings at review time, before a database exists.
//
// A name is fine when it belongs to job_activity or to a CTE that carries its value: only a qualifier
// that is the jobs table, or an alias bound to jobs alone, is judged, and so is an unqualified name in a
// statement over jobs with no job_activity in it.
var (
	reLegacyColumn = regexp.MustCompile(`\b(last_seen_at|hydrated_at|liveness_strikes)\b`)
	reQualified    = regexp.MustCompile(`\b(\w+)\.(last_seen_at|hydrated_at|liveness_strikes)\b`)
)

// legacyColumnUses names the places a statement spells a legacy column against jobs.
func legacyColumnUses(body string) []string {
	sql := strings.ToLower(stripSQLComments(body))
	if !reLegacyColumn.MatchString(sql) {
		return nil
	}
	// A qualifier is judged only when it is the jobs table or an alias bound to jobs and to nothing
	// else; a CTE's alias (`l`, `e`) that carries the activity value under the same name is not.
	onJobs := map[string]bool{}
	for _, name := range jobsNames(sql) {
		onJobs[name] = true
	}
	var out []string
	for _, m := range reQualified.FindAllStringSubmatch(sql, -1) {
		if onJobs[m[1]] {
			out = append(out, m[1]+"."+m[2])
		}
	}
	// Unqualified: a write to jobs (SET / INSERT column list) or a statement with no activity relation at all.
	touchesJobs := regexp.MustCompile(`\b(from|join|update|into)\s+(public\.)?jobs\b`).MatchString(sql)
	touchesActivity := strings.Contains(sql, "job_activity")
	if touchesJobs && !touchesActivity {
		for _, m := range reLegacyColumn.FindAllString(regexp.MustCompile(`\b\w+\.(last_seen_at|hydrated_at|liveness_strikes)\b`).ReplaceAllString(sql, ""), -1) {
			out = append(out, "unqualified "+m+" in a statement over jobs only")
		}
	}
	if setsLegacyOnJobs := regexp.MustCompile(`update\s+(public\.)?jobs\b[^;]*?\bset\b[^;]*?\b(last_seen_at|hydrated_at|liveness_strikes)\s*=`).MatchString(sql); setsLegacyOnJobs {
		out = append(out, "UPDATE jobs SET a legacy column")
	}
	if insertsLegacy := regexp.MustCompile(`insert\s+into\s+(public\.)?jobs\s*\([^)]*\b(last_seen_at|hydrated_at|liveness_strikes)\b`).MatchString(sql); insertsLegacy {
		out = append(out, "INSERT INTO jobs names a legacy column")
	}
	return out
}

func TestLegacyColumnDetector(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
		want bool
	}{
		{"set on jobs", "UPDATE jobs SET last_seen_at = now() WHERE id = $1", true},
		{"qualified on jobs", "SELECT j.liveness_strikes FROM jobs j", true},
		{"jobs.col in RETURNING", "UPDATE jobs SET title = 'x' RETURNING jobs.liveness_strikes", true},
		{"insert column list", "INSERT INTO jobs (source, hydrated_at) VALUES ('a', now())", true},
		{"activity alias", "UPDATE job_activity a SET last_seen_at = now() FROM jobs j WHERE a.job_id = j.id", false},
		{"activity unqualified", "UPDATE job_activity SET liveness_strikes = 0 WHERE job_id = $1 AND liveness_strikes <> 0", false},
		{"join through an alias", "SELECT j.id FROM jobs j JOIN job_activity act ON act.job_id = j.id WHERE act.last_seen_at < now()", false},
		{"output alias only", "SELECT b.strikes AS liveness_strikes FROM bumped b", false},
		{"a comment", "-- UPDATE jobs SET last_seen_at = now()\nSELECT 1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(legacyColumnUses(tc.sql)) > 0; got != tc.want {
				t.Errorf("legacyColumnUses(%q) = %v, want %v", tc.sql, legacyColumnUses(tc.sql), tc.want)
			}
		})
	}
}

func TestNoQueryNamesALegacyActivityColumnOnJobs(t *testing.T) {
	entries, err := os.ReadDir("queries")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		path := filepath.Join("queries", entry.Name())
		src, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range splitNamedQueries(string(src)) {
			for _, use := range legacyColumnUses(stmt.body) {
				t.Errorf("%s: %s uses %s: last_seen_at, hydrated_at and liveness_strikes live in job_activity (release B), "+
					"and the contract release drops them from jobs", path, stmt.name, use)
			}
		}
	}
}

// UpsertJob and UpsertManualJob take FOR NO KEY UPDATE on the existing jobs tuple only while
// their DO UPDATE SET names no column of a unique index. Naming one (public_slug, source,
// external_id, id) upgrades the lock to FOR UPDATE, which conflicts with the KEY SHARE every
// foreign-key child insert holds and closes a deadlock cycle with a cheap transaction that holds
// the activity row. Walked over the two statements' own text.
func TestUpsertDoUpdateNeverSetsAUniqueKeyColumn(t *testing.T) {
	keyAssign := regexp.MustCompile(`(?im)^\s*(id|public_slug|source|external_id)\s*=`)
	for _, f := range []struct{ file, name string }{
		{"queries/jobs_upsert.sql", "UpsertJob"},
		{"queries/jobs_manual.sql", "UpsertManualJob"},
	} {
		src, err := os.ReadFile(f.file)
		if err != nil {
			t.Fatal(err)
		}
		var body string
		for _, stmt := range splitNamedQueries(string(src)) {
			if stmt.name == f.name {
				body = strings.ToLower(stripSQLComments(stmt.body))
			}
		}
		i := strings.Index(body, "do update set")
		if i < 0 {
			t.Fatalf("%s: no DO UPDATE SET found in %s: the walk has drifted from the statement", f.file, f.name)
		}
		set := body[i:]
		if j := strings.Index(set, "returning"); j > 0 {
			set = set[:j]
		}
		if keyAssign.MatchString(set) {
			t.Errorf("%s: %s names a unique-key column in DO UPDATE SET (%q): that upgrades the tuple lock to FOR UPDATE",
				f.file, f.name, keyAssign.FindString(set))
		}
	}
}
