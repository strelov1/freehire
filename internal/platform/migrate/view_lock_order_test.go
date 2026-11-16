package migrate

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	alterTableRe  = regexp.MustCompile(`(?i)^ALTER\s+TABLE\b`)
	replaceViewRe = regexp.MustCompile(`(?i)^CREATE\s+OR\s+REPLACE\s+VIEW\s+(?:public\.)?(\w+)`)
	lockViewRe    = regexp.MustCompile(`(?i)^LOCK\s+(?:TABLE\s+)?(?:ONLY\s+)?(?:public\.)?(\w+)\s+IN\s+ACCESS\s+EXCLUSIVE\s+MODE`)
)

// Applied before this guard existed; each one deadlocked live readers of jobs_read on release.
var viewLockOrderGrandfathered = map[string]bool{
	"0265_jobs_team.sql":            true,
	"0274_jobs_application_url.sql": true,
}

// stripLeadingComments drops the comment lines splitStatements keeps in front of a statement.
func stripLeadingComments(stmt string) string {
	for {
		stmt = strings.TrimSpace(stmt)
		if !strings.HasPrefix(stmt, "--") {
			return stmt
		}
		end := strings.IndexByte(stmt, '\n')
		if end < 0 {
			return ""
		}
		stmt = stmt[end+1:]
	}
}

// viewLockOrderViolations names every view a transactional file replaces after an ALTER TABLE
// without having locked the view first.
func viewLockOrderViolations(sql string) []string {
	var (
		altered bool
		locked  = map[string]bool{}
		bad     []string
	)
	for _, stmt := range splitStatements(sql) {
		stmt = stripLeadingComments(stmt)
		if m := lockViewRe.FindStringSubmatch(stmt); m != nil && !altered {
			locked[strings.ToLower(m[1])] = true
		}
		if alterTableRe.MatchString(stmt) {
			altered = true
		}
		if m := replaceViewRe.FindStringSubmatch(stmt); m != nil && altered && !locked[strings.ToLower(m[1])] {
			bad = append(bad, m[1])
		}
	}
	return bad
}

// ALTER TABLE jobs takes ACCESS EXCLUSIVE on jobs, then CREATE OR REPLACE VIEW jobs_read asks for
// it on the view. A reader of the view goes the other way: the rewriter locks the view, then the
// tables under it. Every read in flight closes a cycle with the migration, and the deadlock
// detector cancels the reads: the releases of 0265 and 0274 each failed about ten job and
// company page requests with 40P01. LOCK TABLE on a view locks the view and then, recursively,
// its tables, the readers' order, so they queue behind the migration instead.
func TestViewLockOrderViolations(t *testing.T) {
	cases := map[string]struct {
		sql  string
		want []string
	}{
		"alter then replace": {
			"ALTER TABLE public.jobs ADD COLUMN c text;\nCREATE OR REPLACE VIEW jobs_read AS SELECT 1;",
			[]string{"jobs_read"},
		},
		"view locked first": {
			"-- readers' order\nLOCK TABLE public.jobs_read IN ACCESS EXCLUSIVE MODE;\nALTER TABLE public.jobs ADD COLUMN c text;\nCREATE OR REPLACE VIEW public.jobs_read AS SELECT 1;",
			nil,
		},
		"lock after the alter is too late": {
			"ALTER TABLE jobs ADD COLUMN c text;\nLOCK TABLE jobs_read IN ACCESS EXCLUSIVE MODE;\nCREATE OR REPLACE VIEW jobs_read AS SELECT 1;",
			[]string{"jobs_read"},
		},
		"a weaker lock does not count": {
			"LOCK TABLE jobs_read IN SHARE MODE;\nALTER TABLE jobs ADD COLUMN c text;\nCREATE OR REPLACE VIEW jobs_read AS SELECT 1;",
			[]string{"jobs_read"},
		},
		"replace without an alter": {
			"CREATE OR REPLACE VIEW jobs_read AS SELECT 1;",
			nil,
		},
	}
	for name, tc := range cases {
		got := viewLockOrderViolations(tc.sql)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s: violations = %v, want %v", name, got, tc.want)
		}
	}
}

func TestViewReplacingMigrationsLockTheViewBeforeAlteringTables(t *testing.T) {
	migs, err := Load(filepath.Join("..", "..", "..", "migrations"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, m := range migs {
		if m.NoTx || viewLockOrderGrandfathered[m.Version] {
			continue
		}
		for _, view := range viewLockOrderViolations(m.SQL) {
			t.Errorf("%s replaces view %s after an ALTER TABLE; start the file with "+
				"LOCK TABLE public.%s IN ACCESS EXCLUSIVE MODE so live readers queue instead of deadlocking",
				m.Version, view, view)
		}
	}
}
