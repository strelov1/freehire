package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "0002_second.sql", "SELECT 2;")
	writeFile(t, dir, "0001_first.sql", "SELECT 1;")
	writeFile(t, dir, "notes.txt", "not a migration")
	writeFile(t, dir, "0009_dup_a.sql", "SELECT 9;")
	writeFile(t, dir, "0009_dup_b.sql", "SELECT 9;")

	migs, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	expected := []string{"0001_first.sql", "0002_second.sql", "0009_dup_a.sql", "0009_dup_b.sql"}
	if len(migs) != len(expected) {
		t.Fatalf("Load returned %d migrations, want %d", len(migs), len(expected))
	}
	for i, v := range expected {
		if migs[i].Version != v {
			t.Errorf("migs[%d].Version = %q, want %q (lexicographic order, non-sql skipped)", i, migs[i].Version, v)
		}
	}
	if migs[0].SQL != "SELECT 1;" {
		t.Errorf("migs[0].SQL = %q, want the file contents", migs[0].SQL)
	}
}

func TestLoad_MissingDir(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected an error for a missing directory, got nil")
	}
}

func TestLoad_NoTxMarker(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "0001_plain.sql", "-- comment\nCREATE TABLE t (id int);\n")
	writeFile(t, dir, "0002_concurrent.sql", "-- build the index online:\n-- migrate: no-transaction\nCREATE INDEX CONCURRENTLY i ON t (id);\n")
	writeFile(t, dir, "0003_late_marker.sql", "CREATE TABLE t (id int);\n-- migrate: no-transaction\n")

	migs, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if migs[0].NoTx {
		t.Errorf("%s: NoTx = true, want false", migs[0].Version)
	}
	if !migs[1].NoTx {
		t.Errorf("%s: NoTx = false, want true (marker in the leading comment block)", migs[1].Version)
	}
	if migs[2].NoTx {
		t.Errorf("%s: NoTx = true, want false (marker after the first statement does not count)", migs[2].Version)
	}
}

// The marker is an instruction, not a topic. A file DISCUSSING it in its preamble was
// opted out by it: hasNoTxMarker asked strings.Contains, so migrations/0126 — whose header
// says "No `migrate: no-transaction` marker, so the two statements are one transaction" —
// answered true, which is the exact opposite of what it says. It survived only by accident,
// because a multi-statement simple query is one implicit transaction anyway.
//
// scripts/check-migrations.mjs repeats this logic, so squawk linted 0126 under the same
// wrong premise.
func TestNoTxMarkerMustBeTheWholeCommentLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "0001_discusses.sql",
		"-- No `migrate: no-transaction` marker, so the two statements are one transaction.\n"+
			"ALTER TABLE t ADD COLUMN c int;\n")
	writeFile(t, dir, "0002_declares.sql", "-- migrate: no-transaction\nCREATE INDEX CONCURRENTLY i ON t (id);\n")
	writeFile(t, dir, "0003_declares_indented.sql", "--   migrate: no-transaction  \nCREATE INDEX CONCURRENTLY i ON t (id);\n")

	migs, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if migs[0].NoTx {
		t.Error("a file that merely NAMES the marker was opted out by it")
	}
	if !migs[1].NoTx {
		t.Error("a file declaring the marker was not opted out")
	}
	if !migs[2].NoTx {
		t.Error("whitespace around the marker defeated it")
	}
}

// The guard on the real catalogue, not on fixtures: this is the file the defect was found
// in, and a fixture cannot notice the next one.
func TestNoRealMigrationIsOptedOutByDiscussingTheMarker(t *testing.T) {
	migs, err := Load(filepath.Join("..", "..", "..", "migrations"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, m := range migs {
		if !m.NoTx {
			continue
		}
		first := ""
		for _, line := range strings.Split(m.SQL, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				first = line
				break
			}
		}
		if first != "-- "+noTxMarker {
			t.Errorf("%s reads as no-transaction, but its first line is %q", m.Version, first)
		}
	}
}

// splitStatements is what makes the marker mean what its files say. The runner sent the
// whole file to one Exec, and pgx forces the simple protocol when there are no arguments,
// which Postgres runs as ONE implicit transaction — so the marker cancelled only the
// runner's own BEGIN/COMMIT. migrations/0135's header describes a guarantee ("NOT VALID +
// VALIDATE CONSTRAINT in one transaction holds ACCESS EXCLUSIVE for the whole scan") that
// it was not getting.
func TestSplitStatementsRespectsQuotingAndComments(t *testing.T) {
	cases := map[string]struct {
		sql  string
		want []string
	}{
		"plain": {
			"CREATE INDEX a ON t (id);\nCREATE INDEX b ON t (id);\n",
			[]string{"CREATE INDEX a ON t (id)", "CREATE INDEX b ON t (id)"},
		},
		"a semicolon inside a string is not a separator": {
			"INSERT INTO t VALUES ('a;b');\nSELECT 1;",
			[]string{"INSERT INTO t VALUES ('a;b')", "SELECT 1"},
		},
		"a doubled quote does not end the string": {
			"INSERT INTO t VALUES ('it''s; fine');",
			[]string{"INSERT INTO t VALUES ('it''s; fine')"},
		},
		"a semicolon inside a dollar-quoted body is not a separator": {
			"CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END $$ LANGUAGE plpgsql;\nSELECT 2;",
			[]string{"CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END $$ LANGUAGE plpgsql", "SELECT 2"},
		},
		"a tagged dollar quote closes only on its own tag": {
			"DO $mig$ BEGIN PERFORM 1; END $mig$;",
			[]string{"DO $mig$ BEGIN PERFORM 1; END $mig$"},
		},
		"a semicolon inside a line comment is not a separator": {
			"-- drop it; then rebuild\nDROP INDEX i;",
			[]string{"-- drop it; then rebuild\nDROP INDEX i"},
		},
		"a semicolon inside a block comment is not a separator": {
			"/* a; b */ SELECT 1;",
			[]string{"/* a; b */ SELECT 1"},
		},
		"a semicolon inside a quoted identifier is not a separator": {
			`CREATE INDEX "odd;name" ON t (id);`,
			[]string{`CREATE INDEX "odd;name" ON t (id)`},
		},
		"a trailing statement without its semicolon still counts": {
			"SELECT 1;\nSELECT 2",
			[]string{"SELECT 1", "SELECT 2"},
		},
		"comment-only tail yields nothing": {
			"SELECT 1;\n-- done\n",
			[]string{"SELECT 1"},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := splitStatements(c.sql)
			if len(got) != len(c.want) {
				t.Fatalf("split into %d statements %q, want %d %q", len(got), got, len(c.want), c.want)
			}
			for i := range got {
				if strings.TrimSpace(got[i]) != c.want[i] {
					t.Errorf("statement %d = %q, want %q", i, strings.TrimSpace(got[i]), c.want[i])
				}
			}
		})
	}
}

// Every no-transaction file in the catalogue must split into the statements it reads as,
// so a future one-statement rule has a number to check and the runner has the same view of
// the file a reader does.
func TestEveryNoTxMigrationSplitsIntoWholeStatements(t *testing.T) {
	migs, err := Load(filepath.Join("..", "..", "..", "migrations"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, m := range migs {
		if !m.NoTx {
			continue
		}
		if len(splitStatements(m.SQL)) == 0 {
			t.Errorf("%s is no-transaction but splits into no statements", m.Version)
		}
	}
}

func TestDecide(t *testing.T) {
	migs := []Migration{
		{Version: "0001_a.sql"},
		{Version: "0002_b.sql"},
		{Version: "0003_c.sql"},
	}
	applied := map[string]bool{"0001_a.sql": true}

	versions := func(ms []Migration) []string {
		var out []string
		for _, m := range ms {
			out = append(out, m.Version)
		}
		return out
	}

	t.Run("fresh database applies pending files", func(t *testing.T) {
		p := decide(migs, applied, false, false)
		if len(p.baseline) != 0 {
			t.Errorf("baseline = %v, want empty", versions(p.baseline))
		}
		if got := versions(p.apply); len(got) != 2 || got[0] != "0002_b.sql" || got[1] != "0003_c.sql" {
			t.Errorf("apply = %v, want [0002_b.sql 0003_c.sql]", got)
		}
	})

	t.Run("legacy schema baselines pending files", func(t *testing.T) {
		p := decide(migs, applied, true, false)
		if len(p.apply) != 0 {
			t.Errorf("apply = %v, want empty", versions(p.apply))
		}
		if got := versions(p.baseline); len(got) != 2 || got[0] != "0002_b.sql" || got[1] != "0003_c.sql" {
			t.Errorf("baseline = %v, want [0002_b.sql 0003_c.sql]", got)
		}
	})

	t.Run("force baseline overrides a fresh database", func(t *testing.T) {
		p := decide(migs, applied, false, true)
		if len(p.apply) != 0 {
			t.Errorf("apply = %v, want empty", versions(p.apply))
		}
		if len(p.baseline) != 2 {
			t.Errorf("baseline = %v, want 2 entries", versions(p.baseline))
		}
	})

	t.Run("nothing pending", func(t *testing.T) {
		all := map[string]bool{"0001_a.sql": true, "0002_b.sql": true, "0003_c.sql": true}
		p := decide(migs, all, true, false)
		if len(p.baseline) != 0 || len(p.apply) != 0 {
			t.Errorf("plan = %+v, want empty", p)
		}
	})
}
