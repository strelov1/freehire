package testdb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shared template is reused across runs by name alone, so the name must move with
// every migration's content and file name, and stay put otherwise.
func TestTemplateNameFollowsTheMigrations(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("0001_a.sql", "CREATE TABLE a (id int);")
	b := write("0002_b.sql", "CREATE TABLE b (id int);")

	name := func(scripts ...string) string {
		t.Helper()
		n, err := templateName(scripts)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	base := name(a, b)
	if !strings.HasPrefix(base, sharedTemplatePrefix) || len(base) > 63 {
		t.Fatalf("template name %q: want prefix %q and a valid identifier length", base, sharedTemplatePrefix)
	}
	if again := name(a, b); again != base {
		t.Fatalf("same migrations, different names: %q vs %q", base, again)
	}
	if added := name(a, b, write("0003_c.sql", "SELECT 1;")); added == base {
		t.Fatal("a new migration kept the old template name")
	}
	write("0002_b.sql", "CREATE TABLE b (id bigint);")
	if edited := name(a, b); edited == base {
		t.Fatal("an edited migration kept the old template name")
	}
	renamed := write("0002_b_renamed.sql", "CREATE TABLE b (id int);")
	write("0002_b.sql", "CREATE TABLE b (id int);")
	if moved := name(a, renamed); moved == base {
		t.Fatal("a renamed migration kept the old template name")
	}
}
