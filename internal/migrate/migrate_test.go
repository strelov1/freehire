package migrate

import (
	"os"
	"path/filepath"
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
