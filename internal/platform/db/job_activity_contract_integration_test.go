//go:build integration

// Release B contract rehearsal. The contract release (C) drops jobs.last_seen_at, hydrated_at and
// liveness_strikes while binaries of release B may still run, and anything in their SQL that still
// names a dropped column fails with 42703 on every call. A source regex cannot see what sqlc
// expanded (a `RETURNING *` or an embedded row becomes an explicit column list), so this does what
// the database will do: in an isolated copy of the release B schema it installs the intended C
// insert-trigger body, removes the legacy bridge, drops the three columns, and prepares every SQL
// constant of the generated code, then requires that nothing which prepared before now fails.
// Statements are prepared, not executed.
//
// The C migration itself is NOT in migrations/: it is added when the B rollout has drained.
//
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// generatedSQL returns every query constant of the sqlc output in this package by name, read
// from the source with the Go parser so no SQL parser is involved.
func generatedSQL(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("*.sql.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no generated query files (%v)", err)
	}
	out := map[string]string{}
	fset := token.NewFileSet()
	for _, f := range files {
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if sql, ok := constString(t, vs.Values[i]); ok {
						out[name.Name] = sql
					}
				}
			}
		}
	}
	return out
}

// constString evaluates a string constant expression: a literal, or literals joined by + (sqlc
// splices a backtick that occurs in a query comment into the raw string as ` + "`" + `).
func constString(t *testing.T, e ast.Expr) (string, bool) {
	t.Helper()
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		if err != nil {
			t.Fatalf("unquote %s: %v", e.Value, err)
		}
		return s, true
	case *ast.BinaryExpr:
		left, leftOK := constString(t, e.X)
		right, rightOK := constString(t, e.Y)
		return left + right, e.Op == token.ADD && leftOK && rightOK
	}
	return "", false
}

// prepareAll prepares every statement on one connection and returns the failures by name.
func prepareAll(t *testing.T, pool *pgxpool.Pool, stmts map[string]string) map[string]string {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	failed := map[string]string{}
	for name, sql := range stmts {
		if _, err := conn.Conn().Prepare(ctx, name, sql); err != nil {
			failed[name] = err.Error()
		}
	}
	// DeallocateAll, not `DEALLOCATE ALL`: pgx caches a prepared statement by name and SQL on the
	// client and would answer the second pass from that cache without asking the server.
	if err := conn.Conn().DeallocateAll(ctx); err != nil {
		t.Fatalf("deallocate: %v", err)
	}
	return failed
}

// knownBaselineFailures are the generated statements that do not prepare even on the release B
// schema, each with the one reason that is expected. The baseline comparison below ignores them in
// the after-pass, so the list is exact on purpose: any other baseline failure, or the same
// statement failing for a different reason, fails the test, and an entry whose SQL names a legacy
// column is refused (a statement that already fails would hide it: Postgres reports one error).
var knownBaselineFailures = map[string]string{
	// internal/platform/db/link_contributions_backfill.sql.go has no source in queries/ and no
	// caller; migrations/0131_drop_link_contributions.sql dropped its table. A dead generated file.
	"listLinkContributionsForBackfill": `relation "link_contributions" does not exist`,
}

var reLegacyName = regexp.MustCompile(`(?i)\b(last_seen_at|hydrated_at|liveness_strikes)\b`)

// unexpectedBaseline lists what is wrong with the baseline failures: a failure not in the
// allow-list, an allow-listed one failing for another reason, an allow-listed one that now
// prepares (the entry is stale), and an allow-listed statement that names a legacy column.
func unexpectedBaseline(stmts, before, allow map[string]string) []string {
	var bad []string
	for name, msg := range before {
		reason, ok := allow[name]
		switch {
		case !ok:
			bad = append(bad, "unexpected baseline failure "+name+": "+msg)
		case !strings.Contains(msg, reason):
			bad = append(bad, "baseline failure "+name+" has changed reason: "+msg+" (expected it to contain "+reason+")")
		}
	}
	for name := range allow {
		if _, fails := before[name]; !fails {
			bad = append(bad, "allow-list entry "+name+" prepares now or no longer exists: remove it")
		}
		if sql, ok := stmts[name]; ok && reLegacyName.MatchString(sql) {
			bad = append(bad, "allow-listed statement "+name+" names a legacy column: its failure would hide the stale reference")
		}
	}
	sort.Strings(bad)
	return bad
}

// The allow-list logic itself, without a database: each way a baseline can go wrong is caught.
func TestUnexpectedBaselineCatchesEveryDrift(t *testing.T) {
	allow := map[string]string{"dead": `relation "gone" does not exist`}
	stmts := map[string]string{"dead": "SELECT 1 FROM gone", "other": "SELECT 1"}
	for _, tc := range []struct {
		name         string
		before       map[string]string
		stmts        map[string]string
		wantProblems int
	}{
		{"exactly the allow-list", map[string]string{"dead": `ERROR: relation "gone" does not exist`}, stmts, 0},
		{"a new failure", map[string]string{"dead": `relation "gone" does not exist`, "other": "boom"}, stmts, 1},
		{"changed reason", map[string]string{"dead": `column "x" does not exist`}, stmts, 1},
		{"stale entry", map[string]string{}, stmts, 1},
		{"allow-listed statement naming a legacy column",
			map[string]string{"dead": `relation "gone" does not exist`},
			map[string]string{"dead": "SELECT last_seen_at FROM gone"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := unexpectedBaseline(tc.stmts, tc.before, allow); len(got) != tc.wantProblems {
				t.Errorf("problems = %v, want %d", got, tc.wantProblems)
			}
		})
	}
}

func TestReleaseCContractPreparesEveryGeneratedStatement(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	stmts := generatedSQL(t)
	if len(stmts) < 1000 {
		t.Fatalf("found only %d query constants: the extraction has drifted from the generated files", len(stmts))
	}

	// A control that must break: proves the rehearsal can see a stale column at all.
	const control = "controlNamesALegacyColumn"
	stmts[control] = `SELECT last_seen_at FROM jobs WHERE id = $1`

	before := prepareAll(t, pool, stmts)
	real := len(stmts) - 1 // the control is not a generated statement
	t.Logf("release B schema: %d of %d generated statements prepare, %d fail (allow-list: %d)",
		real-len(before), real, len(before), len(knownBaselineFailures))
	if _, bad := before[control]; bad {
		t.Fatal("the control does not prepare on the release B schema: the rehearsal baseline is wrong")
	}
	if problems := unexpectedBaseline(stmts, before, knownBaselineFailures); len(problems) > 0 {
		t.Fatalf("the release B baseline is not what the allow-list says:\n%s", strings.Join(problems, "\n"))
	}

	for _, ddl := range []string{
		// The intended release C insert trigger: every new job gets an activity row of defaults.
		`CREATE OR REPLACE FUNCTION job_activity_insert() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		    INSERT INTO job_activity AS a (job_id) VALUES (NEW.id)
		    ON CONFLICT (job_id) DO UPDATE SET
		        last_seen_at = now(), hydrated_at = NULL, liveness_strikes = 0, state_version = a.state_version + 1;
		    RETURN NULL;
		END;
		$$`,
		// A trigger bound with UPDATE OF <column> blocks DROP COLUMN, so the bridge goes first.
		`DROP TRIGGER job_activity_legacy_bridge ON jobs`,
		`DROP FUNCTION job_activity_legacy_bridge()`,
		`ALTER TABLE jobs DROP COLUMN last_seen_at, DROP COLUMN hydrated_at, DROP COLUMN liveness_strikes`,
	} {
		if _, err := pool.Exec(ctx, ddl); err != nil {
			t.Fatalf("contract DDL failed (%v):\n%s", err, ddl)
		}
	}

	after := prepareAll(t, pool, stmts)
	t.Logf("after the contract DDL: %d of %d generated statements prepare, %d fail (the %d baseline failure(s) + the control, which must break)",
		real-(len(after)-1), real, len(after)-1, len(before))
	if _, caught := after[control]; !caught {
		t.Fatal("the control statement still prepares after the columns were dropped: the rehearsal cannot see stale columns")
	}
	var broken []string
	for name, msg := range after {
		if _, was := before[name]; !was && name != control {
			broken = append(broken, name+": "+msg)
		}
	}
	sort.Strings(broken)
	if len(broken) > 0 {
		t.Errorf("%d statements prepared on the release B schema and fail once the legacy columns are dropped:\n%s",
			len(broken), strings.Join(broken, "\n"))
	}
	// The rehearsal must also see the statements that matter: they prepare, and the view still serves them.
	for _, name := range []string{"observeJobActivity", "touchOpenJob", "refreshUnchangedJob", "refreshCoveredJobs", "refreshCoveredJob", "getJob", "upsertJob"} {
		if _, ok := stmts[name]; !ok {
			t.Errorf("statement %s is not among the generated constants", name)
		} else if msg, bad := after[name]; bad {
			t.Errorf("%s does not prepare without the legacy columns: %s", name, msg)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs_read`).Scan(&n); err != nil {
		t.Errorf("jobs_read does not survive the drop: %v", err)
	}
}
