package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// rawSQLCall matches a query issued straight against a pgx pool or connection —
// pool.Query(...), h.pool.QueryRow(...), cfg.Pool.Exec(...) and so on.
var rawSQLCall = regexp.MustCompile(`(?i)\b\w*pool\.(Query|QueryRow|Exec|SendBatch)\(`)

// TestNoRawSQLInHandlers keeps every read and write in this package behind sqlc or a
// domain store.
//
// The rule is not style. A hand-written statement is invisible to sqlc, so a column
// renamed in migrations/ compiles clean and fails when a user makes the request — which is
// exactly how the autofill contact read used to break. Stated as a test rather than as a
// sentence in AGENTS.md, because this codebase's most common defect is a true-sounding
// sentence that nothing enforces.
//
// The population is derived from the package's own sources, not from a hand-kept list, so
// a new file is covered the moment it is added.
func TestNoRawSQLInHandlers(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		scanned++
		for i, line := range strings.Split(string(src), "\n") {
			if rawSQLCall.MatchString(line) {
				t.Errorf("%s:%d issues SQL straight at the pool — use sqlc or a domain store:\n\t%s",
					name, i+1, strings.TrimSpace(line))
			}
		}
	}

	// Guard the guard: a rule that silently scans nothing passes forever.
	if scanned == 0 {
		t.Fatal("scanned no source files; the rule would pass vacuously")
	}
}
