package job_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/modroot"
)

// A posting is observed (job_activity) in the SAME transaction that writes it, by the second
// command job.Upsert issues after UpsertJob. A caller that reaches the generated UpsertJob or
// ObserveJobActivity directly writes a posting nobody observed: for a new row its hydration stays
// NULL (read as "stale body"), for an existing one last_seen_at stops moving and the unseen
// sweep closes a live posting. Only upsert.go may name them.
func TestOnlyJobUpsertCallsTheGeneratedFullWrite(t *testing.T) {
	root, err := modroot.Find()
	if err != nil {
		t.Fatal(err)
	}
	guarded := map[string]bool{"UpsertJob": true, "ObserveJobActivity": true}
	allowed := filepath.ToSlash(filepath.Join("internal", "job", "job", "upsert.go"))
	generated := filepath.ToSlash(filepath.Join("internal", "platform", "db")) + "/"

	var offenders []string
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "web", "extension", "design-system":
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") || rel == allowed || strings.HasPrefix(rel, generated) {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", rel, perr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && guarded[sel.Sel.Name] {
					offenders = append(offenders, rel+": "+sel.Sel.Name+" at "+fset.Position(call.Pos()).String())
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range offenders {
		t.Errorf("%s — use job.Upsert", o)
	}
}
